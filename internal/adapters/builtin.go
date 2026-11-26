package adapters

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Claude parses Claude Code transcripts. Each assistant message carries a
// message.usage block whose input_tokens already excludes cached tokens.
type Claude struct{ home string }

// NewClaude returns the Claude Code adapter.
func NewClaude(home string) *Claude { return &Claude{home: home} }

// Name implements Adapter.
func (c *Claude) Name() string { return "claude" }

// Agent implements Adapter.
func (c *Claude) Agent() string { return "claude" }

// Globs implements Adapter.
func (c *Claude) Globs() []string {
	return []string{filepath.ToSlash(filepath.Join(c.home, ".claude", "projects", "**", "*.jsonl"))}
}

// Needles implements Prefilterer.
func (c *Claude) Needles() []string { return []string{`"usage"`} }

// ParseLine implements Adapter.
func (c *Claude) ParseLine(line []byte, ctx *Context) []Record {
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	if str(m, "type") != "assistant" {
		return nil
	}
	msg := obj(m, "message")
	if msg == nil {
		return nil
	}
	usage := obj(msg, "usage")
	if usage == nil {
		return nil
	}
	// A retried request can repeat the same message; count it once.
	if !ctx.MarkSeen(str(msg, "id") + "|" + str(m, "requestId")) {
		return nil
	}
	model := str(msg, "model")
	if model == "" || model == "<synthetic>" {
		// Synthetic turns are generated locally and consume no model tokens.
		return nil
	}
	t, _ := timeFrom(m, "timestamp")
	return []Record{{
		Time:       t,
		Model:      model,
		Input:      i64(usage, "input_tokens"),
		Output:     i64(usage, "output_tokens"),
		CacheRead:  i64(usage, "cache_read_input_tokens"),
		CacheWrite: i64(usage, "cache_creation_input_tokens"),
	}}
}

// Codex parses OpenAI Codex rollouts. Token accounting lives in token_count
// events that report a per-call delta; the model is announced once per turn in
// a turn_context event, so it is carried in the scan context.
type Codex struct{ home string }

// NewCodex returns the Codex adapter.
func NewCodex(home string) *Codex { return &Codex{home: home} }

// Name implements Adapter.
func (c *Codex) Name() string { return "codex" }

// Agent implements Adapter.
func (c *Codex) Agent() string { return "codex" }

// Globs implements Adapter.
func (c *Codex) Globs() []string {
	return []string{
		filepath.ToSlash(filepath.Join(c.home, ".codex", "sessions", "**", "rollout-*.jsonl")),
		filepath.ToSlash(filepath.Join(c.home, ".codex", "sessions", "**", "rollout-*.jsonl.zst")),
	}
}

// Needles implements Prefilterer.
func (c *Codex) Needles() []string { return []string{`"token_count"`, `"turn_context"`} }

// ParseLine implements Adapter.
func (c *Codex) ParseLine(line []byte, ctx *Context) []Record {
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	payload := obj(m, "payload")
	if payload == nil {
		return nil
	}
	switch str(m, "type") {
	case "turn_context", "session_meta":
		if model := str(payload, "model"); model != "" {
			ctx.Model = model
		}
		return nil
	case "event_msg":
		if str(payload, "type") != "token_count" {
			return nil
		}
		info := obj(payload, "info")
		if info == nil {
			return nil
		}
		last := obj(info, "last_token_usage")
		if last == nil {
			// Older rollouts only expose the cumulative counter; without a
			// per-call delta counting it would double count every turn.
			return nil
		}
		cached := i64(last, "cached_input_tokens")
		input := i64(last, "input_tokens") - cached // input_tokens includes cached
		if input < 0 {
			input = 0
		}
		t, _ := timeFrom(m, "timestamp")
		return []Record{{
			Time:       t,
			Model:      ctx.Model,
			Input:      input,
			Output:     i64(last, "output_tokens"),
			CacheRead:  cached,
			CacheWrite: i64(last, "cache_write_input_tokens"),
		}}
	}
	return nil
}

// Grok parses Grok Build's unified log. Inference completions report
// prompt/completion tokens; the model is whatever the shell last selected.
type Grok struct {
	home   string
	once   bool
	defMod string
}

// NewGrok returns the Grok Build adapter.
func NewGrok(home string) *Grok { return &Grok{home: home} }

