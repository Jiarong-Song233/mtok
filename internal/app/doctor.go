package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/songjiarong/mtok/internal/adapters"
	"github.com/songjiarong/mtok/internal/globx"
	"github.com/songjiarong/mtok/internal/magpie"
	"github.com/songjiarong/mtok/internal/render"
)

// doctor checks every source end to end: are the logs there, do the fields
// parse, does the total agree with magpie. Nothing missing is left silent.
func (e *env) doctor(o options) int {
	w := e.args.Stdout
	fmt.Fprintf(w, "mtok %s  doctor\n", e.args.Version)
	fmt.Fprintf(w, "  platform   %s  %s\n", e.args.Platform, e.args.GoVersion)
	fmt.Fprintf(w, "  config     %s\n", e.paths.ConfigDir)
	fmt.Fprintf(w, "  database   %s\n", e.paths.DB)
	fmt.Fprintf(w, "  pricing    %s\n", e.price.Source())

	if err := e.syncNow(o); err != nil {
		fmt.Fprintf(w, "  sync error %v\n", err)
	}
	r, files, bytes := e.store.Stats()
	days := e.store.Days()
	first, last := "—", "—"
	if len(days) > 0 {
		first, last = days[0], days[len(days)-1]
	}
	fmt.Fprintf(w, "  data       %d rows, %d files tracked, %s, days %s..%s\n",
		r, files, ingestBytes(bytes), first, last)
	if e.store.GetMeta("last_sync") != "" {
		fmt.Fprintf(w, "  last sync  %s\n", e.store.GetMeta("last_sync"))
	}
	fmt.Fprintln(w)

	stats := map[string]adaptersStat{}
	for _, s := range e.sync.Stats {
		stats[s.Name] = adaptersStat{
			files: s.Files, changed: s.Changed, records: s.Records,
			bytes: s.Bytes, models: s.Models, errorsN: s.Errors,
			lastErr: s.LastErr, notime: s.NoTime, skipped: int64(s.Skipped), elapsedMS: s.ElapsedMS,
			globs: s.Globs,
		}
	}
	fmt.Fprintln(w, e.hdr("ADAPTER      状态      文件   增  记录/本次  耗时   模型"))
	for _, a := range e.adapters {
		st, seen := stats[a.Name()]
		status := "ok"
		switch {
		case !seen:
			status = "disabled"
		case st.errorsN > 0 && st.files > 0 && st.changed == 0:
			status = "error"
		case st.files == 0:
			status = "no logs"
		case st.records == 0 && st.changed == 0 && st.files > 0:
			status = "cached"
		}
		modelTxt := strings.Join(st.models, ",")
		if len(st.models) > 2 {
			modelTxt = strings.Join(st.models[:2], ",") + fmt.Sprintf("+%d", len(st.models)-2)
		}
		if modelTxt == "" {
			modelTxt = "—"
		}
		fmt.Fprintf(w, "  %-12s %-9s %5d %3d %8d %5dms  %s\n",
			a.Name(), status, st.files, st.changed, st.records, st.elapsedMS, render.Clip(modelTxt, 24))
		if st.notime > 0 {
			fmt.Fprintf(w, "      %s %d 条记录无时间戳，已计入扫描当天\n", e.dim("!"), st.notime)
		}
		if st.skipped > 0 {
			fmt.Fprintf(w, "      %s %d 个压缩归档已入库，未重复扫描\n", e.dim("·"), st.skipped)
		}
	}
	for _, err := range e.declErrs {
		fmt.Fprintf(w, "  %s 声明式 adapter 失败: %v\n", e.dim("!"), err)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, e.hdr("暂不支持（本机存在，但日志里没有可直接读取的 token 计数）"))
	missing := 0
	for _, u := range adapters.UnsupportedSources(e.paths.Home) {
		if _, err := os.Stat(u.Probe); err != nil {
			continue
		}
		missing++
		fmt.Fprintf(w, "  %-10s %s\n", u.Name, e.dim(render.Clip(u.Reason, 60)))
	}
	if missing == 0 {
		fmt.Fprintln(w, "  "+e.dim("（无）"))
	}
	fmt.Fprintln(w)

	// Cross-check against the proxy for today, the only window where both
	// sides have seen the same requests.
	if e.set.Magpie && !o.noMagpie {
		fmt.Fprintln(w, e.hdr("与 magpie 今日口径对照（/v1/magpie/usage?period=today）"))
		c := magpie.New(e.set.MagpieURL)
		u, err := c.Usage("today")
		if err != nil {
			fmt.Fprintf(w, "  %s magpie 不可用: %v\n", e.dim("·"), trimErr(err))
		} else {
			today := e.todayTotals()
			fmt.Fprintf(w, "  %-10s %12s %12s %12s\n", "", "input", "output", "cache_read")
			fmt.Fprintf(w, "  %-10s %12d %12d %12d\n", "mtok", today.input, today.output, today.cacheRead)
			fmt.Fprintf(w, "  %-10s %12d %12d %12d\n", "magpie", u.Input, u.Output, u.CacheRead)
			fmt.Fprintf(w, "  %-10s %12s %12s %12s\n", "差值",
				diffPct(today.input, u.Input), diffPct(today.output, u.Output), diffPct(today.cacheRead, u.CacheRead))
			fmt.Fprintln(w, "  "+e.dim("只有经过 magpie 的请求才会出现在右侧：直连厂商的 agent 只计入 mtok。"))
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, e.hdr("提示"))
	if !e.set.Zstd {
		fmt.Fprintln(w, "  "+e.dim("zstd 已关闭：.jsonl.zst 归档（codex/dsh）不计入，可在 config.yaml 打开。"))
	}
	if len(e.set.Disabled) > 0 {
		var names []string
		for n := range e.set.Disabled {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Fprintf(w, "  %s 已通过 config.yaml 禁用: %s\n", e.dim("·"), strings.Join(names, " "))
	}
	fmt.Fprintln(w, "  "+e.dim("模型名归一化后跨厂商合并；来源目录可用 mtok adapters 查看。"))
	return 0
}

type adaptersStat struct {
	files, changed                    int
	records, errorsN, notime, skipped int64
	elapsedMS                         int64
	_                                 struct{}
	bytes                             int64
	models                            []string
	lastErr                           string
	globs                             []string
}

type totals struct {
	input, output, cacheRead, cacheWrite int64
}

// todayTotals sums the stored rows for the local day.
func (e *env) todayTotals() totals {
	day := nowDay()
	var t totals
	for _, r := range e.store.Rows() {
		if r.Day != day {
			continue
		}
		t.input += r.Input
		t.output += r.Output
		t.cacheRead += r.CacheRead
		t.cacheWrite += r.CacheWrite
	}
	return t
}

func diffPct(a, b int64) string {
	if b == 0 {
		if a == 0 {
			return "0%"
		}
		return "n/a"
	}
	d := (float64(a) - float64(b)) / float64(b) * 100
	return fmt.Sprintf("%+.1f%%", d)
}

func ingestBytes(n int64) string { return humanKB(n) }

// listAdapters prints every adapter, its globs and load state.
func (e *env) listAdapters(o options) int {
	w := e.args.Stdout
	fmt.Fprintf(w, "mtok %s  adapters\n\n", e.args.Version)
	fmt.Fprintln(w, e.hdr("BUILT-IN"))
	for _, a := range e.adapters {
		state := "on"
		if e.set.Disabled[a.Name()] {
			state = "off"
		}
		kind := "go"
		if _, ok := a.(*adapters.Decl); ok {
			kind = "yaml"
		}
		fmt.Fprintf(w, "  %-12s %-4s %-4s %s\n", a.Name(), kind, state, e.dim(a.Agent()))
		for _, g := range a.Globs() {
			fmt.Fprintf(w, "      %s\n", e.compactPath(g))
		}
		needles := adapters.Needles(a)
		if len(needles) > 0 {
			fmt.Fprintf(w, "      %s %s\n", e.dim("prefilter:"), e.dim(strings.Join(needles, " | ")))
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, e.hdr("暂不支持"))
	for _, u := range adapters.UnsupportedSources(e.paths.Home) {
		state := "absent"
		if _, err := os.Stat(u.Probe); err == nil {
			state = "present"
		}
		fmt.Fprintf(w, "  %-10s %-8s %s\n", u.Name, state, e.dim(render.Clip(u.Reason, 56)))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  声明式目录 %s\n", e.paths.Adaptersd)
	if len(e.declErrs) == 0 {
		fmt.Fprintln(w, "  "+e.dim("（未发现加载错误）"))
	}
	for _, err := range e.declErrs {
		fmt.Fprintf(w, "  %s %v\n", e.dim("!"), err)
	}
	return 0
}

// showConfig prints resolved paths and settings.
func (e *env) showConfig(o options) int {
	w := e.args.Stdout
	fmt.Fprintf(w, "mtok %s  config\n\n", e.args.Version)
	rows := [][2]string{
		{"config dir", e.paths.ConfigDir},
		{"config.yaml", e.paths.Config},
		{"aliases.yaml", e.paths.Aliases},
		{"pricing.json", e.set.Pricing},
		{"adapters.d", e.paths.Adaptersd},
		{"database", e.paths.DB},
	}
	for _, p := range rows {
		mark := ""
		if _, err := os.Stat(p[1]); err == nil {
			mark = e.dim("  [present]")
		}
		fmt.Fprintf(w, "  %-13s %s%s\n", p[0], e.compactPath(p[1]), mark)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %-13s width=%d top=%d currency=%s rate=%.2f magpie=%v zstd=%v\n",
		"settings", e.set.Width, e.set.Top, e.set.Currency, e.set.Rate, e.set.Magpie, e.set.Zstd)
	if len(e.set.Disabled) > 0 {
		var names []string
		for n := range e.set.Disabled {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Fprintf(w, "  %-13s %s\n", "disabled", strings.Join(names, " "))
	}
	r, files, bytes := e.store.Stats()
	fmt.Fprintf(w, "  %-13s %d rows, %d files, %s on disk\n", "store", r, files, humanKB(bytes))
	if e.store.GetMeta("last_sync") != "" {
		fmt.Fprintf(w, "  %-13s %s\n", "last sync", e.store.GetMeta("last_sync"))
	}
	fmt.Fprintf(w, "  %-13s %s\n", "pricing", e.price.Source())
	return 0
}

func (e *env) hdr(s string) string {
	if e.args.IsTTY {
		return "  \x1b[1;97m" + s + "\x1b[0m"
	}
	return "  " + s
}

func (e *env) dim(s string) string {
	if e.args.IsTTY {
		return "\x1b[90m" + s + "\x1b[0m"
	}
	return s
}

// compactPath shortens the home directory to ~.
func (e *env) compactPath(p string) string {
	home := e.paths.Home
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + filepath.ToSlash(p[len(home):])
	}
	return filepath.ToSlash(p)
}

func humanKB(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// expandGlob is used by tests to confirm patterns resolve.
func (e *env) expandGlob(pattern string) []string {
	return globx.Expand(pattern, e.paths.Home)
}
