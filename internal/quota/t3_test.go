package quota

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const t3AntigravityFixture = `{
  "displayName": "Antigravity",
  "driver": "antigravity",
  "usageLimits": {
    "windows": [
      {"label":"Claude & GPT (shared) · 5-hour","usedPercent":0,"windowDurationMins":300},
      {"label":"Gemini · 5-hour","usedPercent":12.5,"resetsAt":"11/25/2026 17:29:10","windowDurationMins":300},
      {"label":"Gemini · Weekly","usedPercent":30,"resetsAt":"12/01/2026 16:32:35","windowDurationMins":10080}
    ]
  }
}`

const t3CodexFixture = `{
  "displayName": "Codex",
  "driver": "codex",
  "usageLimits": {"windows": [
    {"label":"Session","usedPercent":90,"windowDurationMins":300}
  ]}
}`

func writeT3Cache(t *testing.T, home, name, body string) {
	t.Helper()
	dir := filepath.Join(home, ".t3", "caches")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestT3ReadsGeminiWindows(t *testing.T) {
	home := t.TempDir()
	writeT3Cache(t, home, "antigravity.json", t3AntigravityFixture)
	qs, err := T3(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 1 {
		t.Fatalf("expected 1 quota, got %d", len(qs))
	}
	q := qs[0]
	if q.Provider != "gemini" || q.Name != "Gemini (Antigravity)" || q.Source != "t3" {
		t.Fatalf("unexpected quota identity: %+v", q)
	}
	// Tightest window is weekly at 30% used -> 70% remaining.
	if int(q.Remaining) != 70 {
		t.Fatalf("expected remaining 70, got %v", q.Remaining)
	}
	if len(q.Windows) != 3 {
		t.Fatalf("expected 3 windows, got %d", len(q.Windows))
	}
	// Untouched shared pool is excluded; only the two consumed windows show.
	if q.Detail == "" || q.Detail == "5h剩100%" {
		t.Fatalf("detail should list consumed windows: %q", q.Detail)
	}
	if !strings.Contains(q.Detail, "7d剩70%") {
		t.Fatalf("detail missing weekly window: %q", q.Detail)
	}
	// Reset timestamp parsed to a positive unix second.
	weekly := q.Windows[2]
	if weekly.ResetsAt <= 0 {
		t.Fatalf("weekly reset not parsed: %d", weekly.ResetsAt)
	}
}

func TestT3SkipsCodexAndMissingDir(t *testing.T) {
	home := t.TempDir()
	writeT3Cache(t, home, "codex.json", t3CodexFixture)
	qs, err := T3(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 0 {
		t.Fatalf("codex must be skipped (session reader covers it), got %d", len(qs))
	}

	empty, err := T3(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("missing caches dir should yield no quotas, got %d", len(empty))
	}
}
