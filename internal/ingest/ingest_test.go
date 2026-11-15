package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"

	"github.com/songjiarong/mtok/internal/adapters"
	"github.com/songjiarong/mtok/internal/model"
	"github.com/songjiarong/mtok/internal/store"
)

// fixture builds a temp home with one declarative adapter over logs/*.jsonl.
func fixture(t *testing.T) (home string, ad adapters.Adapter, st *store.Store) {
	t.Helper()
	home = t.TempDir()
	spec := `
name: demo
glob: ` + filepath.ToSlash(filepath.Join(home, "logs", "**", "*.jsonl")) + `
record:
  time:
    path: at
  model:
    path: model
  input:
    path: usage.prompt_tokens
  output:
    path: usage.completion_tokens
`
	a, err := adapters.Compile(adapters.DeclSpec{
		Name: "demo",
		Glob: filepath.ToSlash(filepath.Join(home, "logs", "**", "*.jsonl")),
		Record: adapters.DeclRecord{
			Time:   adapters.DeclField{Path: "at"},
			Model:  adapters.DeclField{Path: "model"},
			Input:  adapters.DeclField{Path: "usage.prompt_tokens"},
			Output: adapters.DeclField{Path: "usage.completion_tokens"},
		},
	}, home)
	if err != nil {
		t.Fatalf("compile: %v\n%s", err, spec)
	}
	s, err := store.Open(filepath.Join(home, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return home, a, s
}

func line(day, modelName string, in, out int) string {
	return `{"at":"` + day + `T12:00:00Z","model":"` + modelName + `","usage":{"prompt_tokens":` +
		itoa(in) + `,"completion_tokens":` + itoa(out) + `}}` + "\n"
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

func run(t *testing.T, st *store.Store, ad adapters.Adapter, home string) Result {
	t.Helper()
	res, err := Run(st, []adapters.Adapter{ad}, model.New(), Options{Home: home, Zstd: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	return res
}

func totals(t *testing.T, st *store.Store) (in, out, calls int64) {
	t.Helper()
	for _, r := range st.Rows() {
		in += r.Input
		out += r.Output
		calls += r.Calls
	}
	return
}

func TestIngestReadsOnlyNewBytes(t *testing.T) {
	home, ad, st := fixture(t)
	logPath := filepath.Join(home, "logs", "a.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(line("2026-11-15", "glm-5.3", 100, 10)+line("2026-11-15", "glm-5.3", 200, 20)), 0o644); err != nil {
		t.Fatal(err)
	}

	res := run(t, st, ad, home)
	if res.Records != 2 || res.Changed != 1 {
		t.Fatalf("first pass records=%d changed=%d", res.Records, res.Changed)
	}
	in, out, calls := totals(t, st)
	if in != 300 || out != 30 || calls != 2 {
		t.Fatalf("totals in=%d out=%d calls=%d", in, out, calls)
	}
	fs, ok := st.FileState(logPath)
	if !ok || fs.Off != int64(len(line("2026-11-15", "glm-5.3", 100, 10)+line("2026-11-15", "glm-5.3", 200, 20))) {
		t.Fatalf("offset not recorded: %+v", fs)
	}

	// Second pass with no new bytes must read nothing and add nothing.
	res = run(t, st, ad, home)
	if res.Records != 0 || res.Bytes != 0 || res.Changed != 0 {
		t.Fatalf("second pass should be a no-op: %+v", res)
	}
	if in2, _, calls2 := totals(t, st); in2 != 300 || calls2 != 2 {
		t.Fatalf("second pass double counted: in=%d calls=%d", in2, calls2)
	}

	// Appending one line must add exactly that line.
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line("2026-11-16", "glm-5.3", 50, 5)); err != nil {
		t.Fatal(err)
	}
	f.Close()
	res = run(t, st, ad, home)
	if res.Records != 1 || res.Bytes != int64(len(line("2026-11-16", "glm-5.3", 50, 5))) {
		t.Fatalf("append should read one line: %+v", res)
	}
	if in, _, _ := totals(t, st); in != 350 {
		t.Fatalf("append miscounted: in=%d", in)
	}
	// The new day produced a second row.
	if len(st.Rows()) != 2 {
		t.Fatalf("want 2 rows, got %d", len(st.Rows()))
	}
}

func TestIngestWaitsForCompleteLine(t *testing.T) {
	home, ad, st := fixture(t)
	logPath := filepath.Join(home, "logs", "b.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	partial := strings.TrimSuffix(line("2026-11-15", "glm-5.3", 10, 1), "\n")
	if err := os.WriteFile(logPath, []byte(partial), 0o644); err != nil {
		t.Fatal(err)
	}
	res := run(t, st, ad, home)
	if res.Records != 0 {
		t.Fatalf("partial line should be ignored, got %d records", res.Records)
	}
	fs, _ := st.FileState(logPath)
	if fs.Off != 0 {
		t.Fatalf("offset should stay at 0, got %d", fs.Off)
	}
	// Completing the line makes it count exactly once.
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("\n")
	f.Close()
	res = run(t, st, ad, home)
	if res.Records != 1 {
		t.Fatalf("completed line should count once, got %d", res.Records)
	}
}

func TestIngestResetsOnTruncation(t *testing.T) {
	home, ad, st := fixture(t)
	logPath := filepath.Join(home, "logs", "c.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(line("2026-11-15", "glm-5.3", 100, 10)), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, st, ad, home)
	// Rotation: a smaller file replaces it, so the old offset is invalid.
	if err := os.WriteFile(logPath, []byte(line("2026-11-15", "glm-5.3", 7, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	res := run(t, st, ad, home)
	if res.Records != 1 {
		t.Fatalf("rotated file should be re-read, got %d", res.Records)
	}
	fs, _ := st.FileState(logPath)
	if fs.Off != int64(len(line("2026-11-15", "glm-5.3", 7, 1))) {
		t.Fatalf("offset not reset: %+v", fs)
	}
}

func TestIngestZstdArchiveOnce(t *testing.T) {
	home, _, st := fixture(t)
	ad, err := adapters.Compile(adapters.DeclSpec{
		Name: "zst",
		Glob: filepath.ToSlash(filepath.Join(home, "logs", "**", "*.jsonl.zst")),
		Record: adapters.DeclRecord{
			Time:   adapters.DeclField{Path: "at"},
			Model:  adapters.DeclField{Path: "model"},
			Input:  adapters.DeclField{Path: "usage.prompt_tokens"},
			Output: adapters.DeclField{Path: "usage.completion_tokens"},
		},
	}, home)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "logs", "arch")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := line("2026-11-10", "deepseek-flash", 11, 2) + line("2026-11-10", "deepseek-flash", 13, 4)
	raw, err := os.Create(filepath.Join(dir, "s.jsonl.zst"))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := zstd.NewWriter(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	enc.Close()
	raw.Close()

	res := run(t, st, ad, home)
	if res.Records != 2 {
		t.Fatalf("want 2 records from archive, got %d", res.Records)
	}
	// A second pass must not decode (and therefore must not double count) it.
	res = run(t, st, ad, home)
	if res.Records != 0 || res.Changed != 0 {
		t.Fatalf("archive re-read: %+v", res)
	}
	if in, _, _ := totals(t, st); in != 24 {
		t.Fatalf("archive double counted: in=%d", in)
	}
}

func TestIngestDisabledAdapter(t *testing.T) {
	home, ad, st := fixture(t)
	logPath := filepath.Join(home, "logs", "d.jsonl")
	os.MkdirAll(filepath.Dir(logPath), 0o755)
	os.WriteFile(logPath, []byte(line("2026-11-15", "glm-5.3", 5, 1)), 0o644)
	res, err := Run(st, []adapters.Adapter{ad}, model.New(), Options{Home: home, Zstd: true, Disabled: map[string]bool{"demo": true}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 0 || len(res.Disabled) != 1 {
		t.Fatalf("disabled adapter still ran: %+v", res)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{512: "512B", 1024: "1.0KB", 1536: "1.5KB", 5 << 20: "5.0MB"}
	for in, want := range cases {
		if got := HumanBytes(in); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
