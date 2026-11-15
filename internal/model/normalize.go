// Package model normalizes vendor-specific model identifiers into one
// canonical display name so usage from different agents merges into one row.
package model

import (
	"strings"
	"unicode"
)

// Key folds an identifier into a lookup key: lower case, alphanumerics only.
// "provider/DeepSeek-V4_1~Flash" and "deepseekv41flash" collapse to the same key.
func Key(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < unicode.MaxASCII {
			switch {
			case r >= '0' && r <= '9', r >= 'a' && r <= 'z':
				b.WriteRune(r)
			case r >= 'A' && r <= 'Z':
				b.WriteRune(r + 32)
			}
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// Normalizer resolves raw identifiers to canonical names. A zero Normalizer is
// not usable; build one with New.
type Normalizer struct {
	canon map[string]string // key -> canonical display name
}

// New returns a Normalizer seeded with the built-in table.
func New() *Normalizer {
	n := &Normalizer{canon: make(map[string]string, 512)}
	n.load(builtinAliases)
	return n
}

// Add maps extra identifiers onto canonical. Keys are folded with Key first, so
// user config can be written with any separator style.
func (n *Normalizer) Add(canonical string, aliases ...string) {
	n.canon[Key(canonical)] = canonical
	for _, a := range aliases {
		if a == "" {
			continue
		}
		n.canon[Key(a)] = canonical
	}
}

func (n *Normalizer) load(m map[string][]string) {
	for canonical, aliases := range m {
		n.Add(canonical, aliases...)
	}
}

// Resolve maps one raw identifier to its canonical display name. Unknown names
// fall back to a prettified form so they still render on one line.
func (n *Normalizer) Resolve(raw string) string {
	s := clean(raw)
	if s == "" {
		return "Unknown"
	}
	if c, ok := n.canon[Key(s)]; ok {
		return c
	}
	return Prettify(s)
}

// Known reports whether raw resolves through the alias table (no prettify).
func (n *Normalizer) Known(raw string) bool {
	_, ok := n.canon[Key(clean(raw))]
	return ok
}

// clean removes decorations that never carry model identity: a bracketed
// prefix, a per-token price suffix such as "@5k", tier suffixes in parens and
// any provider path.
func clean(s string) string {
	s = strings.TrimSpace(s)
	for {
		if !strings.HasPrefix(s, "[") {
			break
		}
		i := strings.IndexByte(s, ']')
		if i < 0 {
			break
		}
		s = strings.TrimSpace(s[i+1:])
	}
	if i := strings.IndexByte(s, '('); i >= 0 && strings.HasSuffix(s, ")") {
		s = strings.TrimSpace(s[:i])
	}
	if i := strings.LastIndexByte(s, '@'); i > 0 {
		tail := s[i+1:]
		if tail != "" && strings.Trim(tail, "0123456789.kmKM") == "" {
			s = strings.TrimSpace(s[:i])
		}
	}
	// Dated snapshots such as gpt-4.1-2025-04-14 are the same model as gpt-4.1.
	s = trimDateSuffix(s)
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = strings.TrimSpace(s[i+1:])
	}
	return s
}

// trimDateSuffix removes a trailing -YYYY-MM-DD snapshot stamp.
func trimDateSuffix(s string) string {
	if len(s) < 11 {
		return s
	}
	tail := s[len(s)-10:]
	if tail[4] != '-' || tail[7] != '-' || s[len(s)-11] != '-' {
		return s
	}
	for i, r := range tail {
		if i == 4 || i == 7 {
			continue
		}
		if r < '0' || r > '9' {
			return s
		}
	}
	return s[:len(s)-11]
}

var capWords = map[string]string{
	"gpt": "GPT", "glm": "GLM", "ai": "AI", "api": "API", "sdk": "SDK",
	"llm": "LLM", "mimo": "MiMo", "deepseek": "DeepSeek", "qwen": "Qwen",
	"kimi": "Kimi", "minimax": "MiniMax", "claude": "Claude", "gemini": "Gemini",
	"grok": "Grok", "hy": "Hy", "step": "Step", "sensenova": "SenseNova",
	"codex": "Codex", "sonnet": "Sonnet", "opus": "Opus", "haiku": "Haiku",
	"flash": "Flash", "pro": "Pro", "max": "Max", "mini": "Mini", "turbo": "Turbo",
	"preview": "Preview", "lite": "Lite", "reasoner": "Reasoner", "chat": "Chat",
	"coder": "Coder", "vl": "VL", "moe": "MoE", "it": "IT", "v": "v",
}

// Prettify title-cases a loose identifier: "deepseek_v4_1_flash" -> "DeepSeek-V4.1-Flash".
func Prettify(name string) string {
	parts := splitIdent(clean(name))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		low := strings.ToLower(p)
		if strings.HasPrefix(low, "v") && isDigits(low[1:]) {
			out = append(out, low)
			continue
		}
		if up, ok := capWords[low]; ok {
			out = append(out, up)
			continue
		}
		if isDigits(p) {
			if len(out) > 0 && endsWithDigit(out[len(out)-1]) {
				out[len(out)-1] += "." + p
				continue
			}
			out = append(out, p)
			continue
		}
		out = append(out, upperFirst(p))
	}
	if len(out) == 0 {
		return "Unknown"
	}
	return strings.Join(out, "-")
}

// splitIdent splits on separators and between letters and digits, then folds
// "v41" into "v4 1" so version numbers get dotted later.
func splitIdent(s string) []string {
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '-' || r == '_' || r == '.' || r == '~' || r == ' ' || r == ':' {
			flush()
			continue
		}
		if cur.Len() > 0 {
			prev := rs[i-1]
			if unicode.IsDigit(r) != unicode.IsDigit(prev) {
				flush()
			}
		}
		cur.WriteRune(r)
	}
	flush()
	// Join a lone "v" with following digits, then treat a multi-digit run as a
	// version: v, 41 -> v4, 1.
	for i := 0; i < len(parts)-1; i++ {
		if (parts[i] == "v" || parts[i] == "V") && isDigits(parts[i+1]) {
			parts[i] = "v" + parts[i+1]
			parts = append(parts[:i+1], parts[i+2:]...)
		}
	}
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) > 2 && p[0] == 'v' && isDigits(p[1:]) {
			parts[i] = p[:2]
			parts = append(parts[:i+1], append([]string{p[2:]}, parts[i+1:]...)...)
			i++
		}
	}
	return parts
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func endsWithDigit(s string) bool {
	if s == "" {
		return false
	}
	r := rune(s[len(s)-1])
	return r >= '0' && r <= '9'
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if len(r) == 1 {
		return string(unicode.ToUpper(r[0]))
	}
	r[0] = unicode.ToUpper(r[0])
	for i := 1; i < len(r); i++ {
		r[i] = unicode.ToLower(r[i])
	}
	return string(r)
}
