package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/songjiarong/mtok/internal/adapters"
	"github.com/songjiarong/mtok/internal/config"
	"github.com/songjiarong/mtok/internal/ingest"
	"github.com/songjiarong/mtok/internal/magpie"
	"github.com/songjiarong/mtok/internal/model"
	"github.com/songjiarong/mtok/internal/pricing"
	"github.com/songjiarong/mtok/internal/render"
	"github.com/songjiarong/mtok/internal/store"
)

// IsTerminal reports whether w is an interactive console.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// env bundles everything a command needs after loading.
type env struct {
	args     Args
	paths    config.Paths
	set      config.Settings
	norm     *model.Normalizer
	price    *pricing.Table
	store    *store.Store
	adapters []adapters.Adapter
	declErrs []error
	sync     ingest.Result
	synced   bool
	queryMS  int64
}

// Run executes the CLI and returns the process exit code.
func Run(a Args) int {
	o, err := parse(a.Argv)
	if err != nil {
		fmt.Fprintln(a.Stderr, "mtok: "+err.Error())
		fmt.Fprintln(a.Stderr, "try `mtok help`")
		return 2
	}
	if o.help || o.cmd == "help" {
		usage(a.Stdout, a.Version)
		return 0
	}
	paths, err := config.Resolve()
	if err != nil {
		fmt.Fprintln(a.Stderr, "mtok: "+err.Error())
		return 1
	}
	if o.cmd == "version" {
		fmt.Fprintf(a.Stdout, "mtok %s (%s, %s, commit %s, built %s)\n",
			a.Version, a.Platform, a.GoVersion, a.Commit, a.Date)
		return 0
	}
	if o.cmd == "config" && o.initCfg {
		if err := writeExamples(paths); err != nil {
			fmt.Fprintln(a.Stderr, "mtok: "+err.Error())
			return 1
		}
		fmt.Fprintln(a.Stdout, "wrote default config to "+paths.ConfigDir)
		return 0
	}

	e, err := load(a, paths)
	if err != nil {
		fmt.Fprintln(a.Stderr, "mtok: "+err.Error())
		return 1
	}
	defer e.store.Close()

	switch o.cmd {
	case "doctor":
		return e.doctor(o)
	case "adapters":
		return e.listAdapters(o)
	case "config":
		return e.showConfig(o)
	case "version":
		fmt.Fprintf(a.Stdout, "mtok %s\n", a.Version)
		return 0
	}
	if o.since != "" || o.until != "" {
		o.rangeArg = ""
	}
	return e.report(o)
}

// load resolves paths, config, aliases, prices, adapters and the store.
func load(a Args, paths config.Paths) (*env, error) {
	set, _, err := config.Load(paths)
	if err != nil {
		return nil, err
	}
	norm := model.New()
	userAliases, err := config.Aliases(paths)
	if err != nil {
		return nil, err
	}
	for raw, canonical := range userAliases {
		norm.Add(canonical, raw)
	}
	price, priceErr := pricing.Load(set.Pricing, set.Rate)
	if priceErr != nil && price == nil {
		return nil, priceErr
	}
	st, err := store.Open(paths.DB)
	if err != nil {
		return nil, err
	}
	ads := adapters.Builtins(paths.Home)
	decl, declErrs := adapters.LoadDir(paths.Adaptersd, paths.Home)
	ads = append(ads, decl...)
	return &env{
		args: a, paths: paths, set: set, norm: norm, price: price,
		store: st, adapters: ads, declErrs: declErrs,
	}, nil
}

// sync performs the incremental ingest once per process.
func (e *env) syncNow(o options) error {
	if e.synced {
		return nil
	}
	res, err := ingest.Run(e.store, e.adapters, e.norm, ingest.Options{
		Home:     e.paths.Home,
		Zstd:     e.set.Zstd,
		Rebuild:  o.rebuild,
		Disabled: e.set.Disabled,
	})
	e.sync = res
	e.synced = true
	if err != nil {
		return err
	}
	return e.store.Save()
}

// span is the resolved reporting window.
type span struct {
	since, until string
	label        string
}

