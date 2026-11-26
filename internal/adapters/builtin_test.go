package adapters

import (
	"testing"
	"time"
)

func one(t *testing.T, a Adapter, line string, ctx *Context) []Record {
	t.Helper()
	return a.ParseLine([]byte(line), ctx)
}

func TestClaudeAssistantUsage(t *testing.T) {
	a := NewClaude(t.TempDir())
	ctx := &Context{}
	line := `{"type":"assistant","timestamp":"2026-11-15T05:48:07.815Z","requestId":"req_1","message":{"id":"msg_1","model":"claude-opus-5-5","usage":{"input_tokens":12,"output_tokens":34,"cache_read_input_tokens":560,"cache_creation_input_tokens":7}}}`
	recs := one(t, a, line, ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Model != "claude-opus-5-5" || r.Input != 12 || r.Output != 34 || r.CacheRead != 560 || r.CacheWrite != 7 {
		t.Fatalf("unexpected record %+v", r)
	}
	if r.Time.IsZero() {
		t.Fatal("timestamp not parsed")
	}
	// A repeated message id must not be counted twice.
	if got := one(t, a, line, ctx); len(got) != 0 {
		t.Fatalf("duplicate counted: %+v", got)
	}
}

func TestClaudeIgnoresNonAssistantAndSynthetic(t *testing.T) {
	a := NewClaude(t.TempDir())
	ctx := &Context{}
	user := `{"type":"user","timestamp":"2026-11-15T05:48:07.815Z","message":{"role":"user","content":"hi"}}`
	if got := one(t, a, user, ctx); len(got) != 0 {
		t.Fatalf("user line counted: %+v", got)
	}
	syn := `{"type":"assistant","timestamp":"2026-11-15T05:48:07.815Z","message":{"id":"m2","model":"<synthetic>","usage":{"input_tokens":1,"output_tokens":2}}}`
	if got := one(t, a, syn, ctx); len(got) != 0 {
		t.Fatalf("synthetic line counted: %+v", got)
	}
}

func TestCodexModelCarryOverAndCacheSplit(t *testing.T) {
	a := NewCodex(t.TempDir())
	ctx := &Context{}
	turn := `{"timestamp":"2026-11-15T08:38:09.251Z","type":"turn_context","payload":{"turn_id":"t1","model":"gpt-6.1-sol"}}`
	if got := one(t, a, turn, ctx); len(got) != 0 {
		t.Fatalf("turn_context should not emit: %+v", got)
	}
	if ctx.Model != "gpt-6.1-sol" {
		t.Fatalf("model not carried, got %q", ctx.Model)
	}
	usage := `{"timestamp":"2026-11-15T08:38:17.094Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":28403,"cached_input_tokens":14464,"cache_write_input_tokens":3,"output_tokens":157}}}}`
	recs := one(t, a, usage, ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Input != 28403-14464 {
		t.Errorf("input should exclude cached: %d", r.Input)
	}
	if r.CacheRead != 14464 || r.Output != 157 || r.CacheWrite != 3 {
		t.Errorf("unexpected record %+v", r)
	}
	if r.Model != "gpt-6.1-sol" {
		t.Errorf("model not attached: %q", r.Model)
	}
}

func TestGrokInference(t *testing.T) {
	dir := t.TempDir()
	a := NewGrok(dir)
	ctx := &Context{}
	// No config.toml in the temp home: the model stays empty until changed.
	one(t, a, `{"ts":"2026-11-15T07:50:44.052Z","msg":"model changed","ctx":{"model":"magpie/auto/auto"}}`, ctx)
	if ctx.Model != "magpie/auto/auto" {
		t.Fatalf("model change missed: %q", ctx.Model)
	}
	recs := one(t, a, `{"ts":"2026-11-15T05:48:07.815Z","msg":"shell.turn.inference_done","ctx":{"prompt_tokens":200459,"cached_prompt_tokens":200192,"completion_tokens":980}}`, ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Input != 200459-200192 || r.CacheRead != 200192 || r.Output != 980 {
		t.Fatalf("unexpected record %+v", r)
	}
}

func TestGrokDefaultModelFromConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/.grok/config.toml", `[models]
default = "magpie/group/auto-auto"
default_reasoning_effort = "high"
`)
	a := NewGrok(dir)
	ctx := &Context{}
	recs := one(t, a, `{"ts":"2026-11-15T05:48:07.815Z","msg":"shell.turn.inference_done","ctx":{"prompt_tokens":10,"cached_prompt_tokens":4,"completion_tokens":2}}`, ctx)
	if len(recs) != 1 || recs[0].Model != "magpie/group/auto-auto" {
		t.Fatalf("default model not applied: %+v", recs)
	}
}

func TestGeminiModelSelectionAndUsage(t *testing.T) {
	a := NewGemini(t.TempDir())
	ctx := &Context{}
	user := `{"step_index":0,"type":"USER_INPUT","created_at":"2026-11-15T02:29:21Z","content":"<USER_SETTINGS_CHANGE>\nThe user changed setting ` + "`Model Selection`" + ` from None to Gemini 3.8 Flash (High). No need to comment.\n</USER_SETTINGS_CHANGE>"}`
	one(t, a, user, ctx)
	if ctx.Model != "Gemini 3.8 Flash (High)" {
		t.Fatalf("model selection not parsed: %q", ctx.Model)
	}
	plan := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-11-15T02:29:22Z","input_tokens":13927,"cache_read_tokens":0,"output_tokens":314}`
	recs := one(t, a, plan, ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Input != 13927 || r.Output != 314 || r.CacheRead != 0 {
		t.Fatalf("unexpected record %+v", r)
	}
}

func TestGeminiCLIModelFieldAndThinkingStrip(t *testing.T) {
	a := NewGemini(t.TempDir())
	ctx := &Context{}
	// antigravity-cli records the model on the response line and appends the
	// thinking level, which must be stripped so sessions merge by model.
	plan := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-11-25T09:04:16Z","model":"gemini-4-hydrogen-high","input_tokens":18248,"cache_read_tokens":0,"output_tokens":1061}`
	recs := one(t, a, plan, ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	if recs[0].Model != "gemini-4-hydrogen" {
		t.Fatalf("thinking level not stripped: %q", recs[0].Model)
	}
	if recs[0].Input != 18248 || recs[0].Output != 1061 {
		t.Fatalf("unexpected record %+v", recs[0])
	}
	// A real model variant ending in "-thinking" must be left intact.
	claude := `{"step_index":2,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":"2026-11-25T09:04:17Z","model":"claude-opus-4-6-thinking","input_tokens":10,"output_tokens":2}`
	recs = one(t, a, claude, ctx)
	if len(recs) != 1 || recs[0].Model != "claude-opus-4-6-thinking" {
		t.Fatalf("claude thinking variant mangled: %+v", recs)
	}
}

func TestKimiWireStatusUpdate(t *testing.T) {
	a := NewKimi(t.TempDir())
	ctx := &Context{}
	line := `{"timestamp": 1776507198.6702769, "message": {"type": "StatusUpdate", "payload": {"context_tokens": 15893, "token_usage": {"input_other": 6677, "output": 89, "input_cache_read": 9216, "input_cache_creation": 5}}}}`
	recs := one(t, a, line, ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Input != 6677 || r.Output != 89 || r.CacheRead != 9216 || r.CacheWrite != 5 {
		t.Fatalf("unexpected record %+v", r)
	}
	if r.Time.Unix() != 1776507198 {
		t.Fatalf("unix seconds not parsed: %v", r.Time)
	}
}

func TestZCodeModelIO(t *testing.T) {
	a := NewZCode(t.TempDir())
	ctx := &Context{}
	line := `{"completedAt":"2026-11-02T15:17:24.660Z","model":{"modelId":"GLM-5.3-Flash","providerId":"account:bigmodel-start-plan"},"response":{"usage":{"inputTokens":28241,"outputTokens":141,"cacheReadTokens":0,"cacheWriteTokens":2,"totalTokens":28384}}}`
	recs := one(t, a, line, ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Model != "GLM-5.3-Flash" || r.Input != 28241 || r.Output != 141 || r.CacheWrite != 2 {
		t.Fatalf("unexpected record %+v", r)
	}
	if r.Time.IsZero() {
		t.Fatal("completedAt not parsed")
	}
}

func TestPiMessage(t *testing.T) {
	a := NewPi(t.TempDir())
	ctx := &Context{}
	line := `{"type":"message","id":"m1","timestamp":"2026-11-08T10:08:32.324Z","message":{"role":"assistant","provider":"magpie","model":"auto/auto","usage":{"input":120,"output":30,"cacheRead":400,"cacheWrite":8}}}`
	recs := one(t, a, line, ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Input != 120 || r.Output != 30 || r.CacheRead != 400 || r.CacheWrite != 8 || r.Model != "auto/auto" {
		t.Fatalf("unexpected record %+v", r)
	}
}

func TestDSHMessageSource(t *testing.T) {
	a := NewDSH(t.TempDir())
	ctx := &Context{}
	line := `{"type":"assistant/message","seq":16,"time":1793004895952,"data":{"turn":1,"step":1,"message":{"role":"assistant","source":{"kind":"model","provider":"deepseek-account","model":"deepseek-flash"}},"usage":{"inputTokens":4441,"outputTokens":69,"cacheReadTokens":2944,"cacheWriteTokens":1,"totalTokens":7454}}}`
	recs := one(t, a, line, ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Model != "deepseek-flash" || r.Input != 4441 || r.Output != 69 || r.CacheRead != 2944 || r.CacheWrite != 1 {
		t.Fatalf("unexpected record %+v", r)
	}
	if r.Time.UnixMilli() != 1793004895952 {
		t.Fatalf("millisecond time not parsed: %v", r.Time)
	}
	if r.Time.Before(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("implausible time %v", r.Time)
	}
}

func TestNeedlesMatchRealLines(t *testing.T) {
	cases := []struct {
		a     Adapter
		line  string
		match bool
	}{
		{NewGrok("."), `{"msg":"shell.turn.inference_done","ctx":{}}`, true},
		{NewClaude("."), `{"type":"assistant","message":{"usage":{}}}`, true},
		{NewCodex("."), `{"type":"event_msg","payload":{"type":"token_count"}}`, true},
	}
	for _, c := range cases {
		if got := MatchesNeedles([]byte(c.line), Needles(c.a)); got != c.match {
			t.Errorf("%s prefilter = %v, want %v", c.a.Name(), got, c.match)
		}
		if !MatchesNeedles([]byte(`{"nothing":1}`), Needles(c.a)) && len(Needles(c.a)) > 0 {
			continue
		}
	}
	if MatchesNeedles([]byte(`{"unrelated":true}`), Needles(NewGrok("."))) {
		t.Error("unrelated line should be filtered out")
	}
}