// Name implements Adapter.
func (g *Grok) Name() string { return "grok" }

// Agent implements Adapter.
func (g *Grok) Agent() string { return "grok" }

// Globs implements Adapter.
func (g *Grok) Globs() []string {
	return []string{filepath.ToSlash(filepath.Join(g.home, ".grok", "logs", "*.jsonl"))}
}

// Needles implements Prefilterer.
func (g *Grok) Needles() []string { return []string{"inference_done", "model changed"} }

// DefaultModel reads [models] default from ~/.grok/config.toml once, used for
// records written before the first "model changed" event.
func (g *Grok) DefaultModel() string {
	if g.once {
		return g.defMod
	}
	g.once = true
	data, err := readFile(filepath.Join(g.home, ".grok", "config.toml"))
	if err != nil {
		return ""
	}
	g.defMod = tomlString(data, "default")
	return g.defMod
}

// ParseLine implements Adapter.
func (g *Grok) ParseLine(line []byte, ctx *Context) []Record {
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	body := obj(m, "ctx")
	if body == nil {
		return nil
	}
	switch str(m, "msg") {
	case "model changed":
		if model := str(body, "model"); model != "" {
			ctx.Model = model
		}
		return nil
	case "shell.turn.inference_done":
		if ctx.Model == "" {
			ctx.Model = g.DefaultModel()
		}
		cached := i64(body, "cached_prompt_tokens")
		input := i64(body, "prompt_tokens") - cached // prompt_tokens includes cached
		if input < 0 {
			input = 0
		}
		t, _ := timeFrom(m, "ts")
		return []Record{{
			Time:      t,
			Model:     ctx.Model,
			Input:     input,
			Output:    i64(body, "completion_tokens"),
			CacheRead: cached,
		}}
	}
	return nil
}

// Gemini parses Antigravity planner responses. The log has no model field, so
// the "Model Selection" notice inside user turns is used as the current model.
type Gemini struct{ home string }

// NewGemini returns the Antigravity adapter.
func NewGemini(home string) *Gemini { return &Gemini{home: home} }

// Name implements Adapter.
func (g *Gemini) Name() string { return "gemini" }

// Agent implements Adapter.
func (g *Gemini) Agent() string { return "antigravity" }

// Globs implements Adapter. Only the full transcript is read; the chunked and
// non-full copies contain the same records. Both the original Antigravity app
// and the newer antigravity-cli keep transcripts under .gemini.
func (g *Gemini) Globs() []string {
	return []string{
		filepath.ToSlash(filepath.Join(g.home, ".gemini", "antigravity", "brain", "*", ".system_generated", "logs", "transcript_full.jsonl")),
		filepath.ToSlash(filepath.Join(g.home, ".gemini", "antigravity-cli", "brain", "*", ".system_generated", "logs", "transcript_full.jsonl")),
	}
}

// Needles implements Prefilterer.
func (g *Gemini) Needles() []string { return []string{`"PLANNER_RESPONSE"`, `Model Selection`} }

// ParseLine implements Adapter.
func (g *Gemini) ParseLine(line []byte, ctx *Context) []Record {
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	switch str(m, "type") {
	case "USER_INPUT":
		if model := modelSelection(str(m, "content")); model != "" {
			ctx.Model = model
		}
		return nil
	case "PLANNER_RESPONSE":
		in, out := i64(m, "input_tokens"), i64(m, "output_tokens")
		cr := i64(m, "cache_read_tokens")
		if in+out+cr == 0 {
			return nil
		}
		// antigravity-cli records the model on every response; the original
		// Antigravity app omits it, so fall back to the Model Selection notice.
		model := str(m, "model")
		if model != "" {
			model = stripGeminiThinking(model)
			ctx.Model = model
		} else {
			model = ctx.Model
		}
		t, _ := timeFrom(m, "created_at")
		return []Record{{
			Time:      t,
			Model:     model,
			Input:     in,
			Output:    out,
			CacheRead: cr,
		}}
	}
	return nil
}

