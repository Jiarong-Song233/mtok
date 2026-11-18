package app

import (
	"testing"
	"time"

	"github.com/Jiarong-Song233/mtok/internal/render"
	"github.com/Jiarong-Song233/mtok/internal/store"
)

func TestParseFlagsAnywhere(t *testing.T) {
	o, err := parse([]string{"7d", "--json", "--width", "80", "--top=5", "--by-agent", "doctor"})
	if err != nil {
		t.Fatal(err)
	}
	if o.rangeArg != "7d" || !o.json || o.width != 80 || o.top != 5 || !o.byAgent {
		t.Fatalf("unexpected options %+v", o)
	}
	if o.cmd != "doctor" {
		t.Fatalf("command not detected: %q", o.cmd)
	}
	if _, err := parse([]string{"--width"}); err == nil {
		t.Fatal("missing value should fail")
	}
	if _, err := parse([]string{"--nope"}); err == nil {
		t.Fatal("unknown flag should fail")
	}
}

func TestResolveSpan(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 30, 0, 0, time.Local)
	cases := []struct {
		opts  options
		since string
		until string
		label string
	}{
		{options{}, "2026-11-15", "2026-11-15", "今天"},
		{options{rangeArg: "today"}, "2026-11-15", "2026-11-15", "今天"},
		{options{rangeArg: "7d"}, "2026-11-09", "2026-11-15", "近 7 天"},
		{options{rangeArg: "7"}, "2026-11-09", "2026-11-15", "近 7 天"},
		{options{rangeArg: "30d"}, "2026-10-17", "2026-11-15", "近 30 天"},
		{options{rangeArg: "month"}, "2026-11-01", "2026-11-15", "本月"},
		{options{rangeArg: "all"}, "0000-01-01", "2026-11-15", "全部"},
		{options{rangeArg: "2026-10-03"}, "2026-10-03", "2026-10-03", "指定日"},
		{options{since: "2026-11-01", until: "2026-11-03"}, "2026-11-01", "2026-11-03", "自定义"},
	}
	for _, c := range cases {
		got, err := resolveSpan(c.opts, now)
		if err != nil {
			t.Fatalf("%+v: %v", c.opts, err)
		}
		if got.since != c.since || got.until != c.until || got.label != c.label {
			t.Errorf("%+v -> %+v, want %s..%s %s", c.opts, got, c.since, c.until, c.label)
		}
	}
	if _, err := resolveSpan(options{rangeArg: "banana"}, now); err == nil {
		t.Fatal("invalid range should fail")
	}
}

func TestSpanLabel(t *testing.T) {
	if got := spanLabel(span{since: "2026-11-15", until: "2026-11-15"}, nil); got != "11-15" {
		t.Errorf("single day = %q", got)
	}
	if got := spanLabel(span{since: "2026-11-01", until: "2026-11-15"}, nil); got != "11-01~11-15" {
		t.Errorf("range = %q", got)
	}
	if got := spanLabel(span{since: "2025-12-20", until: "2026-01-05"}, nil); got != "2025-12-20~2026-01-05" {
		t.Errorf("cross-year = %q", got)
	}
	rows := []store.Row{{Day: "2026-04-08"}, {Day: "2026-11-15"}}
	if got := spanLabel(span{since: "0000-01-01", until: "2026-11-15"}, rows); got != "04-08~11-15" {
		t.Errorf("all = %q", got)
	}
	if got := spanLabel(span{since: "0000-01-01", until: "2026-11-15"}, nil); got != "—" {
		t.Errorf("empty all = %q", got)
	}
}

func TestParsePricingUpdate(t *testing.T) {
	o, err := parse([]string{"pricing", "--update"})
	if err != nil {
		t.Fatal(err)
	}
	if o.cmd != "pricing" || !o.pricingUpdate {
		t.Fatalf("unexpected options %+v", o)
	}
}

func TestParsePricingFetch(t *testing.T) {
	o, err := parse([]string{"pricing", "--fetch"})
	if err != nil {
		t.Fatal(err)
	}
	if o.cmd != "pricing" || !o.pricingFetch {
		t.Fatalf("unexpected options %+v", o)
	}
}

func TestParseQuotaSchemaAndCheck(t *testing.T) {
	o, err := parse([]string{"quota", "--json", "--check"})
	if err != nil {
		t.Fatal(err)
	}
	if o.cmd != "quota" || !o.json || !o.check {
		t.Fatalf("unexpected options %+v", o)
	}
	o, err = parse([]string{"schema"})
	if err != nil {
		t.Fatal(err)
	}
	if o.cmd != "schema" {
		t.Fatalf("schema command = %q", o.cmd)
	}
}

func TestExitForBudget(t *testing.T) {
	if got := exitFor(options{check: true}, &render.View{}); got != 0 {
		t.Fatalf("no budget should exit 0, got %d", got)
	}
	v := &render.View{Budget: &render.Budget{
		Currency: "CNY",
		Daily:    &render.BudgetPeriod{Limit: 10, Spent: 12, Exceeded: true},
	}}
	if got := exitFor(options{check: true}, v); got != 3 {
		t.Fatalf("budget breach should exit 3, got %d", got)
	}
	if got := exitFor(options{}, v); got != 0 {
		t.Fatalf("normal report should exit 0, got %d", got)
	}
}

func TestParseDays(t *testing.T) {
	for in, want := range map[string]int{"7": 7, "7d": 7, "30days": 30, "1": 1} {
		got, ok := parseDays(in)
		if !ok || got != want {
			t.Errorf("parseDays(%q) = %d,%v", in, got, ok)
		}
	}
	for _, in := range []string{"d", "7x", "", "07a"} {
		if _, ok := parseDays(in); ok {
			t.Errorf("parseDays(%q) should fail", in)
		}
	}
}
