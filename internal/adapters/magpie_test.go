package adapters

import "testing"

func TestMagpieJournalLine(t *testing.T) {
	m := NewMagpie("/home/u", "/home/u/.config/magpie/usage.jsonl")
	ctx := &Context{}
	line := `{"route_id":1793547638166,"t":"2026-11-01T09:40:38.1594232-06:00","agent":"Grok","provider":"workbuddy","model":"deepseek-v4.1-flash","req":"deepseek-flash","served":"deepseek-v4.1-flash","in":46795,"out":166,"cache_read":0,"cache_write":7,"reasoning":9,"status":200}`
	recs := m.ParseLine([]byte(line), ctx)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	r := recs[0]
	if r.Model != "deepseek-v4.1-flash" {
		t.Errorf("model = %q, want the served model", r.Model)
	}
	if r.Agent != "grok" {
		t.Errorf("agent = %q, want lowercase journal client", r.Agent)
	}
	if r.Input != 46795 || r.Output != 166 || r.CacheWrite != 7 {
		t.Errorf("unexpected tokens %+v", r)
	}
	if r.Time.IsZero() || r.Time.Day() != 1 {
		t.Errorf("timestamp not parsed: %v", r.Time)
	}
	if !m.JournalSource() {
		t.Error("journal adapter must mark itself as a journal source")
	}
}

func TestMagpiePrefersServedOverRouteCode(t *testing.T) {
	m := NewMagpie(t.TempDir(), "")
	// magpie keeps the requested route in model/req and the real model in served.
	line := `{"t":"2026-11-02T10:00:00-06:00","agent":"claude","model":"qfmodel","req":"qfmodel","served":"Qwen3.8-Flash","in":10,"out":1}`
	recs := m.ParseLine([]byte(line), &Context{})
	if len(recs) != 1 || recs[0].Model != "Qwen3.8-Flash" {
		t.Fatalf("served model not used: %+v", recs)
	}
	// Without served, fall back to model, then req.
	recs = m.ParseLine([]byte(`{"t":"2026-11-02T10:00:00-06:00","agent":"x","model":"glm-5.3","in":5,"out":1}`), &Context{})
	if len(recs) != 1 || recs[0].Model != "glm-5.3" {
		t.Fatalf("model fallback failed: %+v", recs)
	}
	recs = m.ParseLine([]byte(`{"t":"2026-11-02T10:00:00-06:00","agent":"x","req":"auto/auto","in":5,"out":1}`), &Context{})
	if len(recs) != 1 || recs[0].Model != "auto/auto" {
		t.Fatalf("req fallback failed: %+v", recs)
	}
}

func TestMagpieDefaultPath(t *testing.T) {
	m := NewMagpie("/home/u", "")
	if got := m.Globs()[0]; got == "" || !contains(got, "magpie/usage.jsonl") {
		t.Fatalf("unexpected default journal path %q", got)
	}
}

func TestIsRouteModel(t *testing.T) {
	routes := []string{
		"auto/auto", "auto", "group/auto-auto", "group/auto-deepseek-v4-1-flash",
		"magpie/auto/auto", "magpie/group/auto-auto", "auto/glm-5.3", "auto-glm-5.3",
		"[cc]auto/auto", "AUTO/AUTO",
	}
	for _, r := range routes {
		if !IsRouteModel(r) {
			t.Errorf("IsRouteModel(%q) = false, want true", r)
		}
	}
	direct := []string{
		"", "claude-opus-5-5", "gpt-6.1-sol", "deepseek-flash",
		"workbuddy/deepseek-v4.1-flash", "glm-5.3", "codex/gpt-6.1-sol",
		"sn-glm-5-3",
	}
	for _, r := range direct {
		if IsRouteModel(r) {
			t.Errorf("IsRouteModel(%q) = true, want false", r)
		}
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && indexOf(s, sub) >= 0 }
