package adapters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeclarativeJSONLAdapter(t *testing.T) {
	dir := t.TempDir()
	spec := `
name: demoagent
agent: demo
glob: ` + filepath.ToSlash(dir) + `/**/*.jsonl
format: jsonl
contains: '"usage"'
record:
  filter:
    - path: kind
      equals: response
  time:
    path: at
  model:
    path: meta.model
  input:
    path: usage.prompt_tokens
  output:
    path: usage.completion_tokens
  cache_read:
    path: usage.cached_tokens
  cache_write:
    path: usage.cache_write_tokens
  input_includes_cache: true
`
	path := filepath.Join(dir, "demo.yaml")
	writeFile(t, path, spec)
	a, err := LoadFile(path, dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if a.Name() != "demoagent" || a.Agent() != "demo" {
		t.Fatalf("unexpected identity %q %q", a.Name(), a.Agent())
	}
	ctx := &Context{}
	line := `{"kind":"response","at":"2026-11-15T01:02:03Z","meta":{"model":"acme-v41"},"usage":{"prompt_tokens":100,"completion_tokens":20,"cached_tokens":70,"cache_write_tokens":5}}`
	recs := a.ParseLine([]byte(line), ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Model != "acme-v41" {
		t.Errorf("model = %q", r.Model)
	}
	if r.Input != 25 { // 100 - 70 - 5
		t.Errorf("input should be uncached: %d", r.Input)
	}
	if r.Output != 20 || r.CacheRead != 70 || r.CacheWrite != 5 {
		t.Errorf("unexpected record %+v", r)
	}
	if r.Time.IsZero() {
		t.Error("time not parsed")
	}
	// The filter must reject other kinds.
	if got := a.ParseLine([]byte(`{"kind":"other","usage":{"prompt_tokens":1}}`), ctx); len(got) != 0 {
		t.Errorf("filter ignored: %+v", got)
	}
}

func TestDeclarativeRegexAdapter(t *testing.T) {
	dir := t.TempDir()
	spec := `
name: logfile
glob: ` + filepath.ToSlash(dir) + `/*.log
format: regex
regex: '^(?P<time>[^ ]+) model=(?P<model>[^ ]+) in=(?P<input>\d+) out=(?P<output>\d+)'
`
	path := filepath.Join(dir, "logfile.yaml")
	writeFile(t, path, spec)
	a, err := LoadFile(path, dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	recs := a.ParseLine([]byte("2026-11-15T01:02:03Z model=glm-5.3 in=10 out=20"), &Context{})
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	if recs[0].Input != 10 || recs[0].Output != 20 || recs[0].Model != "glm-5.3" {
		t.Fatalf("unexpected record %+v", recs[0])
	}
}

func TestDeclarativeRejectsInvalid(t *testing.T) {
	if _, err := Compile(DeclSpec{Name: "x"}, "."); err == nil {
		t.Error("missing glob should fail")
	}
	if _, err := Compile(DeclSpec{Name: "x", Glob: "*.jsonl"}, "."); err == nil {
		t.Error("missing field mapping should fail")
	}
	if _, err := Compile(DeclSpec{Name: "x", Glob: "*.jsonl", Format: "regex"}, "."); err == nil {
		t.Error("regex format without regex should fail")
	}
}

func TestDeclarativeLoadDirReportsBadFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "good.yaml"), `
name: good
glob: `+filepath.ToSlash(dir)+`/*.jsonl
record:
  input:
    path: usage.input
`)
	writeFile(t, filepath.Join(dir, "bad.yaml"), "name: bad\n")
	ads, errs := LoadDir(dir, dir)
	if len(ads) != 1 || ads[0].Name() != "good" {
		t.Fatalf("want the good adapter, got %d", len(ads))
	}
	if len(errs) != 1 {
		t.Fatalf("want one error, got %d", len(errs))
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestDeclarativeDisabledSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "off.yaml")
	writeFile(t, path, "name: off\ndisabled: true\nglob: '*'\nrecord:\n  input:\n    path: a\n")
	a, err := LoadFile(path, dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if a != nil {
		t.Fatal("disabled adapter should not load")
	}
}
