// Package ingest walks the adapter globs, reads only the bytes appended since
// the last run, and folds the records into the aggregate store.
package ingest

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/songjiarong/mtok/internal/adapters"
	"github.com/songjiarong/mtok/internal/globx"
	"github.com/songjiarong/mtok/internal/model"
	"github.com/songjiarong/mtok/internal/store"
)

// Options controls one ingest pass.
type Options struct {
	Home     string
	Zstd     bool
	Rebuild  bool
	Disabled map[string]bool
	Workers  int
}

// AdapterStat reports what one adapter did in this pass.
type AdapterStat struct {
	Name        string
	Globs       []string
	Files       int
	Changed     int
	Bytes       int64
	Lines       int64
	Records     int64
	NoTime      int64
	Errors      int64
	Skipped     int
	Models      []string
	LastErr     string
	ElapsedMS   int64
	Unsupported bool
	Reason      string
}

// Result is the outcome of a pass.
type Result struct {
	Stats    []AdapterStat
	Files    int
	Changed  int
	Records  int64
	Bytes    int64
	Elapsed  time.Duration
	Pruned   int
	Rebuilt  bool
	Disabled []string
}

type adapterOut struct {
	stat  AdapterStat
	rows  map[string]store.Row
	files map[string]store.FileState
	seen  map[string]bool
}

// Run executes one ingest pass. Adapters are scanned concurrently; results are
// merged into the store on the calling goroutine so no locking is needed.
func Run(st *store.Store, ads []adapters.Adapter, norm *model.Normalizer, opt Options) (Result, error) {
	start := time.Now()
	res := Result{Rebuilt: opt.Rebuild}
	if opt.Rebuild {
		st.Reset()
	}
	var active []adapters.Adapter
	for _, a := range ads {
		if opt.Disabled[a.Name()] {
			res.Disabled = append(res.Disabled, a.Name())
			continue
		}
		active = append(active, a)
	}
	workers := opt.Workers
	if workers <= 0 {
		workers = 4
	}
	if workers > len(active) && len(active) > 0 {
		workers = len(active)
	}
	outs := make([]adapterOut, len(active))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for i, a := range active {
		wg.Add(1)
		go func(i int, a adapters.Adapter) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			outs[i] = scanAdapter(st, a, norm, opt)
		}(i, a)
	}
	wg.Wait()

	seen := map[string]bool{}
	for _, o := range outs {
		res.Stats = append(res.Stats, o.stat)
		res.Files += o.stat.Files
		res.Changed += o.stat.Changed
		res.Records += o.stat.Records
		res.Bytes += o.stat.Bytes
		for _, r := range o.rows {
			st.Add(r)
		}
		for p, fs := range o.files {
			st.SetFileState(p, fs)
			seen[p] = true
		}
		for p := range o.seen {
			seen[p] = true
		}
	}
	res.Pruned = st.PruneFiles(seen)
	st.SetMeta("last_sync", time.Now().Format(time.RFC3339))
	res.Elapsed = time.Since(start)
	sort.Slice(res.Stats, func(i, j int) bool { return res.Stats[i].Name < res.Stats[j].Name })
	return res, nil
}

// scanAdapter finds files, reads new bytes and aggregates rows in memory.
func scanAdapter(st *store.Store, a adapters.Adapter, norm *model.Normalizer, opt Options) adapterOut {
	out := adapterOut{
		rows:  map[string]store.Row{},
		files: map[string]store.FileState{},
		seen:  map[string]bool{},
	}
	started := time.Now()
	out.stat.Name = a.Name()
	out.stat.Globs = a.Globs()
	models := map[string]bool{}

	for _, pattern := range a.Globs() {
		for _, path := range globx.Expand(pattern, opt.Home) {
			out.seen[path] = true
			out.stat.Files++
			prev, existed := st.FileState(path)
			info, err := os.Stat(path)
			if err != nil {
				out.stat.Errors++
				out.stat.LastErr = err.Error()
				continue
			}
			compressed := isCompressed(path)
			if compressed && !opt.Zstd {
				out.stat.Skipped++
				continue
			}
			size, mtime := info.Size(), info.ModTime().UnixNano()
			if existed && prev.Size == size && prev.MTime == mtime {
				continue
			}
			if compressed {
				// Archives cannot be seeked: parse once, never again, so a
				// grown frame can never be counted twice.
				if existed && prev.Done {
					out.stat.Skipped++
					continue
				}
			}
			out.stat.Changed++
			ctx := &adapters.Context{Path: path}
			offset := int64(0)
			if existed && !compressed {
				offset = prev.Off
				ctx.Model = prev.Model
			}
			if offset > size {
				offset = 0 // truncated or rotated
			}
			read, lines, recs, notime, err := readFile(path, offset, compressed, a, ctx, func(r adapters.Record) {
				mergeRecord(out.rows, r, a, norm, models)
			})
			if err != nil {
				out.stat.Errors++
				out.stat.LastErr = err.Error()
			}
			out.stat.Bytes += read
			out.stat.Lines += lines
			out.stat.Records += recs
			out.stat.NoTime += notime
			fs := store.FileState{Size: size, MTime: mtime, Model: ctx.Model}
			if compressed {
				fs.Done = true
				fs.Off = size
			} else {
				fs.Off = offset + read
			}
			out.files[path] = fs
		}
	}
	for m := range models {
		out.stat.Models = append(out.stat.Models, m)
	}
	sort.Strings(out.stat.Models)
	out.stat.ElapsedMS = time.Since(started).Milliseconds()
	return out
}

