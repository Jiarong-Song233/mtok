package adapters

import "path/filepath"

// Builtins returns the adapters compiled into mtok, in display order.
func Builtins(home string) []Adapter {
	return []Adapter{
		NewClaude(home),
		NewCodex(home),
		NewGemini(home),
		NewGrok(home),
		NewKimi(home),
		NewZCode(home),
		NewPi(home),
		NewDSH(home),
	}
}

// Unsupported describes an agent whose local logs mtok cannot read yet, with
// the reason shown by `mtok doctor` so nothing is silently missing.
type Unsupported struct {
	Name   string
	Probe  string // file or directory whose presence means the agent is in use
	Reason string
}

// UnsupportedSources returns the known-inaccessible sources found on this machine.
func UnsupportedSources(home string) []Unsupported {
	return []Unsupported{
		{
			Name:   "opencode",
			Probe:  filepath.Join(home, ".local", "share", "opencode", "opencode.db"),
			Reason: "usage lives in an SQLite database; mtok reads text logs only (export to JSONL or write a declarative adapter)",
		},
		{
			Name:   "openclaw",
			Probe:  filepath.Join(home, ".openclaw", "state", "openclaw.sqlite"),
			Reason: "usage lives in an SQLite database; not parsed yet",
		},
		{
			Name:   "cursor",
			Probe:  filepath.Join(home, ".cursor"),
			Reason: "transcripts are stored without token counters; usage is only visible in Cursor's own dashboard",
		},
		{
			Name:   "copilot",
			Probe:  filepath.Join(home, ".copilot"),
			Reason: "no token counters in local logs",
		},
	}
}