// modelSelection extracts the target of a "Model Selection ... to X." notice.
func modelSelection(content string) string {
	const marker = "Model Selection"
	i := indexOf(content, marker)
	if i < 0 {
		return ""
	}
	rest := content[i:]
	j := indexOf(rest, " to ")
	if j < 0 {
		return ""
	}
	rest = rest[j+4:]
	end := len(rest)
	for k := 0; k < len(rest); k++ {
		r := rest[k]
		// A period only ends the sentence when it is followed by a space or
		// the end; "Gemini 3.8" keeps its version dot.
		if r == '\n' || r == '`' {
			end = k
			break
		}
		if r == '.' && (k+1 == len(rest) || rest[k+1] == ' ' || rest[k+1] == '\n') {
			end = k
			break
		}
	}
	return trimSpace(rest[:end])
}

// stripGeminiThinking drops the trailing thinking-level segment Antigravity
// appends to Gemini slugs (gemini-4-hydrogen-high -> gemini-4-hydrogen), so the
// same model at different reasoning levels merges into one row. Only Gemini
// slugs are touched; claude-opus-4-6-thinking keeps its own suffix.
func stripGeminiThinking(model string) string {
	if !strings.HasPrefix(strings.ToLower(model), "gemini") {
		return model
	}
	for _, s := range []string{"-xhigh", "-high", "-medium", "-low"} {
		if strings.HasSuffix(model, s) {
			return strings.TrimSuffix(model, s)
		}
	}
	return model
}

// Kimi parses Kimi CLI wire logs. Per-step token counts use input_other for
// uncached input; the model is not recorded, so the CLI default model is used.
type Kimi struct {
	home   string
	once   bool
	defMod string
}

// NewKimi returns the Kimi adapter.
func NewKimi(home string) *Kimi { return &Kimi{home: home} }

// Name implements Adapter.
func (k *Kimi) Name() string { return "kimi" }

// Agent implements Adapter.
func (k *Kimi) Agent() string { return "kimi" }

// Globs implements Adapter.
func (k *Kimi) Globs() []string {
	return []string{filepath.ToSlash(filepath.Join(k.home, ".kimi", "sessions", "**", "wire.jsonl"))}
}

// Needles implements Prefilterer.
func (k *Kimi) Needles() []string { return []string{`"token_usage"`} }

// DefaultModel reads default_model from ~/.kimi/config.toml once.
func (k *Kimi) DefaultModel() string {
	if k.once {
		return k.defMod
	}
	k.once = true
	data, err := readFile(filepath.Join(k.home, ".kimi", "config.toml"))
	if err != nil {
		return ""
	}
	k.defMod = tomlString(data, "default_model")
	return k.defMod
}

// ParseLine implements Adapter.
func (k *Kimi) ParseLine(line []byte, ctx *Context) []Record {
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	msg := obj(m, "message")
	if msg == nil || str(msg, "type") != "StatusUpdate" {
		return nil
	}
	payload := obj(msg, "payload")
	if payload == nil {
		return nil
	}
	usage := obj(payload, "token_usage")
	if usage == nil {
		return nil
	}
	if ctx.Model == "" {
		ctx.Model = k.DefaultModel()
	}
	var t time.Time
	if secs := f64(m, "timestamp"); secs > 0 {
		t = time.Unix(int64(secs), int64((secs-float64(int64(secs)))*1e9))
	}
	return []Record{{
		Time:       t,
		Model:      ctx.Model,
		Input:      i64(usage, "input_other"),
		Output:     i64(usage, "output"),
		CacheRead:  i64(usage, "input_cache_read"),
		CacheWrite: i64(usage, "input_cache_creation"),
	}}
}

// ZCode parses ZCode model-io rollouts, which record the resolved model id and
// the provider usage block for every request.
type ZCode struct{ home string }

// NewZCode returns the ZCode adapter.
func NewZCode(home string) *ZCode { return &ZCode{home: home} }

// Name implements Adapter.
func (z *ZCode) Name() string { return "zcode" }

// Agent implements Adapter.
func (z *ZCode) Agent() string { return "zcode" }

// Globs implements Adapter.
func (z *ZCode) Globs() []string {
	return []string{filepath.ToSlash(filepath.Join(z.home, ".zcode", "cli", "rollout", "model-io-*.jsonl"))}
}

// Needles implements Prefilterer.
func (z *ZCode) Needles() []string { return []string{`"cacheReadTokens"`, `"inputTokens"`} }

