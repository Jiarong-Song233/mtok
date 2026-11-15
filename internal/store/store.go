// Package store keeps the aggregated usage counters plus the per-file ingest
// offsets that make incremental scans possible.
package store

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Version is bumped whenever State changes shape; an older file is discarded
// and rebuilt from the logs.
const Version = 4

// FileState records how far a log file has been consumed.
type FileState struct {
	Size  int64  `json:"s"`
	MTime int64  `json:"m"`
	Off   int64  `json:"o"`
	Model string `json:"k,omitempty"` // adapter carry-over (e.g. codex turn model)
	Done  bool   `json:"d,omitempty"` // whole-file-only source (compressed archives)
}

// SourceMagpie marks rows recovered from the magpie proxy journal instead of an
// agent's own log.
const SourceMagpie = "magpie"

// Row is one (day, model, agent) aggregate. Token counts follow the canonical
// convention: Input excludes cached input, so rows from different vendors add up.
//
// Source is empty for rows parsed from an agent's local log and SourceMagpie for
// rows parsed from the proxy journal. Routed marks local rows whose model name
// was only a proxy route (auto/auto, group/*, magpie/*): those carry no model
// identity, so the journal's detail replaces them whenever it covers the day.
type Row struct {
	Day        string `json:"day"`
	Model      string `json:"model"`
	Agent      string `json:"agent"`
	Source     string `json:"src,omitempty"`
	Routed     bool   `json:"routed,omitempty"`
	Input      int64  `json:"in"`
	Output     int64  `json:"out"`
	CacheRead  int64  `json:"cr"`
	CacheWrite int64  `json:"cw"`
	Calls      int64  `json:"n"`
}

// Total is the sum of all token fields of a row.
func (r Row) Total() int64 { return r.Input + r.Output + r.CacheRead + r.CacheWrite }

// FromMagpie reports whether the row came from the proxy journal.
func (r Row) FromMagpie() bool { return r.Source == SourceMagpie }

// CoverageKey identifies the (day, agent) pair a row belongs to; it is the unit
// used to decide whether the proxy journal already covers a routed local row.
func CoverageKey(day, agent string) string { return day + "\x1f" + agent }

func rowKey(r Row) string {
	key := r.Day + "\x1f" + r.Model + "\x1f" + r.Agent
	if r.Source != "" {
		key += "\x1f" + r.Source
	}
	if r.Routed {
		key += "\x1froute"
	}
	return key
}

// State is the whole database. It stays small enough (kilobytes to a few MB)
// that snapshot-rewrite is simpler and faster than a page-based engine.
type State struct {
	Version int                  `json:"v"`
	Meta    map[string]string    `json:"meta"`
	Files   map[string]FileState `json:"files"`
	Rows    map[string]*Row      `json:"rows"`
}

// Store is a State plus its on-disk path.
type Store struct {
	path  string
	state State
	dirty bool
	lock  *os.File
}

// Open loads the store at path, creating an empty one when missing. A corrupt
// file is moved aside instead of failing the whole command.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	s.state = State{
		Version: Version,
		Meta:    map[string]string{},
		Files:   map[string]FileState{},
		Rows:    map[string]*Row{},
	}
	if err := s.lockFor(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		s.corrupt()
		return s, nil
	}
	defer zr.Close()
	var st State
	if err := json.NewDecoder(zr).Decode(&st); err != nil {
		s.corrupt()
		return s, nil
	}
	if st.Version != Version {
		// Same file, older schema: keep it on disk (it is still readable by an
		// older mtok) and simply start a fresh in-memory state that the next
		// Save replaces.
		s.dirty = true
		return s, nil
	}
	if st.Files != nil {
		s.state.Files = st.Files
	}
	if st.Rows != nil {
		s.state.Rows = st.Rows
	}
	if st.Meta != nil {
		s.state.Meta = st.Meta
	}
	return s, nil
}

func (s *Store) corrupt() {
	_ = os.Rename(s.path, s.path+".corrupt")
	s.state = State{
		Version: Version,
		Meta:    map[string]string{},
		Files:   map[string]FileState{},
		Rows:    map[string]*Row{},
	}
	s.dirty = true
}