// resolveSpan maps the CLI arguments onto ISO day bounds.
func resolveSpan(o options, now time.Time) (span, error) {
	day := func(t time.Time) string { return t.Format("2006-01-02") }
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	s := span{since: day(today), until: day(today), label: "今天"}
	r := strings.ToLower(strings.TrimSpace(o.rangeArg))
	switch {
	case r == "", r == "today", r == "t", r == "d", r == "day":
	case r == "week", r == "w":
		s.since, s.label = day(today.AddDate(0, 0, -6)), "近 7 天"
	case r == "month", r == "m", r == "mo":
		s.since, s.label = day(time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, now.Location())), "本月"
	case r == "all", r == "a", r == "total":
		s.since, s.label = "0000-01-01", "全部"
	case r == "help":
		return s, errors.New("`mtok help` shows the usage text")
	default:
		if n, ok := parseDays(r); ok {
			if n < 1 {
				n = 1
			}
			s.since, s.label = day(today.AddDate(0, 0, -(n-1))), fmt.Sprintf("近 %d 天", n)
			break
		}
		if t, err := time.ParseInLocation("2006-01-02", r, now.Location()); err == nil {
			s.since, s.until, s.label = day(t), day(t), "指定日"
			break
		}
		return s, fmt.Errorf("无法识别的时间范围 %q（today / 7d / 30d / month / all / 2026-11-15）", o.rangeArg)
	}
	if o.since != "" {
		t, err := time.ParseInLocation("2006-01-02", o.since, now.Location())
		if err != nil {
			return s, fmt.Errorf("--since 需要 YYYY-MM-DD: %q", o.since)
		}
		s.since, s.label = day(t), "自定义"
	}
	if o.until != "" {
		t, err := time.ParseInLocation("2006-01-02", o.until, now.Location())
		if err != nil {
			return s, fmt.Errorf("--until 需要 YYYY-MM-DD: %q", o.until)
		}
		s.until, s.label = day(t), "自定义"
	}
	if s.since > s.until {
		s.since, s.until = s.until, s.since
	}
	return s, nil
}

// parseDays accepts 7, 7d, 7days.
func parseDays(s string) (int, bool) {
	s = strings.TrimSuffix(strings.TrimSuffix(s, "days"), "d")
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// report is the default command: sync, aggregate, render.
func (e *env) report(o options) int {
	now := time.Now()
	sp, err := resolveSpan(o, now)
	if err != nil {
		fmt.Fprintln(e.args.Stderr, "mtok: "+err.Error())
		return 2
	}
	start := time.Now()
	if !o.noSync {
		if err := e.syncNow(o); err != nil {
			fmt.Fprintln(e.args.Stderr, "mtok: 采集失败: "+err.Error())
		}
	}
	elapsed := time.Since(start)

	aggStart := time.Now()
	view := e.buildView(o, sp, now)
	view.Elapsed = elapsed + time.Since(aggStart)
	e.queryMS = time.Since(aggStart).Milliseconds()
	view.Sync = e.syncLine(o)

	if !o.noMagpie && e.set.Magpie {
		c := magpie.New(e.set.MagpieURL)
		if qs, err := c.Quotas(); err == nil {
			for _, q := range qs {
				view.Quotas = append(view.Quotas, render.Quota{Provider: q.Provider, Name: q.Name, Remaining: q.Remaining, Detail: quotaDetail(q)})
			}
			view.MagpieOK = true
		} else {
			view.MagpieErr = trimErr(err)
		}
	}
	if o.usd {
		view.Currency = "USD"
	}
	if o.json {
		view.Color = false
		if err := view.RenderJSON(e.args.Stdout, e.args.Version); err != nil {
			fmt.Fprintln(e.args.Stderr, "mtok: "+err.Error())
			return 1
		}
		return 0
	}
	view.Color = !o.noColor && e.args.IsTTY && os.Getenv("NO_COLOR") == ""
	view.Render(e.args.Stdout)
	return 0
}

// buildView aggregates store rows into the render model.
func (e *env) buildView(o options, sp span, now time.Time) *render.View {
	rows := e.store.Rows()
	type bucket struct {
		name string
		row  render.ModelRow
	}
	buckets := map[string]*bucket{}
	for _, r := range rows {
		if r.Day < sp.since || r.Day > sp.until {
			continue
		}
		key := r.Model
		if o.byAgent {
			key = r.Agent
		}
		b, ok := buckets[key]
		if !ok {
			b = &bucket{name: key, row: render.ModelRow{Name: key, Priced: true}}
			if o.byAgent {
				b.row.Agent = key
			}
			buckets[key] = b
		}
		b.row.Input += r.Input
		b.row.Output += r.Output
		b.row.CacheRead += r.CacheRead
		b.row.CacheWrite += r.CacheWrite
		b.row.Calls += r.Calls
		if p, ok := e.price.Lookup(r.Model); ok {
			b.row.CostUSD += p.USD(r.Input, r.Output, r.CacheRead, r.CacheWrite)
		} else {
			b.row.Priced = false
		}
	}
	out := make([]render.ModelRow, 0, len(buckets))
	totals := render.ModelRow{Name: "合计", Priced: true}
	unpriced := map[string]bool{}
	for _, b := range buckets {
		out = append(out, b.row)
		totals.Input += b.row.Input
		totals.Output += b.row.Output
		totals.CacheRead += b.row.CacheRead
		totals.CacheWrite += b.row.CacheWrite
		totals.Calls += b.row.Calls
		totals.CostUSD += b.row.CostUSD
		if !b.row.Priced {
			unpriced[b.row.Name] = true
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total() != out[j].Total() {
			return out[i].Total() > out[j].Total()
		}
		return out[i].Name < out[j].Name
	})
	var unp []string
	for n := range unpriced {
		unp = append(unp, n)
	}
	sort.Strings(unp)

	width := e.set.Width
	if o.width > 0 {
		width = o.width
	}
	top := e.set.Top
	if o.top > 0 {
		top = o.top
	}
	cur := e.set.Currency
	if cur == "" {
		cur = "CNY"
	}
	v := &render.View{
		Label:    sp.label,
		Span:     spanLabel(sp, rows),
		Since:    sp.since,
		Until:    sp.until,
		Now:      now,
		Rows:     out,
		ByAgent:  o.byAgent,
		Totals:   totals,
		Unpriced: unp,
		Rate:     e.set.Rate,
		Currency: cur,
		Width:    width,
		Top:      top,
	}
	if len(e.declErrs) > 0 {
		v.Note = fmt.Sprintf("%d 个声明式 adapter 加载失败，运行 mtok adapters 查看", len(e.declErrs))
	}
	return v
}

// syncLine summarises the incremental scan in one short line.
func (e *env) syncLine(o options) string {
	q := fmt.Sprintf("查询 %dms", e.queryMS)
	if o.noSync {
		return "本地聚合库（未扫描新日志） / " + q
	}
	r := e.sync
	if r.Changed == 0 {
		return fmt.Sprintf("扫描 %d 文件，无新增 / %dms / %s", r.Files, r.Elapsed.Milliseconds(), q)
	}
	return fmt.Sprintf("扫描 %d 文件 +%d；新增 %d 记录 / %s / %dms / %s",
		r.Files, r.Changed, r.Records, ingest.HumanBytes(r.Bytes), r.Elapsed.Milliseconds(), q)
}

// spanLabel renders the header window, preferring the real data range for "all".
func spanLabel(sp span, rows []store.Row) string {
	since := sp.since
	if sp.since == "0000-01-01" {
		for _, r := range rows {
			if r.Day > "0000-01-01" && (since == "0000-01-01" || r.Day < since) {
				since = r.Day
			}
		}
		if since == "0000-01-01" {
			return "—"
		}
	}
	f := func(d string) string {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return d
		}
		return t.Format("01-02")
	}
	if since == sp.until {
		return f(since)
	}
	if len(since) >= 4 && len(sp.until) >= 4 && since[:4] != sp.until[:4] {
		return since + "~" + sp.until
	}
	return f(since) + "~" + f(sp.until)
}

