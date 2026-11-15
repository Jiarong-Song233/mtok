package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DeclSpec is the YAML schema of a declarative adapter. Everything a new agent
// needs is expressed here; mtok never has to be rebuilt.
type DeclSpec struct {
	Name        string     `yaml:"name"`
	Agent       string     `yaml:"agent"`
	Glob        string     `yaml:"glob"`
	Globs       []string   `yaml:"globs"`
	Format      string     `yaml:"format"`   // jsonl (default) | regex
	Contains    string     `yaml:"contains"` // single prefilter needle
	ContainsAny []string   `yaml:"contains_any"`
	Regex       string     `yaml:"regex"`
	TimeLayout  string     `yaml:"time_layout"`
	Record      DeclRecord `yaml:"record"`
	Disabled    bool       `yaml:"disabled"`
}

// DeclRecord maps log fields onto mtok's canonical record.
type DeclRecord struct {
	Filter             []DeclFilter `yaml:"filter"`
	Time               DeclField    `yaml:"time"`
	Model              DeclField    `yaml:"model"`
	Input              DeclField    `yaml:"input"`
	Output             DeclField    `yaml:"output"`
	CacheRead          DeclField    `yaml:"cache_read"`
	CacheWrite         DeclField    `yaml:"cache_write"`
	InputIncludesCache bool         `yaml:"input_includes_cache"`
	// ModelFromFile keeps the last seen model for the next records.
	ModelFromFile bool `yaml:"model_from_file"`
}

// DeclFilter is a match condition on one field.
type DeclFilter struct {
	Path   string `yaml:"path"`
	Equals string `yaml:"equals"`
	Has    string `yaml:"has"`
}

// DeclField is a single value extractor.
type DeclField struct {
	Path   string `yaml:"path"`
	UnixMS bool   `yaml:"unix_ms"`
	UnixS  bool   `yaml:"unix_s"`
}

// Decl is a compiled declarative adapter.
type Decl struct {
	spec    DeclSpec
	agent   string
	globs   []string
	re      *regexp.Regexp
	needles []string
}

// LoadDir reads every *.yaml / *.yml in dir. Bad files produce partial results:
// the valid adapters still load and each problem is reported.
func LoadDir(dir, home string) ([]Adapter, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []error{err}
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".yaml" || ext == ".yml" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var out []Adapter
	var errs []error
	for _, name := range names {
		a, err := LoadFile(filepath.Join(dir, name), home)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		if a == nil {
			continue
		}
		out = append(out, a)
	}
	return out, errs
}

// LoadFile compiles one declarative adapter. A disabled adapter returns nil.
func LoadFile(path, home string) (Adapter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var spec DeclSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	if spec.Disabled {
		return nil, nil
	}
	return Compile(spec, home)
}

// Compile validates a spec and returns the adapter.
func Compile(spec DeclSpec, home string) (Adapter, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	if spec.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	globs := append([]string{}, spec.Globs...)
	if spec.Glob != "" {
		globs = append(globs, spec.Glob)
	}
	if len(globs) == 0 {
		return nil, fmt.Errorf("glob is required")
	}
	for i, g := range globs {
		globs[i] = strings.TrimSpace(g)
		if strings.HasPrefix(globs[i], "~/") {
			globs[i] = filepath.ToSlash(filepath.Join(home, globs[i][2:]))
		}
	}
	d := &Decl{spec: spec, agent: spec.Agent, globs: globs}
	if d.agent == "" {
		d.agent = spec.Name
	}
	format := strings.ToLower(strings.TrimSpace(spec.Format))
	if format == "" {
		format = "jsonl"
	}
	switch format {
	case "jsonl":
		if spec.Record.Input.Path == "" && spec.Record.Output.Path == "" &&
			spec.Record.CacheRead.Path == "" && spec.Record.CacheWrite.Path == "" {
			return nil, fmt.Errorf("record.input/output/cache_* need at least one path")
		}
	case "regex":
		if spec.Regex == "" {
			return nil, fmt.Errorf("regex format needs a regex")
		}
		re, err := regexp.Compile(spec.Regex)
		if err != nil {
			return nil, fmt.Errorf("regex: %w", err)
		}
		d.re = re
	default:
		return nil, fmt.Errorf("unsupported format %q (want jsonl or regex)", spec.Format)
	}
	if spec.Contains != "" {
		d.needles = append(d.needles, spec.Contains)
	}
	d.needles = append(d.needles, spec.ContainsAny...)
	return d, nil
}

