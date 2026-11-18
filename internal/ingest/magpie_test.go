package ingest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jiarong-Song233/mtok/internal/adapters"
	"github.com/Jiarong-Song233/mtok/internal/model"
)

// The journal adapter must mark its rows as proxy-sourced, and local records
// that only carry a route name must be flagged so the query can drop them.
func TestIngestMarksRoutedAndJournalRows(t *testing.T) {
	home, localAd, st := fixture(t)
	logPath := filepath.Join(home, "logs", "r.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"at":"2026-11-15T10:00:00Z","model":"group/auto-auto","usage":{"prompt_tokens":900,"completion_tokens":10}}` + "\n" +
		`{"at":"2026-11-15T10:01:00Z","model":"claude-opus-5-5","usage":{"prompt_tokens":100,"completion_tokens":5}}` + "\n"
	if err := os.WriteFile(logPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(home, "magpie-usage.jsonl")
	journalBody := `{"t":"2026-11-15T10:00:00-06:00","agent":"grok","model":"hy4free","served":"hy4","in":900,"out":10,"cache_read":50}` + "\n"
	if err := os.WriteFile(journalPath, []byte(journalBody), 0o644); err != nil {
		t.Fatal(err)
	}

	ads := []adapters.Adapter{localAd, adapters.NewMagpie(home, journalPath)}
	if _, err := Run(st, ads, model.New(), Options{Home: home, Zstd: true}); err != nil {
		t.Fatal(err)
	}

	var routed, direct, journal int
	for _, r := range st.Rows() {
		switch {
		case r.FromMagpie():
			journal++
			if r.Model != "Hy4" {
				t.Errorf("journal model = %q, want Hy4 (served)", r.Model)
			}
			if r.Agent != "grok" {
				t.Errorf("journal agent = %q", r.Agent)
			}
		case r.Routed:
			routed++
			if r.Model != "Auto(未指明)" {
				t.Errorf("routed model = %q", r.Model)
			}
		default:
			direct++
			if r.Model != "Claude-Opus-5.5" {
				t.Errorf("direct model = %q", r.Model)
			}
		}
	}
	if routed != 1 || direct != 1 || journal != 1 {
		t.Fatalf("routed=%d direct=%d journal=%d, want 1/1/1", routed, direct, journal)
	}
}

// With the journal adapter disabled the local route row is the only evidence
// and must be stored without the magpie source marker.
func TestIngestJournalCanBeDisabled(t *testing.T) {
	home, localAd, st := fixture(t)
	logPath := filepath.Join(home, "logs", "d.jsonl")
	os.MkdirAll(filepath.Dir(logPath), 0o755)
	os.WriteFile(logPath, []byte(`{"at":"2026-11-15T10:00:00Z","model":"auto/auto","usage":{"prompt_tokens":10,"completion_tokens":1}}`+"\n"), 0o644)
	journalPath := filepath.Join(home, "magpie-usage.jsonl")
	os.WriteFile(journalPath, []byte(`{"t":"2026-11-15T10:00:00-06:00","agent":"grok","served":"hy4","in":10,"out":1}`+"\n"), 0o644)

	ads := []adapters.Adapter{localAd, adapters.NewMagpie(home, journalPath)}
	res, err := Run(st, ads, model.New(), Options{Home: home, Zstd: true, Disabled: map[string]bool{"magpie": true}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 1 {
		t.Fatalf("records = %d, want only the local one", res.Records)
	}
	for _, r := range st.Rows() {
		if r.FromMagpie() {
			t.Fatal("journal row ingested while disabled")
		}
	}
}
