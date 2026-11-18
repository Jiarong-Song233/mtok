// Package render draws the usage table. Every line is measured in terminal
// columns (CJK counts as 2) and clipped so a narrow window never soft-wraps.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Jiarong-Song233/mtok/internal/quota"
)

// ANSI helpers -------------------------------------------------------------

const (
	reset = "\x1b[0m"
)

// JSONSchemaVersion is bumped whenever the machine-readable shape changes.
const JSONSchemaVersion = "1.0"

// Painter applies colour only when the output is a terminal.
type Painter struct{ On bool }

func (p Painter) paint(code, s string) string {
	if !p.On || code == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + reset
}

func (p Painter) dim(s string) string     { return p.paint("90", s) }
func (p Painter) bold(s string) string    { return p.paint("1;97", s) }
func (p Painter) title(s string) string   { return p.paint("1;96", s) }
func (p Painter) green(s string) string   { return p.paint("32", s) }
func (p Painter) cyan(s string) string    { return p.paint("36", s) }
func (p Painter) magenta(s string) string { return p.paint("35", s) }
func (p Painter) yellow(s string) string  { return p.paint("1;33", s) }
func (p Painter) red(s string) string     { return p.paint("31", s) }
func (p Painter) model(s string) string   { return p.paint("96", s) }

