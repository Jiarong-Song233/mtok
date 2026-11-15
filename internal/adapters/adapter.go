// Package adapters parses token usage out of agent logs. A Go adapter
// implements Adapter; a declarative adapter reads the same information from
// YAML, so new sources need no rebuild.
package adapters

import (
	"bytes"
	"encoding/json"
	"time"
)

// Record is one billable event with token counts already converted to mtok's
// canonical convention: Input excludes cached tokens, Output includes reasoning
// tokens (as the vendors report them).
type Record struct {
	Time       time.Time
	Model      string // raw identifier; canonicalized later
	Agent      string // "" means the adapter's own name
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
}

// Context carries per-file state across lines. Model is the model most recently
// announced in this file; a stateful adapter writes it back so an incremental
// scan that starts mid-file still knows the model.
type Context struct {
	Path  string
	Model string
	// Seen lets an adapter deduplicate events inside one read (e.g. repeated
	// message ids in Claude transcripts). It is reset per file per scan.
	Seen map[string]struct{}
}

// MarkSeen records id and reports whether it was new.
func (c *Context) MarkSeen(id string) bool {
	if id == "" {
		return true
	}
	if c.Seen == nil {
		c.Seen = make(map[string]struct{}, 64)
	}
	if _, ok := c.Seen[id]; ok {
		return false
	}
	c.Seen[id] = struct{}{}
	return true
}

// Adapter is the extension point. Implement this interface to add a built-in
// source, or drop a YAML file in ~/.config/mtok/adapters.d for a one-off source.
type Adapter interface {
	// Name is the stable id used by config (agents: {name: false}) and doctor.
	Name() string
	// Agent is the label shown for its usage; defaults to Name.
	Agent() string
	// Globs lists path patterns; "**" matches across directories.
	Globs() []string
	// ParseLine returns the records found in one line of a log file.
	ParseLine(line []byte, ctx *Context) []Record
}

// Prefilterer lets an adapter reject lines by cheap substring tests, which is
// what keeps a 4 GB scan fast: most lines never reach the JSON decoder.
type Prefilterer interface {
	// Needles returns substrings; a line must contain at least one to be parsed.
	Needles() []string
}

// WholeFile marks sources that cannot be seeked (compressed archives). Such a
// file is decoded and parsed in full whenever its size or mtime changes.
type WholeFile interface {
	WholeFile() bool
}

// Needles returns the prefilter for a, or nil.
func Needles(a Adapter) []string {
	if p, ok := a.(Prefilterer); ok {
		return p.Needles()
	}
	return nil
}

// WantsWholeFile reports whether a must be decoded completely.
func WantsWholeFile(a Adapter) bool {
	w, ok := a.(WholeFile)
	return ok && w.WholeFile()
}

// MatchesNeedles reports whether line contains any needle.
func MatchesNeedles(line []byte, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	for _, n := range needles {
		if bytes.Contains(line, []byte(n)) {
			return true
		}
	}
	return false
}

// --- small JSON helpers shared by the built-in adapters ---

// str reads a string field from a decoded object.
func str(m map[string]json.RawMessage, key string) string {
	raw, ok := m[key]
	if !ok || len(raw) == 0 || raw[0] != '"' {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// obj returns a nested object.
func obj(m map[string]json.RawMessage, key string) map[string]json.RawMessage {
	raw, ok := m[key]
	if !ok || len(raw) == 0 || raw[0] != '{' {
		return nil
	}
	var out map[string]json.RawMessage
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

// i64 reads an integer field, accepting float spellings.
func i64(m map[string]json.RawMessage, key string) int64 {
	raw, ok := m[key]
	if !ok || len(raw) == 0 {
		return 0
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&n) != nil {
		return 0
	}
	if v, err := n.Int64(); err == nil {
		return v
	}
	if f, err := n.Float64(); err == nil {
		return int64(f)
	}
	return 0
}

// timeFrom parses the timestamp spellings found in agent logs.
func timeFrom(m map[string]json.RawMessage, key string) (time.Time, bool) {
	s := str(m, key)
	if s == "" {
		return time.Time{}, false
	}
	return ParseTime(s)
}

// ParseTime accepts RFC3339 (with or without fraction) and a few common layouts.
func ParseTime(s string) (time.Time, bool) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000Z0700",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
