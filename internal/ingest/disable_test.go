package ingest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jiarong-Song233/mtok/internal/adapters"
	"github.com/Jiarong-Song233/mtok/internal/model"
)

// Disabling an adapter for one run must not drop its offsets: doing so makes the
// next enabled run re-read the whole log and count everything twice.
func TestDisableThenEnableDoesNotDoubleCount(t *testing.T) {
	home, ad, st := fixture(t)
	logPath := filepath.Join(home, "logs", "keep.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(line("2026-11-15", "glm-5.3", 100, 10)), 0o644); err != nil {
		t.Fatal(err)
	}
	ads := []adapters.Adapter{ad}
	if _, err := Run(st, ads, model.New(), Options{Home: home, Zstd: true}); err != nil {
		t.Fatal(err)
	}
	if in, _, _ := totals(t, st); in != 100 {
		t.Fatalf("first pass in=%d", in)
	}
	// A run that skips the adapter (e.g. --no-magpie style) must keep the
	// offset so the next pass only reads new bytes.
	if _, err := Run(st, ads, model.New(), Options{Home: home, Zstd: true, Disabled: map[string]bool{"demo": true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(st, ads, model.New(), Options{Home: home, Zstd: true}); err != nil {
		t.Fatal(err)
	}
	if in, _, calls := totals(t, st); in != 100 || calls != 1 {
		t.Fatalf("re-enable double counted: in=%d calls=%d", in, calls)
	}
	if fs, ok := st.FileState(logPath); !ok || fs.Off == 0 {
		t.Fatalf("offset lost: %+v", fs)
	}
}