// Width returns the terminal column width of s (East Asian wide = 2).
func Width(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

func runeWidth(r rune) int {
	if r == 0 {
		return 0
	}
	if r < 32 {
		return 0
	}
	if r < 0x1100 {
		return 1
	}
	switch {
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1F64F,
		r >= 0x1F900 && r <= 0x1F9FF,
		r >= 0x20000 && r <= 0x3FFFD:
		return 2
	}
	return 1
}

// Clip shortens s to at most w columns, using a trailing "~" as the marker.
func Clip(s string, w int) string {
	if Width(s) <= w {
		return s
	}
	if w <= 1 {
		return "~"
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := runeWidth(r)
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "~"
}

// PadR pads on the right to w columns.
func PadR(s string, w int) string {
	d := w - Width(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

// PadL pads on the left to w columns.
func PadL(s string, w int) string {
	d := w - Width(s)
	if d <= 0 {
		return s
	}
	return strings.Repeat(" ", d) + s
}

// FmtNum renders a token count compactly.
func FmtNum(n int64) string {
	f := float64(n)
	switch {
	case f >= 1e9:
		return trimZeros(fmt.Sprintf("%.2f", f/1e9)) + "B"
	case f >= 1e6:
		return trimZeros(fmt.Sprintf("%.2f", f/1e6)) + "M"
	case f >= 1e4:
		return trimZeros(fmt.Sprintf("%.1f", f/1e3)) + "k"
	case f >= 1e3:
		return trimZeros(fmt.Sprintf("%.2f", f/1e3)) + "k"
	default:
		return fmt.Sprintf("%d", n)
	}
}

func trimZeros(s string) string {
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// FmtMoney renders a currency amount compactly.
func FmtMoney(v float64) string {
	switch {
	case v >= 1e4:
		return trimZeros(fmt.Sprintf("%.1f", v/1e3)) + "k"
	case v >= 1e3:
		return fmt.Sprintf("%.0f", v)
	case v >= 100:
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}

// ModelRow is one aggregated line of the table.
type ModelRow struct {
	Name       string  `json:"model"`
	Agent      string  `json:"agent,omitempty"`
	Input      int64   `json:"input"`
	Output     int64   `json:"output"`
	CacheRead  int64   `json:"cache_read"`
	CacheWrite int64   `json:"cache_write"`
	Calls      int64   `json:"calls"`
	CostUSD    float64 `json:"cost_usd"`
	Priced     bool    `json:"priced"`
}

// Total returns all tokens of the row.
func (m ModelRow) Total() int64 { return m.Input + m.Output + m.CacheRead + m.CacheWrite }

// Quota is kept as an alias so existing callers keep compiling while quota
// data now carries its source and individual windows.
type Quota = quota.Quota

// BudgetPeriod is the spend against one configured limit.
type BudgetPeriod struct {
	Limit    float64 `json:"limit"`
	Spent    float64 `json:"spent"`
	Currency string  `json:"currency"`
	Exceeded bool    `json:"exceeded"`
}

// Budget is the result of `mtok --check` and the normal-report highlight.
type Budget struct {
	Currency string        `json:"currency"`
	Daily    *BudgetPeriod `json:"daily,omitempty"`
	Monthly  *BudgetPeriod `json:"monthly,omitempty"`
	OK       bool          `json:"ok"`
	Note     string        `json:"note,omitempty"`
}

// Exceeded reports whether any configured limit was crossed.
func (b *Budget) Exceeded() bool {
	return b != nil && ((b.Daily != nil && b.Daily.Exceeded) ||
		(b.Monthly != nil && b.Monthly.Exceeded))
}

// View is everything the renderer needs.
type View struct {
	Label      string
	Span       string
	Since      string
	Until      string
	Elapsed    time.Duration
	Now        time.Time
	Rows       []ModelRow
	ByAgent    bool
	Totals     ModelRow
	Unpriced   []string
	Rate       float64
	Currency   string
	Width      int
	Top        int
	Color      bool
	Quotas     []Quota
	QuotaNotes []string
	Budget     *Budget
	MagpieOK   bool
	MagpieErr  string
	Sync       string
	Note       string
}

// Render writes the human table.
func (v *View) Render(w io.Writer) {
	p := Painter{On: v.Color}
	width := v.Width
	if width < 40 {
		width = 40
	}
	inner := width - 2
	if inner < 30 {
		inner = 30
	}
	var lines []string
	add := func(s string) { lines = append(lines, s) }

	head := "  " + p.title("mtok") + p.bold("  "+v.Label+" "+v.Span)
	if v.Elapsed > 0 {
		head += p.dim(fmt.Sprintf("  %dms", v.Elapsed.Milliseconds()))
	}
	head += p.dim("  " + v.Now.Format("15:04"))
	add(head)
	if v.Sync != "" {
		add("  " + p.dim(Clip(v.Sync, inner)))
	}
	sep := "  " + p.dim(strings.Repeat("─", inner))
	add(sep)

	// Column budget: 2 margin + model + 5 token columns of 6 and a money
	// column of 7, plus separators.
	const (
		numW  = 6
		costW = 7
	)
	wM := inner - 5*(numW+1) - (costW + 1) - 2
	if wM < 12 {
		wM = 12
	}
	if wM > 34 {
		wM = 34
	}
	nameCol := "模型"
	if v.ByAgent {
		nameCol = "来源"
	}
	add("  " + p.bold(PadR(nameCol, wM)) + " " +
		p.dim(PadL("输入", numW)) + " " +
		p.dim(PadL("输出", numW)) + " " +
		p.dim(PadL("缓存读", numW)) + " " +
		p.dim(PadL("缓存写", numW)) + " " +
		p.dim(PadL("总和", numW)) + " " +
		p.dim(PadL(v.moneyLabel(), costW)))

	if len(v.Rows) == 0 {
		add("  " + p.dim("(该时间段无用量)"))
	} else {
		rows := v.Rows
		rest := ""
		if v.Top > 0 && len(rows) > v.Top {
			var tok int64
			for _, r := range rows[v.Top:] {
				tok += r.Total()
			}
			rest = fmt.Sprintf("…另有 %d 行  %s tok", len(rows)-v.Top, FmtNum(tok))
			rows = rows[:v.Top]
		}
		for _, r := range rows {
			cost := "—"
			switch {
			case r.Priced:
				cost = FmtMoney(r.CostUSD * v.rate())
			case r.CostUSD > 0:
				// Some component models have no price: show the priced part.
				cost = FmtMoney(r.CostUSD*v.rate()) + "*"
			}
			add("  " + p.model(PadR(Clip(r.Name, wM), wM)) + " " +
				p.green(PadL(FmtNum(r.Input), numW)) + " " +
				p.cyan(PadL(FmtNum(r.Output), numW)) + " " +
				p.magenta(PadL(FmtNum(r.CacheRead), numW)) + " " +
				p.magenta(PadL(FmtNum(r.CacheWrite), numW)) + " " +
				p.green(PadL(FmtNum(r.Total()), numW)) + " " +
				p.yellow(PadL(cost, costW)))
		}
		if rest != "" {
			add("  " + p.dim(rest))
		}
	}
	add(sep)
	t := v.Totals
	add("  " + p.bold(PadR("合计", wM)) + " " +
		p.green(PadL(FmtNum(t.Input), numW)) + " " +
		p.cyan(PadL(FmtNum(t.Output), numW)) + " " +
		p.magenta(PadL(FmtNum(t.CacheRead), numW)) + " " +
		p.magenta(PadL(FmtNum(t.CacheWrite), numW)) + " " +
		p.green(PadL(FmtNum(t.Total()), numW)) + " " +
		p.yellow(PadL(FmtMoney(t.CostUSD*v.rate()), costW)))

	sym := v.Currency
	if sym == "" {
		sym = "$"
	}
	if v.isUSD() {
		add("  " + p.dim(sym+" 为估算值，非账单/非实际结算"))
	} else {
		add("  " + p.dim(fmt.Sprintf("%s 为估算值（汇率 %.2f），非账单/非实际结算", sym, v.rate())))
	}
	if v.hasPartialCost() {
		add("  " + p.dim("* = 该行含未定价模型，仅为已定价部分"))
	}
	if v.Note != "" {
		add("  " + p.dim(Clip(v.Note, inner)))
	}
	if len(v.Unpriced) > 0 {
		u := v.Unpriced
		txt := strings.Join(u, " ")
		if len(u) > 3 {
			txt = strings.Join(u[:3], " ") + fmt.Sprintf(" 等 %d 个", len(u))
		}
		add("  " + p.dim("未定价: "+Clip(txt, inner-8)))
	}
	if v.Budget != nil {
		if line := budgetLine(v.Budget, sym); line != "" {
			if v.Budget.Exceeded() {
				add("  " + p.red(Clip(line, inner)))
			} else {
				add("  " + p.dim(Clip(line, inner)))
			}
		}
		if v.Budget.Note != "" {
			add("  " + p.dim(Clip(v.Budget.Note, inner)))
		}
	}
	if len(v.Quotas) > 0 {
		add(sep)
		add("  " + p.bold("剩余额度"))
		slot := 27
		perRow := inner / slot
		if perRow < 1 {
			perRow = 1
		}
		var buf strings.Builder
		n := 0
		for _, q := range v.Quotas {
			col := "31"
			if q.Remaining >= 50 {
				col = "32"
			} else if q.Remaining >= 20 {
				col = "33"
			}
			buf.WriteString(p.paint(col, PadR(Clip(quotaCell(q), slot), slot)))
			n++
			if n >= perRow {
				add("  " + buf.String())
				buf.Reset()
				n = 0
			}
		}
		if n > 0 {
			add("  " + buf.String())
		}
		for _, note := range v.QuotaNotes {
			add("  " + p.dim(Clip(note, inner)))
		}
		add("  " + p.dim("Claude / Gemini 额度看各自 CLI 的 /status"))
	} else if len(v.QuotaNotes) > 0 {
		add(sep)
		for _, note := range v.QuotaNotes {
			add("  " + p.dim(Clip(note, inner)))
		}
	} else if v.MagpieErr != "" {
		add("  " + p.dim("magpie 离线（"+Clip(v.MagpieErr, inner-14)+"）"))
	}

	for _, l := range lines {
		if Width(stripAnsi(l)) > width {
			l = Clip(stripAnsi(l), width)
		}
		fmt.Fprintln(w, l)
	}
}

// hasPartialCost reports whether any row mixes priced and unpriced models.
func (v *View) hasPartialCost() bool {
	for _, r := range v.Rows {
		if !r.Priced && r.CostUSD > 0 {
			return true
		}
	}
	return false
}

func quotaCell(q Quota) string {
	tag := ""
	switch q.Source {
	case quota.SourceCodex:
		tag = "本地"
	case quota.SourceMagpie:
		tag = "magpie"
	}
	cell := q.Provider
	if tag != "" {
		cell += "(" + tag + ")"
	}
	if q.Detail != "" {
		return cell + " " + q.Detail
	}
	return cell + " " + fmt.Sprintf("%.0f%%", q.Remaining)
}

func budgetLine(b *Budget, sym string) string {
	parts := make([]string, 0, 2)
	add := func(label string, p *BudgetPeriod) {
		if p == nil {
			return
		}
		s := fmt.Sprintf("预算%s %s%s/%s%s", label, sym, FmtMoney(p.Spent), sym, FmtMoney(p.Limit))
		if p.Exceeded {
			s += " 超限"
		}
		parts = append(parts, s)
	}
	add("今日", b.Daily)
	add("本月", b.Monthly)
	return strings.Join(parts, "；")
}

func quotaColor(remaining float64) string {
	if remaining >= 50 {
		return "32"
	}
	if remaining >= 20 {
		return "33"
	}
	return "31"
}

func resetText(q Quota, now time.Time) string {
	var at int64
	for _, w := range q.Windows {
		if w.ResetsAt > now.Unix() && (at == 0 || w.ResetsAt < at) {
			at = w.ResetsAt
		}
	}
	if at == 0 {
		return ""
	}
	t := time.Unix(at, 0).In(now.Location())
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	return t.Format("01-02 15:04")
}

// RenderQuotas writes the standalone `mtok quota` view.
func (v *View) RenderQuotas(w io.Writer) {
	p := Painter{On: v.Color}
	width := v.Width
	if width < 40 {
		width = 40
	}
	inner := width - 2
	if inner < 30 {
		inner = 30
	}
	var lines []string
	add := func(s string) { lines = append(lines, s) }
	head := "  " + p.title("mtok 额度") + p.bold("  "+v.Now.Format("2006-01-02 15:04"))
	add(head)
	add("  " + p.dim(strings.Repeat("─", inner)))
	if len(v.Quotas) == 0 {
		add("  " + p.dim("没有可用的额度数据"))
	}
	for _, q := range v.Quotas {
		label := q.Provider
		if q.Name != "" {
			label += " · " + q.Name
		}
		if q.Source != "" {
			label += " [" + q.Source + "]"
		}
		add("  " + p.bold(Clip(label, inner)))
		detail := q.Detail
		if detail == "" {
			detail = fmt.Sprintf("%.0f%%", q.Remaining)
		}
		line := "    剩余 " + detail
		if reset := resetText(q, v.Now); reset != "" {
			line += " · 重置 " + reset
		}
		if t, err := time.Parse(time.RFC3339, q.ObservedAt); err == nil {
			line += " · 观测 " + t.In(v.Now.Location()).Format("01-02 15:04")
		}
		add("  " + p.paint(quotaColor(q.Remaining), Clip(line, inner)))
	}
	for _, note := range v.QuotaNotes {
		add("  " + p.dim(Clip(note, inner)))
	}
	add("  " + p.dim("Claude / Gemini：本地无额度数据，用各自 CLI 的 /status 查看"))
	for _, l := range lines {
		if Width(stripAnsi(l)) > width {
			l = Clip(stripAnsi(l), width)
		}
		fmt.Fprintln(w, l)
	}
}

// SchemaJSON prints the stable machine-readable contract for scripts.
func SchemaJSON(w io.Writer) error {
	doc := struct {
		SchemaVersion string         `json:"schema_version"`
		Commands      []string       `json:"commands"`
		Report        []string       `json:"report_fields"`
		Quota         []string       `json:"quota_fields"`
		Budget        []string       `json:"budget_fields"`
		Sources       []string       `json:"sources"`
		ExitCodes     map[string]int `json:"exit_codes"`
	}{
		SchemaVersion: JSONSchemaVersion,
		Commands:      []string{"report", "quota", "doctor", "adapters", "config", "pricing", "schema", "version"},
		Report:        []string{"schema_version", "range", "since", "until", "totals", "group_by", "rows", "cost", "quotas", "budget", "meta"},
		Quota:         []string{"provider", "name", "remaining", "detail", "source", "observed_at", "windows"},
		Budget:        []string{"currency", "daily", "monthly", "ok", "note"},
		Sources:       []string{quota.SourceCodex, quota.SourceMagpie},
		ExitCodes:     map[string]int{"ok": 0, "usage": 2, "runtime": 1, "budget_exceeded": 3},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// QuotaJSON is the machine-readable shape of `mtok quota --json`.
type QuotaJSON struct {
	SchemaVersion string   `json:"schema_version"`
	Quotas        []Quota  `json:"quotas"`
	Notes         []string `json:"notes,omitempty"`
	Generated     string   `json:"generated"`
}

// RenderQuotaJSON writes the standalone quota report.
func RenderQuotaJSON(w io.Writer, quotas []Quota, notes []string, now time.Time) error {
	if quotas == nil {
		quotas = []Quota{}
	}
	out := QuotaJSON{
		SchemaVersion: JSONSchemaVersion,
		Quotas:        quotas,
		Notes:         notes,
		Generated:     now.Format(time.RFC3339),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// moneyLabel is the currency symbol shown in the header column.
func (v *View) moneyLabel() string {
	if v.isUSD() {
		return "$"
	}
	return "¥"
}

func (v *View) isUSD() bool {
	return strings.EqualFold(v.Currency, "USD") || v.Currency == "$"
}

// rate converts USD prices into the displayed currency.
func (v *View) rate() float64 {
	if v.isUSD() {
		return 1
	}
	if v.Rate <= 0 {
		return 7.2
	}
	return v.Rate
}

// stripAnsi removes escape sequences so width math sees only glyphs.
func stripAnsi(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	in := false
	for _, r := range s {
		if r == 0x1b {
			in = true
			continue
		}
		if in {
			if r == 'm' {
				in = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// JSON is the machine-readable shape of a report.
type JSON struct {
	SchemaVersion string     `json:"schema_version"`
	Range         string     `json:"range"`
	Since         string     `json:"since"`
	Until         string     `json:"until"`
	Totals        ModelRow   `json:"totals"`
	By            string     `json:"group_by"`
	Rows          []ModelRow `json:"rows"`
	Cost          JSONCost   `json:"cost"`
	Quotas        []Quota    `json:"quotas,omitempty"`
	Budget        *Budget    `json:"budget,omitempty"`
	Meta          JSONMeta   `json:"meta"`
}

// JSONCost states how the reference price was produced.
type JSONCost struct {
	Currency string   `json:"currency"`
	Rate     float64  `json:"rate"`
	Unpriced []string `json:"unpriced,omitempty"`
	Note     string   `json:"note"`
}

// JSONMeta carries provenance for scripts.
type JSONMeta struct {
	Version   string `json:"version"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Generated string `json:"generated"`
	Sync      string `json:"sync,omitempty"`
}

// RenderJSON writes the JSON report.
func (v *View) RenderJSON(w io.Writer, version string) error {
	rows := v.Rows
	if rows == nil {
		rows = []ModelRow{}
	}
	unp := append([]string{}, v.Unpriced...)
	sort.Strings(unp)
	group := "model"
	if v.ByAgent {
		group = "agent"
	}
	out := JSON{
		SchemaVersion: JSONSchemaVersion,
		Range:         v.Label,
		Since:         v.Since,
		Until:         v.Until,
		Totals:        v.Totals,
		By:            group,
		Rows:          rows,
		Cost: JSONCost{
			Currency: v.moneyLabel(),
			Rate:     v.rate(),
			Unpriced: unp,
			Note:     "reference price, not a bill",
		},
		Quotas: v.Quotas,
		Budget: v.Budget,
		Meta: JSONMeta{
			Version:   version,
			ElapsedMS: v.Elapsed.Milliseconds(),
			Generated: v.Now.Format(time.RFC3339),
			Sync:      v.Sync,
		},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
