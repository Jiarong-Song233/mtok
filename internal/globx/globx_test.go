package globx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchSegments(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*.jsonl", "a.jsonl", true},
		{"**/*.jsonl", "x/y/a.jsonl", true},
		{"**/*.jsonl", "x/y/a.json", false},
		{"*/logs/*.jsonl", "s/logs/a.jsonl", true},
		{"*/logs/*.jsonl", "s/other/a.jsonl", false},
		{"*/*/logs/transcript_full.jsonl", "brain/.system_generated/logs/transcript_full.jsonl", true},
		{"*/logs/transcript_full.jsonl", "brain/logs/transcript_full.jsonl", true},
		{"rollout-*.jsonl.zst", "rollout-1.jsonl.zst", true},
	}
	for _, c := range cases {
		got := MatchSegments(splitPath(c.pattern), splitPath(c.path))
		if got != c.want {
			t.Errorf("MatchSegments(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestMatchPrefixPrunes(t *testing.T) {
	pat := splitPath("*/logs/*.jsonl")
	if MatchPrefix(pat, splitPath("a/chunks")) {
		t.Error("a/chunks cannot contain a match and should be pruned")
	}
	if !MatchPrefix(pat, splitPath("a/logs")) {
		t.Error("a/logs should be walked")
	}
	doublestar := splitPath("**/*.jsonl")
	if !MatchPrefix(doublestar, splitPath("any/depth")) {
		t.Error("** must keep walking")
	}
}

func TestExpandFindsFilesAndSkipsOtherTrees(t *testing.T) {
	root := t.TempDir()
	must := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(root, "a", "logs", "transcript_full.jsonl"), "")
	must(filepath.Join(root, "a", "chunks", "000.jsonl"), "")
	must(filepath.Join(root, "a", "logs", "other.jsonl"), "")
	must(filepath.Join(root, "b", "logs", "transcript_full.jsonl"), "")

	got := Expand(filepath.ToSlash(filepath.Join(root, "*", "logs", "transcript_full.jsonl")), root)
	if len(got) != 2 {
		t.Fatalf("want 2 files, got %d: %v", len(got), got)
	}
	// Reliance on the "**" form still finds nested files.
	deep := Expand(filepath.ToSlash(filepath.Join(root, "**", "*.jsonl")), root)
	if len(deep) != 4 {
		t.Fatalf("want 4 jsonl files, got %d: %v", len(deep), deep)
	}
}

func TestExpandMissingRootIsEmpty(t *testing.T) {
	if got := Expand(filepath.ToSlash(filepath.Join(t.TempDir(), "nope", "**", "*.jsonl")), "."); len(got) != 0 {
		t.Fatalf("want none, got %v", got)
	}
}
