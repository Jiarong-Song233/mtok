// Package quota collects remaining-allowance data from local, read-only
// sources. It never calls a vendor API.
package quota

// Source identifiers exposed in --json so scripts can tell where a number
// came from.
const (
	SourceCodex  = "codex-sessions"
	SourceMagpie = "magpie"
)

// Window is one rate-limit window inside a provider's quota.
type Window struct {
	Label         string  `json:"label"`
	UsedPercent   float64 `json:"used_percent"`
	Remaining     float64 `json:"remaining"`
	WindowMinutes int     `json:"window_minutes,omitempty"`
	ResetsAt      int64   `json:"resets_at,omitempty"`
}

// Quota is one provider's remaining allowance. Remaining is the tightest
// window, and Detail carries every window in compact form.
type Quota struct {
	Provider   string   `json:"provider"`
	Name       string   `json:"name"`
	Remaining  float64  `json:"remaining"`
	Detail     string   `json:"detail,omitempty"`
	Source     string   `json:"source,omitempty"`
	ObservedAt string   `json:"observed_at,omitempty"`
	Windows    []Window `json:"windows,omitempty"`
}

// Merge adds extra quotas while replacing any earlier entry from the same
// provider, so a local source can override a proxy reading.
func Merge(base, extra []Quota) []Quota {
	out := make([]Quota, 0, len(base)+len(extra))
	for _, q := range base {
		replaced := false
		for _, n := range extra {
			if n.Provider == q.Provider {
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, q)
		}
	}
	return append(out, extra...)
}