// lockFor serializes writers so two concurrent mtok runs cannot clobber each
// other. Failure to lock is not fatal: the snapshot write stays atomic.
func (s *Store) lockFor() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	lp := s.path + ".lock"
	for i := 0; i < 20; i++ {
		f, err := os.OpenFile(lp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d %d\n", os.Getpid(), time.Now().Unix())
			s.lock = f
			return nil
		}
		if st, err := os.Stat(lp); err == nil && time.Since(st.ModTime()) > 10*time.Second {
			_ = os.Remove(lp) // stale lock from a killed run
			continue
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil
}

// Close releases the lock. Call after Save.
func (s *Store) Close() {
	if s.lock != nil {
		_ = s.lock.Close()
		_ = os.Remove(s.path + ".lock")
		s.lock = nil
	}
}

// Save writes the snapshot atomically when something changed.
func (s *Store) Save() error {
	if !s.dirty {
		return nil
	}
	s.state.Version = Version
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	enc := json.NewEncoder(zw)
	enc.SetIndent("", "")
	werr := enc.Encode(&s.state)
	if werr == nil {
		werr = zw.Close()
	}
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return werr
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	s.dirty = false
	return nil
}

// Reset drops every counter and offset (--rebuild).
func (s *Store) Reset() {
	s.state.Rows = map[string]*Row{}
	s.state.Files = map[string]FileState{}
	s.state.Meta = map[string]string{}
	s.dirty = true
}

// GetMeta returns a meta value.
func (s *Store) GetMeta(k string) string { return s.state.Meta[k] }

// SetMeta stores a meta value.
func (s *Store) SetMeta(k, v string) {
	if s.state.Meta[k] == v {
		return
	}
	s.state.Meta[k] = v
	s.dirty = true
}

// Path is the snapshot location.
func (s *Store) Path() string { return s.path }

// FileState returns the recorded ingest position for path.
func (s *Store) FileState(path string) (FileState, bool) {
	fs, ok := s.state.Files[path]
	return fs, ok
}

// SetFileState records the ingest position for path.
func (s *Store) SetFileState(path string, fs FileState) {
	if old, ok := s.state.Files[path]; ok && old == fs {
		return
	}
	s.state.Files[path] = fs
	s.dirty = true
}

// PruneFiles drops offsets of files that no longer exist. It returns how many
// entries were removed.
func (s *Store) PruneFiles(seen map[string]bool) int {
	n := 0
	for p := range s.state.Files {
		if !seen[p] {
			delete(s.state.Files, p)
			n++
		}
	}
	if n > 0 {
		s.dirty = true
	}
	return n
}

// Add merges one record into the aggregate.
func (s *Store) Add(r Row) {
	if r.Input == 0 && r.Output == 0 && r.CacheRead == 0 && r.CacheWrite == 0 {
		return
	}
	k := rowKey(r)
	cur, ok := s.state.Rows[k]
	if !ok {
		c := r
		s.state.Rows[k] = &c
		s.dirty = true
		return
	}
	cur.Input += r.Input
	cur.Output += r.Output
	cur.CacheRead += r.CacheRead
	cur.CacheWrite += r.CacheWrite
	cur.Calls += r.Calls
	s.dirty = true
}

// Rows returns every aggregate row sorted by day then model.
func (s *Store) Rows() []Row {
	out := make([]Row, 0, len(s.state.Rows))
	for _, r := range s.state.Rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Day != out[j].Day {
			return out[i].Day < out[j].Day
		}
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].Agent < out[j].Agent
	})
	return out
}

// FileCount reports how many files carry an offset.
func (s *Store) FileCount() int { return len(s.state.Files) }

// Days lists the distinct days present in the store.
func (s *Store) Days() []string {
	set := map[string]bool{}
	for _, r := range s.state.Rows {
		set[r.Day] = true
	}
	out := make([]string, 0, len(set))
	for d := range set {
		if d != "" {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// Stats summarises the snapshot for `mtok config`.
func (s *Store) Stats() (rows int, files int, bytes int64) {
	if st, err := os.Stat(s.path); err == nil {
		bytes = st.Size()
	}
	return len(s.state.Rows), len(s.state.Files), bytes
}
