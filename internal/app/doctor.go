package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/songjiarong/mtok/internal/adapters"
	"github.com/songjiarong/mtok/internal/agg"
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
			lastErr: s.LastErr, notime: s.NoTime, skipped: int64(s.Skipped), elapsedMS: s.ElapsedMS, rotated: s.Rotated,
			globs: s.Globs,
		}
	}
	fmt.Fprintln(w, e.hdr("ADAPTER      状态      文件   增  记录/本次  耗时   本次扫描模型"))
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
		if st.rotated > 0 {
			fmt.Fprintf(w, "      %s %d 个日志被截断重写，历史贡献无法撤回，建议 mtok --rebuild\n", e.dim("!"), st.rotated)
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

	// Cross-check the journal reader against the proxy's own aggregation. The
	// journal is what mtok uses for proxy-side detail, so these two must agree
	// field by field.
	if e.set.Magpie && !o.noMagpie {
		fmt.Fprintln(w, e.hdr("magpie 明细核对（journal vs /v1/magpie/usage）"))
		c := magpie.New(e.set.MagpieURL)
		windows := []struct {
			label  string
			period string
			days   int
		}{
			{"today", "today", 1},
			{"7d", "7d", 7},
			{"30d", "30d", 30},
		}
		fmt.Fprintf(w, "  %-6s %-12s %-12s %-12s %-12s\n", "", "input", "output", "cache_read", "calls")
		for _, win := range windows {
			since := time.Now().AddDate(0, 0, -(win.days - 1)).Format("2006-01-02")
			got := e.magpieSum(since)
			u, err := c.Usage(win.period)
			if err != nil {
				fmt.Fprintf(w, "  %-6s %s %v\n", win.label, e.dim("magpie 不可用:"), trimErr(err))
				continue
			}
			api := u.SumModels()
			fmt.Fprintf(w, "  %-6s %-12d %-12d %-12d %-12d  %s\n", win.label, got.input, got.output, got.cacheRead, got.calls, e.dim("mtok journal"))
			fmt.Fprintf(w, "  %-6s %-12d %-12d %-12d %-12d  %s\n", " api", api.Input, api.Output, api.CacheRead, api.Calls, e.dim("models[] 求和"))
			fmt.Fprintf(w, "  %-6s %s\n", "", e.dim("差值  "+diffPct(got.input, api.Input)+" / "+
				diffPct(got.output, api.Output)+" / "+diffPct(got.cacheRead, api.CacheRead)))
		}
		fmt.Fprintln(w, "  "+e.dim("mtok 用 served（真实底层模型）分组，magpie 的 models[] 用 model 分组：模型行名可能不同，总量逐字段一致。"))
		fmt.Fprintln(w, "  "+e.dim("calls 差值来自 0-token 记录（错误/取消）：mtok 不计空记录，magpie 仍计次；当天仍在写入时个位数漂移属正常。"))
		fmt.Fprintln(w)

		// How the current window splits between proxy detail and direct logs.
		sp, _ := resolveSpan(o, time.Now())
		res := agg.Build(e.store.Rows(), agg.Options{Since: sp.since, Until: sp.until,
			Price: func(string) (float64, float64, float64, float64, bool) { return 0, 0, 0, 0, false }})
		fmt.Fprintln(w, e.hdr("用量构成（当前视图）"))
		fmt.Fprintf(w, "  %-14s %d 条 / %s tok\n", "magpie 明细", res.MagpieRows, fnum(res.MagpieTokens))
		fmt.Fprintf(w, "  %-14s %s tok\n", "直连日志", fnum(res.DirectTokens))
		if res.RoutedDropped > 0 {
			fmt.Fprintf(w, "  %-14s %d 条 / %s tok（已由明细替代）\n", "route 记录", res.RoutedDropped, fnum(res.RoutedDroppedTokens))
		}
		if res.RoutedKept > 0 {
			fmt.Fprintf(w, "  %-14s %d 条 / %s tok（明细未覆盖，保留为 Auto）\n", "route 残留", res.RoutedKept, fnum(res.RoutedKeptTokens))
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
	rotated                           int
	_                                 struct{}
	bytes                             int64
	models                            []string
	lastErr                           string
	globs                             []string
}

type totals struct {
	input, output, cacheRead, cacheWrite, calls int64
}

// magpieSum totals journal rows from since (inclusive) to today.
func (e *env) magpieSum(since string) totals {
	var t totals
	for _, r := range e.store.Rows() {
		if !r.FromMagpie() || r.Day < since {
			continue
		}
		t.input += r.Input
		t.output += r.Output
		t.cacheRead += r.CacheRead
		t.cacheWrite += r.CacheWrite
		t.calls += r.Calls
	}
	return t
}

// fnum is render.FmtNum for compact token counts.
func fnum(n int64) string { return render.FmtNum(n) }

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
	fmt.Fprintf(w, "  %-13s daily=%g monthly=%g (0 = unset)\n",
		"budget", e.set.BudgetDaily, e.set.BudgetMonthly)
	for _, a := range e.adapters {
		if m, ok := a.(*adapters.Magpie); ok {
			state := e.dim("  [absent]")
			if _, err := os.Stat(m.Path()); err == nil {
				state = ""
			}
			fmt.Fprintf(w, "  %-13s %s%s\n", "magpie usage", e.compactPath(m.Path()), state)
		}
	}
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