// readFile consumes a file, calling emit for each record. It returns the number
// of bytes consumed (excluding an incomplete trailing line), line count and the
// number of records/time-less records seen.
func readFile(path string, offset int64, compressed bool, a adapters.Adapter, ctx *adapters.Context, emit func(adapters.Record)) (int64, int64, int64, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	defer f.Close()
	var src io.Reader = f
	var dec *zstd.Decoder
	if compressed {
		if offset > 0 {
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				return 0, 0, 0, 0, err
			}
		}
		dec, err = zstd.NewReader(f, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true))
		if err != nil {
			return 0, 0, 0, 0, err
		}
		defer dec.Close()
		src = dec
	} else if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return 0, 0, 0, 0, err
		}
	}
	needles := adapters.Needles(a)
	br := bufio.NewReaderSize(src, 256*1024)
	var consumed, lines, recs, notime int64
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			consumed += int64(len(line))
			lines++
			body := bytes.TrimRight(line, "\r\n")
			if len(body) > 0 && adapters.MatchesNeedles(body, needles) {
				for _, r := range a.ParseLine(body, ctx) {
					if r.Agent == "" {
						r.Agent = a.Agent()
					}
					if r.Time.IsZero() {
						notime++
					}
					emit(r)
					recs++
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return consumed, lines, recs, notime, err
		}
	}
	return consumed, lines, recs, notime, nil
}

// mergeRecord canonicalizes the model, attributes the day in local time and
// folds the record into the per-adapter aggregate.
func mergeRecord(rows map[string]store.Row, r adapters.Record, a adapters.Adapter, norm *model.Normalizer, models map[string]bool) {
	if r.Input == 0 && r.Output == 0 && r.CacheRead == 0 && r.CacheWrite == 0 {
		return
	}
	if r.Time.IsZero() {
		// No usable timestamp: attribute to the scan day rather than dropping
		// the usage. `mtok doctor` reports how often this happens.
		r.Time = time.Now()
	}
	raw := r.Model
	if strings.TrimSpace(raw) == "" {
		raw = "unknown"
	}
	name := norm.Resolve(raw)
	models[name] = true
	agent := r.Agent
	if agent == "" {
		agent = a.Agent()
	}
	day := r.Time.Local().Format("2006-01-02")
	k := day + "\x1f" + name + "\x1f" + agent
	row, ok := rows[k]
	if !ok {
		row = store.Row{Day: day, Model: name, Agent: agent}
	}
	row.Input += r.Input
	row.Output += r.Output
	row.CacheRead += r.CacheRead
	row.CacheWrite += r.CacheWrite
	row.Calls++
	rows[k] = row
}

// isCompressed reports whether a log file needs whole-file decoding.
func isCompressed(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".zst" || ext == ".zstd"
}

// Summary renders a one-line description of a pass, used by `mtok config`.
func (r Result) Summary() string {
	if r.Changed == 0 {
		return fmt.Sprintf("scan: %d files, no new bytes in %dms", r.Files, r.Elapsed.Milliseconds())
	}
	return fmt.Sprintf("scan: %d files (+%d changed), %s read, %d records in %dms",
		r.Files, r.Changed, HumanBytes(r.Bytes), r.Records, r.Elapsed.Milliseconds())
}

// HumanBytes formats a byte count.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGT"[exp])
}
