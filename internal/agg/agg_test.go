package agg

import (
	"testing"

	"github.com/Jiarong-Song233/mtok/internal/store"
)

// prices used by the tests: every model costs 1 USD per million input token.
func flatPrice(model string) (float64, float64, float64, float64, bool) {
	if model == "Unpriced" {
		return 0, 0, 0, 0, false
	}
	return 1, 0, 0, 0, true
}

func local(day, model, agent string, in int64, routed bool) store.Row {
	return store.Row{Day: day, Model: model, Agent: agent, Input: in, Calls: 1, Routed: routed}
}

func journal(day, model, agent string, in int64) store.Row {
	return store.Row{Day: day, Model: model, Agent: agent, Source: store.SourceMagpie, Input: in, Calls: 1}
}

func opts() Options {
	return Options{Since: "2026-11-01", Until: "2026-11-30", Price: flatPrice}
}

func TestRoutedRowReplacedByJournalDetail(t *testing.T) {
	rows := []store.Row{
		local("2026-11-15", "Auto(未指明)", "grok", 1000, true),
		journal("2026-11-15", "DeepSeek-V4.1-Flash", "grok", 900),
		journal("2026-11-15", "Hy4", "grok", 100),
	}
	res := Build(rows, opts())
	if len(res.Rows) != 2 {
		t.Fatalf("want 2 model rows, got %d: %+v", len(res.Rows), res.Rows)
	}
	for _, r := range res.Rows {
		if r.Name == "Auto(未指明)" {
			t.Fatal("route row should have been replaced by journal detail")
		}
	}
	if res.Totals.Input != 1000 {
		t.Fatalf("total = %d, want exactly the journal detail (1000)", res.Totals.Input)
	}
	if res.RoutedDropped != 1 || res.MagpieRows != 2 {
		t.Fatalf("stats wrong: %+v", res)
	}
}

func TestRoutedRowKeptWithoutCoverage(t *testing.T) {
	// The journal only starts on the 10th, so older route rows are the only
	// evidence of that usage and must count.
	rows := []store.Row{
		local("2026-11-02", "Auto(未指明)", "grok", 500, true),
		journal("2026-11-15", "Hy4", "grok", 10),
	}
	res := Build(rows, opts())
	if res.Totals.Input != 510 {
		t.Fatalf("total = %d, want 510", res.Totals.Input)
	}
	if res.RoutedKept != 1 || res.RoutedKeptTokens != 500 {
		t.Fatalf("stats wrong: %+v", res)
	}
}

func TestCoverageIsPerDayAndAgent(t *testing.T) {
	rows := []store.Row{
		// Journal covers day 15 for grok only.
		journal("2026-11-15", "Hy4", "grok", 10),
		// Same day, different agent: not covered, keep.
		local("2026-11-15", "Auto(未指明)", "pi", 70, true),
		// Same agent, different day: not covered, keep.
		local("2026-11-14", "Auto(未指明)", "grok", 30, true),
	}
	res := Build(rows, opts())
	if res.Totals.Input != 110 {
		t.Fatalf("total = %d, want 110", res.Totals.Input)
	}
	if res.RoutedDropped != 0 || res.RoutedKept != 2 {
		t.Fatalf("stats wrong: %+v", res)
	}
}

func TestDirectRowsAlwaysCount(t *testing.T) {
	rows := []store.Row{
		local("2026-11-15", "Claude-Opus-5.5", "claude", 800, false),
		local("2026-11-15", "Auto(未指明)", "claude", 200, true),
		journal("2026-11-15", "DeepSeek-V4.1-Flash", "claude", 200),
	}
	res := Build(rows, opts())
	if res.Totals.Input != 1000 {
		t.Fatalf("total = %d, want 800 direct + 200 journal", res.Totals.Input)
	}
	var opus bool
	for _, r := range res.Rows {
		if r.Name == "Claude-Opus-5.5" && r.Input == 800 {
			opus = true
		}
	}
	if !opus {
		t.Fatal("direct row lost")
	}
}

func TestExcludeMagpieIsPureLocal(t *testing.T) {
	rows := []store.Row{
		local("2026-11-15", "Auto(未指明)", "grok", 1000, true),
		journal("2026-11-15", "Hy4", "grok", 900),
	}
	o := opts()
	o.ExcludeMagpie = true
	res := Build(rows, o)
	if res.Totals.Input != 1000 {
		t.Fatalf("pure local total = %d, want 1000", res.Totals.Input)
	}
	if res.MagpieRows != 0 {
		t.Fatal("journal rows must be hidden with ExcludeMagpie")
	}
	if len(res.Rows) != 1 || res.Rows[0].Name != "Auto(未指明)" {
		t.Fatalf("unexpected rows %+v", res.Rows)
	}
}

func TestWindowAndGrouping(t *testing.T) {
	rows := []store.Row{
		local("2026-10-01", "Hy4", "grok", 5, false),
		local("2026-11-15", "Hy4", "grok", 7, false),
		local("2026-11-15", "Hy4", "pi", 3, false),
	}
	res := Build(rows, Options{Since: "2026-11-01", Until: "2026-11-30", Price: flatPrice})
	if res.Totals.Input != 10 {
		t.Fatalf("window not applied: %d", res.Totals.Input)
	}
	byAgent := Build(rows, Options{Since: "2026-11-01", Until: "2026-11-30", ByAgent: true, Price: flatPrice})
	if len(byAgent.Rows) != 2 || byAgent.Totals.Input != 10 {
		t.Fatalf("by-agent grouping wrong: %+v", byAgent.Rows)
	}
}

func TestPricingAndUnpriced(t *testing.T) {
	rows := []store.Row{
		local("2026-11-15", "Hy4", "grok", 2_000_000, false),
		local("2026-11-15", "Unpriced", "x", 1_000_000, false),
	}
	res := Build(rows, opts())
	if res.Totals.CostUSD != 2 {
		t.Fatalf("cost = %v, want 2", res.Totals.CostUSD)
	}
	if len(res.Unpriced) != 1 || res.Unpriced[0] != "Unpriced" {
		t.Fatalf("unpriced = %v", res.Unpriced)
	}
}
