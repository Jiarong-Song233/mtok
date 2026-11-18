package pricing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinLookup(t *testing.T) {
	tab, err := Load("", 0)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := tab.Lookup("Claude-Opus-5.5")
	if !ok || p.Input != 4 || p.Output != 20 || p.CacheRead != 0.2 || p.CacheWrite != 5 {
		t.Fatalf("unexpected price %+v ok=%v", p, ok)
	}
	if p, ok := tab.Lookup("GPT-6.2-Astra"); !ok || p.Input != 8 || p.Output != 40 || p.CacheRead != 0.4 || p.CacheWrite != 10 {
		t.Fatalf("unexpected GPT-6.2-Astra price %+v ok=%v", p, ok)
	}
	if p, ok := tab.Lookup("GPT-6.2-Sol"); !ok || p.Input != 2 || p.Output != 10 || p.CacheRead != 0.1 || p.CacheWrite != 2.5 {
		t.Fatalf("unexpected GPT-6.2-Sol price %+v ok=%v", p, ok)
	}
	// Lookup also accepts any spelling of the canonical name.
	if _, ok := tab.Lookup("claude_opus_5_5"); !ok {
		t.Fatal("separator-insensitive lookup failed")
	}
	if _, ok := tab.Lookup("Auto(未指明)"); ok {
		t.Fatal("unpriced model should report false")
	}
	if p, ok := tab.Lookup("hy4-flash"); !ok || p.Input != 0.10 || p.Output != 0.40 {
		t.Fatalf("hy4-flash should be priced, got %+v ok=%v", p, ok)
	}
	if tab.Rate != 7.2 {
		t.Fatalf("default rate %v", tab.Rate)
	}
}

func TestUserOverrideAndDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	body := `{
	  "currency": { "rate": 7.9 },
	  "default": { "input": 1, "output": 2 },
	  "models": { "glm-5.3": { "input": 0.11, "output": 0.22, "cache_read": 0.01 } }
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tab, err := Load(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Rate != 7.9 {
		t.Fatalf("rate override lost: %v", tab.Rate)
	}
	p, ok := tab.Lookup("GLM-5.3")
	if !ok || p.Input != 0.11 || p.CacheRead != 0.01 {
		t.Fatalf("override lost: %+v", p)
	}
	// Unknown models now fall back to the configured default.
	p, ok = tab.Lookup("Totally-Unknown")
	if !ok || p.Input != 1 {
		t.Fatalf("default not applied: %+v", p)
	}
	// An explicit rate flag wins over the file.
	if tab2, _ := Load(path, 8.5); tab2.Rate != 8.5 {
		t.Fatalf("flag override lost: %v", tab2.Rate)
	}
}

func TestUSDCost(t *testing.T) {
	p := Price{Input: 1, Output: 10, CacheRead: 0.1, CacheWrite: 1.25}
	got := p.USD(1_000_000, 1_000_000, 1_000_000, 1_000_000)
	want := 1 + 10 + 0.1 + 1.25
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("USD() = %v, want %v", got, want)
	}
	if p.USD(0, 0, 0, 0) != 0 {
		t.Fatal("zero tokens should cost zero")
	}
}

func TestBrokenFileIsReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	os.WriteFile(path, []byte("{nope"), 0o644)
	if _, err := Load(path, 0); err == nil {
		t.Fatal("broken pricing file should be an error")
	}
}

func TestExampleJSONParses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	if err := os.WriteFile(path, ExampleJSON(), 0o644); err != nil {
		t.Fatal(err)
	}
	tab, err := Load(path, 0)
	if err != nil {
		t.Fatalf("generated example does not load: %v", err)
	}
	if _, ok := tab.Lookup("GPT-6.1-Sol"); !ok {
		t.Fatal("example lost the built-in table")
	}
}