// Name implements Adapter.
func (d *Decl) Name() string { return d.spec.Name }

// Agent implements Adapter.
func (d *Decl) Agent() string { return d.agent }

// Globs implements Adapter.
func (d *Decl) Globs() []string { return d.globs }

// Needles implements Prefilterer.
func (d *Decl) Needles() []string { return d.needles }

// ParseLine implements Adapter.
func (d *Decl) ParseLine(line []byte, ctx *Context) []Record {
	if d.re != nil {
		return d.ParseRegex(string(line), ctx)
	}
	if len(line) == 0 || line[0] != '{' {
		return nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	spec := d.spec
	for _, f := range spec.Record.Filter {
		v := strAt(m, f.Path)
		if f.Equals != "" && v != f.Equals {
			return nil
		}
		if f.Has != "" && !strings.Contains(v, f.Has) {
			return nil
		}
	}
	model := strAt(m, spec.Record.Model.Path)
	if spec.Record.ModelFromFile {
		if model != "" {
			ctx.Model = model
		} else {
			model = ctx.Model
		}
	}
	if model == "" {
		model = ctx.Model
	}
	in := intAt(m, spec.Record.Input.Path)
	cr := intAt(m, spec.Record.CacheRead.Path)
	cw := intAt(m, spec.Record.CacheWrite.Path)
	if spec.Record.InputIncludesCache {
		in -= cr + cw
		if in < 0 {
			in = 0
		}
	}
	t := fieldTime(m, spec.Record.Time, spec.TimeLayout)
	return []Record{{
		Time:       t,
		Model:      model,
		Input:      in,
		Output:     intAt(m, spec.Record.Output.Path),
		CacheRead:  cr,
		CacheWrite: cw,
	}}
}

// ParseRegex applies a regex-format declarative adapter.
func (d *Decl) ParseRegex(line string, ctx *Context) []Record {
	if d.re == nil {
		return nil
	}
	names := d.re.SubexpNames()
	m := d.re.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	get := func(name string) string {
		for i, n := range names {
			if n == name && i < len(m) {
				return m[i]
			}
		}
		return ""
	}
	num := func(name string) int64 {
		s := strings.TrimSpace(get(name))
		if s == "" {
			return 0
		}
		var v int64
		if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
			var f float64
			if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
				return int64(f)
			}
			return 0
		}
		return v
	}
	t := time.Time{}
	if raw := get("time"); raw != "" {
		if d.spec.TimeLayout != "" {
			if parsed, err := time.Parse(d.spec.TimeLayout, raw); err == nil {
				t = parsed
			}
		} else if parsed, ok := ParseTime(raw); ok {
			t = parsed
		}
	}
	model := get("model")
	if model == "" {
		model = ctx.Model
	}
	in := num("input")
	cr := num("cache_read")
	cw := num("cache_write")
	if d.spec.Record.InputIncludesCache {
		in -= cr + cw
		if in < 0 {
			in = 0
		}
	}
	return []Record{{
		Time:       t,
		Model:      model,
		Input:      in,
		Output:     num("output"),
		CacheRead:  cr,
		CacheWrite: cw,
	}}
}

func fieldTime(m map[string]json.RawMessage, f DeclField, layout string) time.Time {
	switch {
	case f.UnixMS:
		if ms := intAt(m, f.Path); ms > 0 {
			return time.UnixMilli(ms)
		}
	case f.UnixS:
		if s := floatAt(m, f.Path); s > 0 {
			return time.Unix(int64(s), 0)
		}
	default:
		raw := strAt(m, f.Path)
		if raw == "" {
			return time.Time{}
		}
		if layout != "" {
			if t, err := time.Parse(layout, raw); err == nil {
				return t
			}
		}
		if t, ok := ParseTime(raw); ok {
			return t
		}
	}
	return time.Time{}
}
