package quota

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

type codexLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   struct {
		Type       string           `json:"type"`
		RateLimits *codexRateLimits `json:"rate_limits"`
	} `json:"payload"`
}

type codexRateLimits struct {
	Primary   *codexWindow `json:"primary"`
	Secondary *codexWindow `json:"secondary"`
	PlanType  string       `json:"plan_type"`
	LimitID   string       `json:"limit_id"`
}

type codexWindow struct {
	UsedPercent   *float64 `json:"used_percent"`
	WindowMinutes int      `json:"window_minutes"`
	ResetsAt      int64    `json:"resets_at"`
}

const codexCacheVersion = 1

type codexCache struct {
	Version   int    `json:"v"`
	CheckedAt string `json:"checked_at"`
	Quota     Quota  `json:"quota"`
}

// Codex reads the newest token_count.rate_limits event from the local Codex
// session logs. A small cache keeps later reads fast: only files modified
// since the previous check are opened. Both plain and zstd-compressed rollout
// files are supported, and one unreadable archive never hides a good reading.
func Codex(home, cachePath string) (Quota, bool, error) {
	best, found, cutoff := loadCodexCache(cachePath)
	bestAt := observedAt(best)

	root := filepath.Join(home, ".codex", "sessions")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) && found {
			return best, true, nil
		}
		if os.IsNotExist(err) {
			return Quota{}, false, nil
		}
		return best, found, err
	}
	var softErr error

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if softErr == nil {
				softErr = err
			}
			return nil
		}
		if d.IsDir() || !isRollout(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if softErr == nil {
				softErr = err
			}
			return nil
		}
		if !cutoff.IsZero() && info.ModTime().Before(cutoff) {
			return nil
		}
		q, at, err := scanCodexFile(path, isCompressed(d.Name()))
		if err != nil {
			if softErr == nil {
				softErr = err
			}
			return nil
		}
		if !found || at.After(bestAt) {
			best, bestAt, found = q, at, true
		}
		return nil
	})
	if walkErr != nil && softErr == nil {
		softErr = walkErr
	}
	if !found {
		return Quota{}, false, softErr
	}
	saveCodexCache(cachePath, best)
	return best, true, nil
}

func loadCodexCache(path string) (Quota, bool, time.Time) {
	if path == "" {
		return Quota{}, false, time.Time{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Quota{}, false, time.Time{}
	}
	var c codexCache
	if err := json.Unmarshal(data, &c); err != nil || c.Version != codexCacheVersion || c.Quota.Provider == "" {
		return Quota{}, false, time.Time{}
	}
	checked, err := time.Parse(time.RFC3339, c.CheckedAt)
	if err != nil {
		checked = time.Time{}
	}
	// Two seconds of overlap covers coarse filesystem timestamps.
	return c.Quota, true, checked.Add(-2 * time.Second)
}

func saveCodexCache(path string, q Quota) {
	if path == "" {
		return
	}
	c := codexCache{
		Version:   codexCacheVersion,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Quota:     q,
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err == nil {
		_ = os.Rename(tmp, path)
	}
}

func observedAt(q Quota) time.Time {
	t, err := time.Parse(time.RFC3339, q.ObservedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

func isRollout(name string) bool {
	if !strings.HasPrefix(name, "rollout-") {
		return false
	}
	for _, suffix := range []string{".jsonl", ".jsonl.zst", ".jsonl.zstd"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func isCompressed(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".zst") ||
		strings.HasSuffix(strings.ToLower(name), ".zstd")
}

func scanCodexFile(path string, compressed bool) (Quota, time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return Quota{}, time.Time{}, err
	}
	defer f.Close()

	var src io.Reader = f
	if compressed {
		dec, err := zstd.NewReader(f, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true))
		if err != nil {
			return Quota{}, time.Time{}, err
		}
		defer dec.Close()
		src = dec
	}

	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	var best Quota
	var bestAt time.Time
	var found bool
	for scanner.Scan() {
		line := scanner.Bytes()
		if !bytes.Contains(line, []byte("rate_limits")) || !bytes.Contains(line, []byte("token_count")) {
			continue
		}
		var l codexLine
		if err := json.Unmarshal(line, &l); err != nil {
			continue
		}
		if l.Type != "event_msg" || l.Payload.Type != "token_count" || l.Payload.RateLimits == nil {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
		if err != nil {
			continue
		}
		q, ok := quotaFromLine(l, at)
		if !ok {
			continue
		}
		if !found || at.After(bestAt) {
			best, bestAt, found = q, at, true
		}
	}
	if err := scanner.Err(); err != nil {
		return Quota{}, time.Time{}, err
	}
	return best, bestAt, nil
}

func quotaFromLine(l codexLine, at time.Time) (Quota, bool) {
	rl := l.Payload.RateLimits
	windows := make([]Window, 0, 2)
	if w, ok := makeWindow(rl.Primary); ok {
		windows = append(windows, w)
	}
	if w, ok := makeWindow(rl.Secondary); ok {
		windows = append(windows, w)
	}
	if len(windows) == 0 {
		return Quota{}, false
	}
	remaining := windows[0].Remaining
	details := make([]string, 0, len(windows))
	for _, w := range windows {
		details = append(details, fmt.Sprintf("%s%.0f%%", w.Label, w.Remaining))
		if w.Remaining < remaining {
			remaining = w.Remaining
		}
	}
	return Quota{
		Provider:   "codex",
		Name:       codexPlanName(rl.PlanType),
		Remaining:  remaining,
		Detail:     strings.Join(details, " "),
		Source:     SourceCodex,
		ObservedAt: at.UTC().Format(time.RFC3339),
		Windows:    windows,
	}, true
}

func makeWindow(w *codexWindow) (Window, bool) {
	if w == nil || w.UsedPercent == nil {
		return Window{}, false
	}
	used := clamp(*w.UsedPercent)
	return Window{
		Label:         windowLabel(w.WindowMinutes),
		UsedPercent:   used,
		Remaining:     clamp(100 - used),
		WindowMinutes: w.WindowMinutes,
		ResetsAt:      w.ResetsAt,
	}, true
}

func windowLabel(minutes int) string {
	switch minutes {
	case 60:
		return "1h"
	case 300:
		return "5h"
	case 1440:
		return "1d"
	case 10080:
		return "7d"
	default:
		if minutes > 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return "窗口"
	}
}

func codexPlanName(plan string) string {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "plus":
		return "Codex Plus"
	case "pro":
		return "Codex Pro"
	case "business":
		return "Codex Business"
	case "enterprise":
		return "Codex Enterprise"
	case "free":
		return "Codex Free"
	default:
		return "Codex"
	}
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
