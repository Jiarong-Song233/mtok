package app

import (
	"fmt"
	"os"
	"time"

	"github.com/Jiarong-Song233/mtok/internal/config"
	"github.com/Jiarong-Song233/mtok/internal/magpie"
	"github.com/Jiarong-Song233/mtok/internal/quota"
	"github.com/Jiarong-Song233/mtok/internal/render"
)

// collectQuotas reads the local Codex session rate limits first, then adds
// magpie's proxy readings. A local Codex reading wins because it comes from
// the session log rather than a second-hand proxy view.
func collectQuotas(paths config.Paths, set config.Settings, noMagpie bool) ([]render.Quota, []string) {
	out := []render.Quota{}
	notes := []string{}

	local, ok, err := quota.Codex(paths.Home, paths.QuotaCache)
	if err != nil && !ok {
		notes = append(notes, "Codex 本地额度读取失败："+trimErr(err))
	}
	if ok {
		out = append(out, local)
	} else if err == nil {
		notes = append(notes, "Codex 本地额度：未找到 rate_limits 事件")
	}

	// T3 gateway caches carry 5-hour and weekly allowances for agents that talk
	// straight to their vendors (Gemini via Antigravity, Claude Code, Grok) and
	// so never appear in the proxied magpie log. Reading them stays offline.
	t3, err := quota.T3(paths.Home)
	if err != nil {
		notes = append(notes, "T3 额度读取失败："+trimErr(err))
	}
	for _, q := range t3 {
		out = append(out, render.Quota{
			Provider:   q.Provider,
			Name:       q.Name,
			Remaining:  q.Remaining,
			Detail:     q.Detail,
			Source:     q.Source,
			ObservedAt: q.ObservedAt,
			Windows:    q.Windows,
		})
	}

	if !noMagpie && set.Magpie {
		qs, err := magpie.New(set.MagpieURL).Quotas()
		if err != nil {
			notes = append(notes, "magpie 额度离线（"+trimErr(err)+"）")
		}
		for _, q := range qs {
			if ok && q.Provider == "codex" {
				continue
			}
			out = append(out, render.Quota{
				Provider:  q.Provider,
				Name:      q.Name,
				Remaining: q.Remaining,
				Detail:    quotaDetail(q),
				Source:    quota.SourceMagpie,
			})
		}
	}
	return out, notes
}

// runQuota serves the standalone quota command without opening the usage
// database or scanning agent logs.
func runQuota(a Args, paths config.Paths, set config.Settings, o options) int {
	now := time.Now()
	qs, notes := collectQuotas(paths, set, o.noMagpie)
	if o.json {
		if err := render.RenderQuotaJSON(a.Stdout, qs, notes, now); err != nil {
			fmt.Fprintln(a.Stderr, "mtok: "+err.Error())
			return 1
		}
		return 0
	}
	width := set.Width
	if o.width > 0 {
		width = o.width
	}
	v := &render.View{
		Now:        now,
		Width:      width,
		Quotas:     qs,
		QuotaNotes: notes,
		Color:      !o.noColor && a.IsTTY && os.Getenv("NO_COLOR") == "",
	}
	v.RenderQuotas(a.Stdout)
	return 0
}
