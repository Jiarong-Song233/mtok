package adapters

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

// JournalSource marks adapters that read the proxy's own usage journal rather
// than an agent log. Their records carry the resolved underlying model, so they
// replace the route-only rows that agent logs cannot identify.
type JournalSource interface {
	JournalSource() bool
}

// Magpie parses magpie's local call journal. Every proxied request is one line
// with the requested route, the model that actually served it and its token
// counts, which is the only place the real model behind auto/group routing is
// visible.
type Magpie struct {
	path string
}

// NewMagpie returns the proxy journal adapter. An empty path falls back to the
// default location used by magpie itself.
func NewMagpie(home, path string) *Magpie {
	if path == "" {
		if env := strings.TrimSpace(envValue("MAGPIE_USAGE_FILE")); env != "" {
			path = env
		} else {
			path = filepath.Join(home, ".config", "magpie", "usage.jsonl")
		}
	}
	return &Magpie{path: path}
}

// Name implements Adapter.
func (m *Magpie) Name() string { return "magpie" }

// Agent implements Adapter. Records override it with the journal's client id.
func (m *Magpie) Agent() string { return "magpie" }

// Globs implements Adapter.
func (m *Magpie) Globs() []string { return []string{filepath.ToSlash(m.path)} }

// Needles implements Prefilterer.
func (m *Magpie) Needles() []string { return []string{`"cache_read"`} }

// JournalSource implements JournalSource.
func (m *Magpie) JournalSource() bool { return true }

// Path is the journal location, shown by `mtok adapters` and doctor.
func (m *Magpie) Path() string { return m.path }

// magpieEntry mirrors one journal line. Unknown fields are ignored on purpose:
// magpie adds fields over time.
type magpieEntry struct {
	T          string `json:"t"`
	Agent      string `json:"agent"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Req        string `json:"req"`
	Served     string `json:"served"`
	In         int64  `json:"in"`
	Out        int64  `json:"out"`
	CacheRead  int64  `json:"cache_read"`
	CacheWrite int64  `json:"cache_write"`
}

// ParseLine implements Adapter.
func (m *Magpie) ParseLine(line []byte, ctx *Context) []Record {
	var e magpieEntry
	if json.Unmarshal(line, &e) != nil {
		return nil
	}
	// served is the model the upstream actually ran; model can still be a route
	// alias such as hy4free or qfmodel, and req is what the client asked for.
	model := firstNonEmpty(e.Served, e.Model, e.Req)
	agent := strings.ToLower(strings.TrimSpace(e.Agent))
	if agent == "" {
		agent = "magpie"
	}
	t := time.Time{}
	if e.T != "" {
		if parsed, ok := ParseTime(e.T); ok {
			t = parsed
		}
	}
	return []Record{{
		Time:       t,
		Model:      model,
		Agent:      agent,
		Input:      e.In,
		Output:     e.Out,
		CacheRead:  e.CacheRead,
		CacheWrite: e.CacheWrite,
	}}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
