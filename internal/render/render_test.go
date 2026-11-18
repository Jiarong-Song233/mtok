package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Jiarong-Song233/mtok/internal/quota"
)

func TestWidthCountsCJKAsTwo(t *testing.T) {
	cases := map[string]int{
		"abc":    3,
		"模型":     4,
		"模型 abc": 8,
		"─":      1,
		"¥":      1,
		"Claude": 6,
	}
	for s, want := range cases {
		if got := Width(s); got != want {
			t.Errorf("Width(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestClipNeverExceedsWidth(t *testing.T) {
	s := "Claude-Opus-5.5-超长模型名称测试"
	for w := 4; w <= 30; w++ {
		got := Clip(s, w)
		if Width(got) > w {
			t.Errorf("Clip(%q, %d) = %q (%d cols)", s, w, got, Width(got))
		}
	}
}

func TestPad(t *testing.T) {
	if got := PadR("模型", 8); Width(got) != 8 {
		t.Errorf("PadR width %d", Width(got))
	}
	if got := PadL("1.2k", 7); got != "   1.2k" {
		t.Errorf("PadL = %q", got)
	}
}

func TestFmtNum(t *testing.T) {
	cases := map[int64]string{
		999: "999", 1500: "1.5k", 15000: "15k", 1250000: "1.25M", 3400000000: "3.4B",
	}
	for n, want := range cases {
		if got := FmtNum(n); got != want {
			t.Errorf("FmtNum(%d) = %q, want %q", n, got, want)
		}
	}
}

func sampleView() *View {
	return &View{
		Label: "今天", Span: "11-15", Since: "2026-11-15", Until: "2026-11-15",
		Now: time.Date(2026, 11, 15, 12, 0, 0, 0, time.Local),
		Rows: []ModelRow{
			{Name: "Claude-Opus-5.5", Input: 168400, Output: 14920000, CacheRead: 4_640_000_000, CostUSD: 9700, Priced: true},
			{Name: "一个非常非常长的模型名字-with-suffix", Input: 1000, Output: 200, CostUSD: 0.5, Priced: true},
		},
		Totals:   ModelRow{Name: "合计", Input: 169400, Output: 14920200, CacheRead: 4_640_000_000, CostUSD: 9700.5, Priced: true},
		Rate:     7.2,
		Currency: "CNY",
		Width:    64,
		Top:      8,
		Quotas:   []Quota{{Provider: "codex", Remaining: 74}, {Provider: "grok", Remaining: 100}},
	}
}

func TestRenderRespectsWidth(t *testing.T) {
	for _, w := range []int{56, 64, 80} {
		var buf bytes.Buffer
		v := sampleView()
		v.Width = w
		v.Render(&buf)
		for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
			if got := Width(line); got > w {
				t.Errorf("width %d: line %q is %d columns", w, line, got)
			}
		}
	}
}

func TestRenderHasTotalColumn(t *testing.T) {
	var buf bytes.Buffer
	sampleView().Render(&buf)
	s := buf.String()
	if !strings.Contains(s, "缓存读") || !strings.Contains(s, "缓存写") {
		t.Fatalf("separate cache columns missing:\n%s", s)
	}
	if !strings.Contains(s, "总和") {
		t.Fatalf("total column header missing:\n%s", s)
	}
	if !strings.Contains(s, "4.66B") {
		t.Fatalf("total token count missing:\n%s", s)
	}
}

func TestRenderHasBillingDisclaimer(t *testing.T) {
	var buf bytes.Buffer
	sampleView().Render(&buf)
	if !strings.Contains(buf.String(), "非账单") {
		t.Fatal("missing not-a-bill disclaimer")
	}
	if !strings.Contains(buf.String(), "剩余额度") {
		t.Fatal("missing quota block")
	}
}

func TestRenderTopTruncatesRows(t *testing.T) {
	var buf bytes.Buffer
	v := sampleView()
	v.Top = 1
	v.Render(&buf)
	if !strings.Contains(buf.String(), "另有 1 行") {
		t.Fatalf("truncation note missing:\n%s", buf.String())
	}
}

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := sampleView().RenderJSON(&buf, "test"); err != nil {
		t.Fatal(err)
	}
	var got JSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(got.Rows) != 2 || got.By != "model" || got.Totals.Input != 169400 {
		t.Fatalf("unexpected json: %+v", got)
	}
	if got.SchemaVersion != JSONSchemaVersion {
		t.Fatalf("schema_version = %q", got.SchemaVersion)
	}
	if got.Cost.Note == "" {
		t.Fatal("cost note missing")
	}
}

func TestRenderJSONIncludesBudget(t *testing.T) {
	var buf bytes.Buffer
	v := sampleView()
	v.Budget = &Budget{
		Currency: "CNY",
		OK:       false,
		Daily:    &BudgetPeriod{Limit: 10, Spent: 12, Currency: "CNY", Exceeded: true},
	}
	if err := v.RenderJSON(&buf, "test"); err != nil {
		t.Fatal(err)
	}
	var got JSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Budget == nil || got.Budget.Daily == nil || !got.Budget.Daily.Exceeded || got.Budget.OK {
		t.Fatalf("unexpected budget json: %+v", got.Budget)
	}
}

func TestSchemaJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := SchemaJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var got struct {
		SchemaVersion string         `json:"schema_version"`
		ExitCodes     map[string]int `json:"exit_codes"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != JSONSchemaVersion || got.ExitCodes["budget_exceeded"] != 3 {
		t.Fatalf("unexpected schema: %+v", got)
	}
}

func TestRenderQuotaJSONIncludesSource(t *testing.T) {
	var buf bytes.Buffer
	qs := []Quota{{
		Provider:   "codex",
		Name:       "Codex Plus",
		Remaining:  16,
		Detail:     "5h16% 7d32%",
		Source:     quota.SourceCodex,
		ObservedAt: "2026-11-17T11:33:48Z",
		Windows: []quota.Window{{
			Label: "5h", UsedPercent: 84, Remaining: 16,
			WindowMinutes: 300, ResetsAt: 1794921702,
		}},
	}}
	now := time.Date(2026, 11, 17, 11, 40, 0, 0, time.UTC)
	if err := RenderQuotaJSON(&buf, qs, []string{"note"}, now); err != nil {
		t.Fatal(err)
	}
	var got QuotaJSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != JSONSchemaVersion || len(got.Quotas) != 1 {
		t.Fatalf("unexpected quota json: %+v", got)
	}
	if got.Quotas[0].Source != quota.SourceCodex || len(got.Quotas[0].Windows) != 1 {
		t.Fatalf("missing source/windows: %+v", got.Quotas[0])
	}
}

func TestRenderQuotasShowsReset(t *testing.T) {
	var buf bytes.Buffer
	v := &View{
		Now:   time.Date(2026, 11, 17, 11, 40, 0, 0, time.UTC),
		Width: 64,
		Quotas: []Quota{{
			Provider:  "codex",
			Name:      "Codex Plus",
			Remaining: 16,
			Detail:    "5h16% 7d32%",
			Source:    quota.SourceCodex,
			Windows:   []quota.Window{{Label: "5h", Remaining: 16, ResetsAt: 1794921702}},
		}},
	}
	v.RenderQuotas(&buf)
	s := buf.String()
	if !strings.Contains(s, "Codex Plus") || !strings.Contains(s, "5h16% 7d32%") || !strings.Contains(s, "重置") {
		t.Fatalf("quota view missing fields:\n%s", s)
	}
}

func TestNoColorWhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	v := sampleView()
	v.Color = false
	v.Render(&buf)
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("ansi escapes leaked into plain output")
	}
}
