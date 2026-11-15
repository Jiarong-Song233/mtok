package model

import "testing"

func TestCleanTrimsSnapshotDates(t *testing.T) {
	cases := map[string]string{
		"gpt-4.1-2025-04-14":        "gpt-4.1",
		"gpt-4.1":                   "gpt-4.1",
		"claude-haiku-4-5":          "claude-haiku-4-5",
		"gemini-2.5-pro-2025-06-17": "gemini-2.5-pro",
	}
	for raw, want := range cases {
		if got := clean(raw); got != want {
			t.Errorf("clean(%q) = %q, want %q", raw, got, want)
		}
	}
	n := New()
	if got := n.Resolve("gpt-4.1-2025-04-14"); got != "GPT-4.1" {
		t.Errorf("dated snapshot = %q, want GPT-4.1", got)
	}
	if got := n.Resolve("hy4-free"); got != "Hy4" {
		t.Errorf("hy4-free = %q, want Hy4", got)
	}
	if got := n.Resolve("hy4-preview"); got != "Hy4-Preview" {
		t.Errorf("hy4-preview = %q", got)
	}
}