// quotaDetail renders the per-provider detail (codex shows both windows) and
// notes when several accounts were merged.
func quotaDetail(q magpie.Quota) string {
	d := q.Detail
	if n := q.Accounts(); n > 1 {
		if d != "" {
			return d + fmt.Sprintf("×%d", n)
		}
		return fmt.Sprintf("%.0f%%×%d", q.Remaining, n)
	}
	return d
}

// trimErr shortens a network error to its last cause for the one-line footer.
func trimErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 && i+2 < len(s) {
		return s[i+2:]
	}
	return s
}

// usage prints the help text.
func usage(w io.Writer, version string) {
	lines := []string{
		"mtok " + version + " — 极轻量全局 token 用量检测与计费管理",
		"",
		"  mtok                 今天",
		"  mtok 7d | 7          近 7 天",
		"  mtok 30d             近 30 天",
		"  mtok month           本月",
		"  mtok all             全部历史",
		"  mtok --since 2026-11-01 --until 2026-11-15",
		"",
		"  选项",
		"  --json, -j           机器可读输出",
		"  --by-agent, -a       按来源汇总，而不是按统一模型合并",
		"  --width, -w N        最大行宽（默认 64）",
		"  --top, -n N          表格最多显示多少行（默认 8）",
		"  --usd                以美元显示参考价",
		"  --no-magpie          不连接 magpie 查额度",
		"  --no-sync            跳过日志扫描，只查本地聚合库",
		"  --rebuild            清空聚合库后全量重建",
		"  --no-color           强制去色",
		"",
		"  命令",
		"  mtok doctor          各来源日志、字段与口径自检",
		"  mtok adapters        列出 adapter 与加载状态",
		"  mtok config          显示配置与数据路径（--init 生成默认配置）",
		"  mtok version         版本信息",
		"",
		"  数据全部来自本机日志，不上传；金额为参考价，非账单。",
	}
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
}

func writeExamples(p config.Paths) error {
	if err := os.MkdirAll(p.ConfigDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(p.Adaptersd, 0o755); err != nil {
		return err
	}
	files := []struct {
		path string
		body string
	}{
		{p.Config, config.ExampleConfig},
		{p.Aliases, config.ExampleAliases},
		{filepath.Join(p.Adaptersd, "example.yaml.txt"), config.ExampleAdapter},
	}
	for _, f := range files {
		if _, err := os.Stat(f.path); err == nil {
			continue
		}
		if err := os.WriteFile(f.path, []byte(f.body), 0o644); err != nil {
			return err
		}
	}
	if _, err := os.Stat(p.Pricing); os.IsNotExist(err) {
		if err := os.WriteFile(p.Pricing, pricing.ExampleJSON(), 0o644); err != nil {
			return err
		}
	}
	return nil
}
