package agg

import "testing"

func TestDisplayAgent(t *testing.T) {
	cases := map[string]string{
		"claude":        "claude",
		"grok":          "grok",
		"opencode":      "opencode",
		"mozilla":       "magpie/mozilla",
		"node":          "magpie/node",
		"curl":          "magpie/curl",
		"python-urllib": "magpie/python-urllib",
		"magpie":        "magpie",
		"":              "unknown",
	}
	for in, want := range cases {
		if got := DisplayAgent(in); got != want {
			t.Errorf("DisplayAgent(%q) = %q, want %q", in, got, want)
		}
	}
}
