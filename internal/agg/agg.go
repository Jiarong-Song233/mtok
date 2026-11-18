// Package agg folds stored rows into display rows. It owns the rule that keeps
// proxy-served usage from being counted twice: local rows that only carry a
// route name are replaced by the journal's resolved models whenever the journal
// covers the same day and agent.
package agg

import (
	"sort"
	"strings"

	"github.com/Jiarong-Song233/mtok/internal/render"
	"github.com/Jiarong-Song233/mtok/internal/store"
)

// Options selects the window and grouping.
type Options struct {
	Since   string
	Until   string
	ByAgent bool
	// ExcludeMagpie drops journal rows and keeps route-only local rows, which
	// is the pure-local view behind `--no-magpie`.
	ExcludeMagpie bool
	// Price returns the reference price for a canonical model name.
	Price func(model string) (input, output, cacheRead, cacheWrite float64, ok bool)
}

// Result is what the renderer and doctor need.
type Result struct {
	Rows     []render.ModelRow
	Totals   render.ModelRow
	Unpriced []string

	// MagpieRows and MagpieTokens count the journal side of the window.
	MagpieRows   int
	MagpieTokens int64
	// RoutedDropped counts local route-only rows replaced by journal detail;
	// RoutedKept counts those the journal could not cover (kept as the only
	// evidence of that usage).
	RoutedDropped       int
	RoutedDroppedTokens int64
	RoutedKept          int
	RoutedKeptTokens    int64
	// DirectTokens is the usage that came straight from agent logs.
	DirectTokens int64
}

// knownAgents are the ids that name an actual coding agent, so a journal row
// attributed to one of them merges with that agent's direct-log rows.
var knownAgents = map[string]bool{
	"claude": true, "codex": true, "gemini": true, "antigravity": true,
	"grok": true, "kimi": true, "zcode": true, "pi": true, "dsh": true,
	"opencode": true, "openclaw": true, "qwen": true, "copilot": true,
}

// DisplayAgent labels a journal row for --by-agent. The proxy also records
// bare HTTP clients (node, curl, Mozilla, python-urllib); those are prefixed so
// they are not mistaken for an agent.
func DisplayAgent(agent string) string {
	if agent == "" {
		return "unknown"
	}
	if knownAgents[agent] {
		return agent
	}
	if strings.HasPrefix(agent, "magpie") {
		return agent
	}
	return "magpie/" + agent
}

// Build aggregates rows inside [Since, Until].
func Build(rows []store.Row, opt Options) Result {
	// Coverage is decided per (day, agent): a routed local row is only replaced
	// when the journal actually has rows for that agent on that day.
	covered := make(map[string]bool, 64)
	if !opt.ExcludeMagpie {
		for _, r := range rows {
			if r.FromMagpie() {
				covered[store.CoverageKey(r.Day, r.Agent)] = true
			}
		}
	}

	var res Result
	buckets := map[string]*render.ModelRow{}
	unpriced := map[string]bool{}
	for _, r := range rows {
		if r.Day < opt.Since || r.Day > opt.Until {
			continue
		}
		switch {
		case r.FromMagpie():
			if opt.ExcludeMagpie {
				continue
			}
			res.MagpieRows++
			res.MagpieTokens += r.Total()
		case r.Routed:
			if covered[store.CoverageKey(r.Day, r.Agent)] {
				res.RoutedDropped++
				res.RoutedDroppedTokens += r.Total()
				continue
			}
			res.RoutedKept++
			res.RoutedKeptTokens += r.Total()
		default:
			res.DirectTokens += r.Total()
		}

		key := r.Model
		if opt.ByAgent {
			key = DisplayAgent(r.Agent)
		}
		b, ok := buckets[key]
		if !ok {
			b = &render.ModelRow{Name: key, Priced: true}
			if opt.ByAgent {
				b.Agent = key
			}
			buckets[key] = b
		}
		b.Input += r.Input
		b.Output += r.Output
		b.CacheRead += r.CacheRead
		b.CacheWrite += r.CacheWrite
		b.Calls += r.Calls
		if opt.Price != nil {
			if in, out, cr, cw, priced := opt.Price(r.Model); priced {
				b.CostUSD += float64(r.Input)/1e6*in + float64(r.Output)/1e6*out +
					float64(r.CacheRead)/1e6*cr + float64(r.CacheWrite)/1e6*cw
			} else {
				b.Priced = false
				unpriced[r.Model] = true
			}
		}
	}

	out := make([]render.ModelRow, 0, len(buckets))
	totals := render.ModelRow{Name: "合计", Priced: true}
	for _, b := range buckets {
		out = append(out, *b)
		totals.Input += b.Input
		totals.Output += b.Output
		totals.CacheRead += b.CacheRead
		totals.CacheWrite += b.CacheWrite
		totals.Calls += b.Calls
		totals.CostUSD += b.CostUSD
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total() != out[j].Total() {
			return out[i].Total() > out[j].Total()
		}
		return out[i].Name < out[j].Name
	})
	for n := range unpriced {
		res.Unpriced = append(res.Unpriced, n)
	}
	sort.Strings(res.Unpriced)
	res.Rows = out
	res.Totals = totals
	return res
}
