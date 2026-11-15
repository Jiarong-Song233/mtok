package adapters

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// readFile is os.ReadFile with the error swallowed: adapters degrade silently
// when an optional config file is absent.
func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

// tomlString extracts a top-level string assignment from a TOML file without
// pulling in a TOML parser. Only simple `key = "value"` lines are matched.
func tomlString(data []byte, key string) string {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, key) {
			continue
		}
		rest := strings.TrimSpace(line[len(key):])
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		rest = strings.TrimSpace(rest[1:])
		if len(rest) >= 2 && rest[0] == '"' {
			if end := strings.IndexByte(rest[1:], '"'); end >= 0 {
				return rest[1 : 1+end]
			}
		}
	}
	return ""
}

// lookup resolves a dotted path inside a decoded JSON object. Array indexes are
// written as a[0]; nested indexes as a[0][1].
func lookup(m map[string]json.RawMessage, path string) (json.RawMessage, bool) {
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	objCur := m
	var cur json.RawMessage
	for i, part := range parts {
		name, idxs := splitIndex(part)
		if objCur == nil {
			return nil, false
		}
		raw, ok := objCur[name]
		if !ok {
			return nil, false
		}
		cur = raw
		for _, idx := range idxs {
			var arr []json.RawMessage
			if json.Unmarshal(cur, &arr) != nil || idx < 0 || idx >= len(arr) {
				return nil, false
			}
			cur = arr[idx]
		}
		if i == len(parts)-1 {
			break
		}
		objCur = nil
		if len(cur) > 0 && cur[0] == '{' {
			next := map[string]json.RawMessage{}
			if json.Unmarshal(cur, &next) == nil {
				objCur = next
			}
		}
	}
	return cur, true
}

// splitIndex splits "a[0][2]" into "a" and [0 2].
func splitIndex(part string) (string, []int) {
	open := strings.IndexByte(part, '[')
	if open < 0 {
		return part, nil
	}
	name := part[:open]
	var idxs []int
	rest := part[open:]
	for {
		i := strings.IndexByte(rest, '[')
		j := strings.IndexByte(rest, ']')
		if i < 0 || j < 0 || j < i {
			break
		}
		if n, err := strconv.Atoi(rest[i+1 : j]); err == nil {
			idxs = append(idxs, n)
		}
		rest = rest[j+1:]
	}
	return name, idxs
}

// strAt returns a string field at path.
func strAt(m map[string]json.RawMessage, path string) string {
	raw, ok := lookup(m, path)
	if !ok || len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		return ""
	}
	return strings.Trim(string(raw), " ")
}

// intAt returns an integer field at path.
func intAt(m map[string]json.RawMessage, path string) int64 {
	raw, ok := lookup(m, path)
	if !ok || len(raw) == 0 {
		return 0
	}
	var n json.Number
	dec := json.NewDecoder(strings.NewReader(string(raw)))
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

// floatAt returns a float field at path.
func floatAt(m map[string]json.RawMessage, path string) float64 {
	raw, ok := lookup(m, path)
	if !ok || len(raw) == 0 {
		return 0
	}
	f, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		return 0
	}
	return f
}