// ParseLine implements Adapter.
func (z *ZCode) ParseLine(line []byte, ctx *Context) []Record {
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	resp := obj(m, "response")
	if resp == nil {
		return nil
	}
	usage := obj(resp, "usage")
	if usage == nil {
		return nil
	}
	model := ""
	if mm := obj(m, "model"); mm != nil {
		model = str(mm, "modelId")
	}
	if model == "" {
		model = str(resp, "modelId")
	}
	t, _ := timeFrom(m, "completedAt")
	return []Record{{
		Time:       t,
		Model:      model,
		Input:      i64(usage, "inputTokens"),
		Output:     i64(usage, "outputTokens"),
		CacheRead:  i64(usage, "cacheReadTokens"),
		CacheWrite: i64(usage, "cacheWriteTokens"),
	}}
}

// Pi parses the pi harness session files.
type Pi struct{ home string }

// NewPi returns the pi adapter.
func NewPi(home string) *Pi { return &Pi{home: home} }

// Name implements Adapter.
func (p *Pi) Name() string { return "pi" }

// Agent implements Adapter.
func (p *Pi) Agent() string { return "pi" }

// Globs implements Adapter.
func (p *Pi) Globs() []string {
	return []string{filepath.ToSlash(filepath.Join(p.home, ".pi", "agent", "sessions", "**", "*.jsonl"))}
}

// Needles implements Prefilterer.
func (p *Pi) Needles() []string { return []string{`"cacheRead"`} }

// ParseLine implements Adapter.
func (p *Pi) ParseLine(line []byte, ctx *Context) []Record {
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	msg := obj(m, "message")
	if msg == nil {
		return nil
	}
	usage := obj(msg, "usage")
	if usage == nil {
		return nil
	}
	if str(m, "type") != "message" || str(msg, "role") != "assistant" {
		return nil
	}
	t, _ := timeFrom(m, "timestamp")
	return []Record{{
		Time:       t,
		Model:      str(msg, "model"),
		Input:      i64(usage, "input"),
		Output:     i64(usage, "output"),
		CacheRead:  i64(usage, "cacheRead"),
		CacheWrite: i64(usage, "cacheWrite"),
	}}
}

// DSH parses session archives of the dsh harness. The logs are zstd-compressed,
// so ingest decodes them whole on each change instead of seeking.
type DSH struct{ home string }

// NewDSH returns the dsh adapter.
func NewDSH(home string) *DSH { return &DSH{home: home} }

// Name implements Adapter.
func (d *DSH) Name() string { return "dsh" }

// Agent implements Adapter.
func (d *DSH) Agent() string { return "dsh" }

// Globs implements Adapter.
func (d *DSH) Globs() []string {
	return []string{filepath.ToSlash(filepath.Join(d.home, ".dsh", "sessions", "**", "session.v4.jsonl.zstd"))}
}

// Needles implements Prefilterer.
func (d *DSH) Needles() []string { return []string{`"cacheReadTokens"`} }

// ParseLine implements Adapter.
func (d *DSH) ParseLine(line []byte, ctx *Context) []Record {
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	usage := obj(m, "usage")
	data := obj(m, "data")
	if usage == nil && data != nil {
		usage = obj(data, "usage")
	}
	if usage == nil {
		return nil
	}
	model := ""
	if data != nil {
		if msg := obj(data, "message"); msg != nil {
			if src := obj(msg, "source"); src != nil {
				model = str(src, "model")
			}
		}
		if model == "" {
			if src := obj(data, "source"); src != nil {
				model = str(src, "model")
			}
		}
		if model == "" {
			model = str(data, "model")
		}
	}
	if model != "" {
		ctx.Model = model
	}
	var t time.Time
	if ms := i64(m, "time"); ms > 0 {
		t = time.UnixMilli(ms)
	}
	return []Record{{
		Time:       t,
		Model:      ctx.Model,
		Input:      i64(usage, "inputTokens"),
		Output:     i64(usage, "outputTokens"),
		CacheRead:  i64(usage, "cacheReadTokens"),
		CacheWrite: i64(usage, "cacheWriteTokens"),
	}}
}

// --- tiny string/number helpers kept local to avoid extra imports in callers ---

func indexOf(s, sub string) int {
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func f64(m map[string]json.RawMessage, key string) float64 {
	raw, ok := m[key]
	if !ok || len(raw) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		return 0
	}
	return v
}
