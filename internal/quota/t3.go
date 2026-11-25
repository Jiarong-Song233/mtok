package quota

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// T3 (the `t3` gateway CLI) keeps one cache file per connected agent under
// ~/.t3/caches. Each cache carries that agent's rate-limit windows as plain
// JSON written by the gateway itself, so reading it stays offline and
// read-only. Antigravity (Gemini), Claude Code, Grok and others publish their
// 5-hour and weekly allowances this way — data mtok otherwise never sees
// because those agents talk to their vendors directly rather than through a
// proxied log.
type t3Cache struct {
	DisplayName string         `json:"displayName"`
	Driver      string         `json:"driver"`
	InstanceID  string         `json:"instanceId"`
	UsageLimits *t3UsageLimits `json:"usageLimits"`
}

type t3UsageLimits struct {
	Windows []t3Window `json:"windows"`
}

type t3Window struct {
	Label         string  `json:"label"`
	UsedPercent   float64 `json:"usedPercent"`
	ResetsAt      string  `json:"resetsAt"`
	WindowMinutes int     `json:"windowDurationMins"`
}

// t3Provider maps a T3 driver to the provider name mtok reports. Codex is
// skipped because the dedicated Codex session-log reader already covers it
// from a more authoritative source.
func t3Provider(driver, displayName string) (provider, name string, ok bool) {
	switch strings.ToLower(driver) {
	case "antigravity":
		return "gemini", "Gemini (Antigravity)", true
	case "claudeagent":
		return "claude", "Claude Code", true
	case "codex":
		return "", "", false
	case "grok":
		return "grok", "Grok", true
	case "cursor":
		return "cursor", "Cursor", true
	case "kiro":
		return "kiro", "Kiro", true
	case "opencode":
		return "opencode", "OpenCode", true
	}
	d := strings.TrimSpace(displayName)
	if d == "" {
		return "", "", false
	}
	return strings.ToLower(d), d, true
}

// T3 reads every agent cache under ~/.t3/caches and returns one Quota per
// agent that reports rate-limit windows. Missing directory yields no quotas
// and no error — T3 is optional.
func T3(home string) ([]Quota, error) {
	root := filepath.Join(home, ".t3", "caches")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Quota
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, e.Name()))
		if err != nil {
			continue
		}
		var c t3Cache
		if err := json.Unmarshal(data, &c); err != nil {
			continue
		}
		if c.UsageLimits == nil || len(c.UsageLimits.Windows) == 0 {
			continue
		}
		provider, name, ok := t3Provider(c.Driver, c.DisplayName)
		if !ok {
			continue
		}
		out = append(out, t3Quota(provider, name, c.UsageLimits.Windows))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out, nil
}

func t3Quota(provider, name string, windows []t3Window) Quota {
	q := Quota{Provider: provider, Name: name, Source: "t3"}
	q.Windows = make([]Window, 0, len(windows))
	remaining := 100.0
	details := make([]t3Win, 0, len(windows))
	for _, w := range windows {
		used := clamp(w.UsedPercent)
		win := Window{
			Label:         t3ShortLabel(w, windows),
			UsedPercent:   used,
			Remaining:     clamp(100 - used),
			WindowMinutes: w.WindowMinutes,
			ResetsAt:      t3ResetsUnix(w.ResetsAt),
		}
		q.Windows = append(q.Windows, win)
		if win.Remaining < remaining {
			remaining = win.Remaining
		}
		// Only windows the caller actually drew on belong in the compact line;
		// an untouched shared pool at 100% is noise next to a consumed one.
		if used >= 0.5 {
			details = append(details, t3Win{
				short: windowLabel(w.WindowMinutes),
				tail:  t3LabelTail(w.Label),
				used:  used,
			})
		}
	}
	if len(details) == 0 {
		// Nothing consumed yet — surface the tightest window instead of an
		// empty line.
		details = append(details, t3Win{short: windowLabel(windows[0].WindowMinutes), used: clamp(windows[0].UsedPercent)})
	}
	q.Remaining = remaining
	q.Detail = t3Detail(details)
	q.ObservedAt = time.Now().UTC().Format(time.RFC3339)
	return q
}

// t3Win is one consumed window in the compact detail line: a short duration
// label, an optional pool name used only when two windows share a duration,
// and how much of the pool is used.
type t3Win struct {
	short string
	tail  string
	used  float64
}

// t3Detail renders the consumed windows most-used first, capped so the
// compact quota line stays readable when an agent publishes several pools
// (Antigravity, for example, splits Gemini from a shared Claude & GPT pool).
func t3Detail(wins []t3Win) string {
	sort.SliceStable(wins, func(i, j int) bool { return wins[i].used > wins[j].used })
	n := len(wins)
	if n > 3 {
		n = 3
	}
	shown := wins[:n]
	counts := map[string]int{}
	for _, w := range shown {
		counts[w.short]++
	}
	parts := make([]string, 0, n)
	for _, w := range shown {
		label := w.short
		if counts[w.short] > 1 && w.tail != "" {
			label += "·" + w.tail
		}
		parts = append(parts, fmt.Sprintf("%s剩%.0f%%", label, 100-w.used))
	}
	return strings.Join(parts, " ")
}

// t3ShortLabel derives a compact window label. When an agent publishes
// several pools of the same duration the shared/Gemini distinction is kept by
// falling back to the vendor's own label suffix.
func t3ShortLabel(w t3Window, all []t3Window) string {
	base := windowLabel(w.WindowMinutes)
	if w.WindowMinutes <= 0 {
		if s := t3LabelTail(w.Label); s != "" {
			return s
		}
		return base
	}
	// Count windows sharing this duration; if more than one, disambiguate.
	same := 0
	for _, o := range all {
		if o.WindowMinutes == w.WindowMinutes {
			same++
		}
	}
	if same <= 1 {
		return base
	}
	if tail := t3LabelTail(w.Label); tail != "" && tail != base {
		return base + "·" + tail
	}
	return base
}

// t3LabelTail pulls the human part after a "· " separator from a vendor label
// like "Gemini · 5-hour" -> "Gemini".
func t3LabelTail(label string) string {
	s := strings.TrimSpace(label)
	if i := strings.Index(s, "·"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	s = strings.TrimSuffix(s, "-hour")
	s = strings.TrimSpace(s)
	return s
}

// t3ResetsUnix parses the vendor's reset timestamp into a unix second count.
// T3 writes these without a timezone, so they are read in local time — the
// same clock the quota line renders back in.
func t3ResetsUnix(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	layouts := []string{
		"1/2/2006 15:04:05",
		"01/02/2006 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t.Unix()
		}
	}
	return 0
}
