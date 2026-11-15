package store

import (
	"path/filepath"
	"testing"
)

func TestAddMergesSameKey(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Add(Row{Day: "2026-11-15", Model: "GLM-5.3", Agent: "claude", Input: 10, Output: 1, Calls: 1})
	s.Add(Row{Day: "2026-11-15", Model: "GLM-5.3", Agent: "claude", Input: 5, Output: 2, CacheRead: 7, Calls: 1})
	s.Add(Row{Day: "2026-11-15", Model: "GLM-5.3", Agent: "codex", Input: 1, Calls: 1})
	rows := s.Rows()
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Agent != "claude" || rows[0].Input != 15 || rows[0].Output != 3 || rows[0].CacheRead != 7 || rows[0].Calls != 2 {
		t.Fatalf("merge wrong: %+v", rows[0])
	}
	if s.Save() != nil {
		t.Fatal("save")
	}
}

func TestEmptyRowIgnored(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.Add(Row{Day: "2026-11-15", Model: "X", Agent: "y"})
	if len(s.Rows()) != 0 {
		t.Fatal("zero-token row should be dropped")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	db := filepath.Join(t.TempDir(), "sub", "t.db")
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Add(Row{Day: "2026-11-14", Model: "Kimi-K3.1-Pro", Agent: "kimi", Input: 100, Output: 20, CacheRead: 40, CacheWrite: 5, Calls: 3})
	s.SetFileState("/x/y.jsonl", FileState{Size: 100, MTime: 42, Off: 100, Model: "kimi-k3.1-pro"})
	s.SetMeta("last_sync", "now")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s.Close()

	again, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	rows := again.Rows()
	if len(rows) != 1 || rows[0].Input != 100 || rows[0].CacheWrite != 5 {
		t.Fatalf("round trip lost data: %+v", rows)
	}
	fs, ok := again.FileState("/x/y.jsonl")
	if !ok || fs.Off != 100 || fs.Model != "kimi-k3.1-pro" {
		t.Fatalf("file state lost: %+v", fs)
	}
	if again.GetMeta("last_sync") != "now" {
		t.Fatal("meta lost")
	}
}

func TestPruneDropsGoneFiles(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.SetFileState("a", FileState{Off: 1})
	s.SetFileState("b", FileState{Off: 2})
	if n := s.PruneFiles(map[string]bool{"a": true}); n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	if s.FileCount() != 1 {
		t.Fatalf("file count %d", s.FileCount())
	}
}

func TestResetClearsEverything(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.Add(Row{Day: "2026-11-15", Model: "M", Agent: "a", Input: 1, Calls: 1})
	s.SetFileState("f", FileState{Off: 1})
	s.SetMeta("k", "v")
	s.Reset()
	if len(s.Rows()) != 0 || s.FileCount() != 0 || s.GetMeta("k") != "" {
		t.Fatal("reset incomplete")
	}
}

func TestCorruptFileIsMovedAside(t *testing.T) {
	db := filepath.Join(t.TempDir(), "t.db")
	s, _ := Open(db)
	s.Add(Row{Day: "2026-11-15", Model: "M", Agent: "a", Input: 1, Calls: 1})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := writeBad(db); err != nil {
		t.Fatal(err)
	}
	again, err := Open(db)
	if err != nil {
		t.Fatalf("open should recover: %v", err)
	}
	defer again.Close()
	if len(again.Rows()) != 0 {
		t.Fatal("corrupt db should start empty")
	}
	if _, err := statFile(db + ".corrupt"); err != nil {
		t.Fatal("corrupt copy missing")
	}
}

func TestDays(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.Add(Row{Day: "2026-11-15", Model: "M", Agent: "a", Input: 1, Calls: 1})
	s.Add(Row{Day: "2026-11-01", Model: "M", Agent: "a", Input: 1, Calls: 1})
	days := s.Days()
	if len(days) != 2 || days[0] != "2026-11-01" {
		t.Fatalf("days = %v", days)
	}
}
