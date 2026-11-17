package quota

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func writeRollout(t *testing.T, home, name, body string) string {
	t.Helper()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "11", "17")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeZstRollout(t *testing.T, home, name, body string) string {
	t.Helper()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "11", "17")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	enc, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func rateLimitLine(ts string, primary, secondary float64) string {
	return `{"type":"event_msg","timestamp":"` + ts + `","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":` +
		f64(primary) + `,"window_minutes":300,"resets_at":1794921702},"secondary":{"used_percent":` +
		f64(secondary) + `,"window_minutes":10080,"resets_at":1795163619},"plan_type":"plus"}}}`
}

func f64(v float64) string {
	if v == float64(int(v)) {
		return itoa(int(v))
	}
	return "0"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestCodexReadsNewestLocalAndCompressedRateLimits(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, home, "rollout-old.jsonl", rateLimitLine("2026-11-17T03:00:00Z", 90, 10)+"\n")
	writeZstRollout(t, home, "rollout-new.jsonl.zst", rateLimitLine("2026-11-17T11:33:48.564Z", 84, 68)+"\n")
	writeZstRollout(t, home, "rollout-corrupt.jsonl.zst", "not a zstd stream")

	q, ok, err := Codex(home, "")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a quota")
	}
	if q.Provider != "codex" || q.Name != "Codex Plus" || q.Source != SourceCodex {
		t.Fatalf("unexpected identity: %+v", q)
	}
	if q.Remaining != 16 || q.Detail != "5h16% 7d32%" {
		t.Fatalf("unexpected remaining/detail: %+v", q)
	}
	if len(q.Windows) != 2 || q.Windows[0].Label != "5h" || q.Windows[1].Label != "7d" {
		t.Fatalf("unexpected windows: %+v", q.Windows)
	}
	if q.ObservedAt != "2026-11-17T11:33:48Z" {
		t.Fatalf("observed_at = %q", q.ObservedAt)
	}
}

func TestCodexCacheFallsBackWhenSessionsDisappear(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(t.TempDir(), "quota-cache.json")
	writeRollout(t, home, "rollout-cached.jsonl", rateLimitLine("2026-11-17T11:33:48Z", 84, 68)+"\n")

	first, ok, err := Codex(home, cache)
	if err != nil || !ok {
		t.Fatalf("first read ok=%v err=%v", ok, err)
	}
	if err := os.RemoveAll(filepath.Join(home, ".codex")); err != nil {
		t.Fatal(err)
	}
	second, ok, err := Codex(home, cache)
	if err != nil || !ok {
		t.Fatalf("cached read ok=%v err=%v", ok, err)
	}
	if second.ObservedAt != first.ObservedAt || second.Detail != first.Detail {
		t.Fatalf("cache changed quota: %+v -> %+v", first, second)
	}
}

func TestCodexWithoutSessionsReturnsNothing(t *testing.T) {
	q, ok, err := Codex(t.TempDir(), "")
	if err != nil || ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	if q.Provider != "" {
		t.Fatalf("unexpected quota: %+v", q)
	}
}
