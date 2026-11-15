package model

import "testing"

func TestKeyFoldsSeparatorsAndCase(t *testing.T) {
	same := []string{
		"GLM-5.3", "glm-5-3", "GLM_5_3", "glm 5.3", "glm53", "GLM~5~3",
	}
	want := Key("glm53")
	for _, s := range same {
		if got := Key(s); got != want {
			t.Errorf("Key(%q) = %q, want %q", s, got, want)
		}
	}
}

func TestCleanStripsDecorations(t *testing.T) {
	cases := map[string]string{
		"[cc]glm-5.3@5k":          "glm-5.3",
		"provider/GLM-5.3":        "GLM-5.3",
		"Gemini 3.8 Flash (High)": "Gemini 3.8 Flash",
		"claude-opus-5.5@200k":    "claude-opus-5.5",
	}
	for raw, want := range cases {
		if got := clean(raw); got != want {
			t.Errorf("clean(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestResolveCanonicalTable(t *testing.T) {
	n := New()
	cases := map[string]string{
		"claude-opus-5-5":               "Claude-Opus-5.5",
		"anthropic/claude-opus-5.5":     "Claude-Opus-5.5",
		"claude_opus_5_5":               "Claude-Opus-5.5",
		"gmodel":                        "GLM-5.3",
		"gfmodel":                       "GLM-5.3-Flash",
		"dmodel":                        "DeepSeek-V4-Pro",
		"dfmodel":                       "DeepSeek-Flash",
		"qfmodel":                       "Qwen3.8-Flash",
		"qmodel_38max":                  "Qwen3.8-Max",
		"kmodel_latest":                 "Kimi-K3.1-Pro",
		"kmodel":                        "Kimi-K2.8-Preview",
		"mmodel":                        "MiniMax-M3.1-Flash",
		"jmodel":                        "Step-5-Preview",
		"sn-glm-5-3":                    "GLM-5.3",
		"sn-deepseek-v4-1-flash-1017":   "DeepSeek-V4.1-Flash",
		"group/auto-auto":               "Auto(未指明)",
		"magpie/auto/auto":              "Auto(未指明)",
		"gpt-6.1-sol":                   "GPT-6.1-Sol",
		"Gemini 3.8 Flash (High)":       "Gemini-3.8-Flash",
		"gemini-3-8-flash-tiered":       "Gemini-3.8-Flash",
		"workbuddy/deepseek-v4.1-flash": "DeepSeek-V4.1-Flash",
	}
	for raw, want := range cases {
		if got := n.Resolve(raw); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestResolveUnknownPrettifies(t *testing.T) {
	n := New()
	cases := map[string]string{
		"acme-v41-turbo": "Acme-v4.1-Turbo",
		"new_model_2_0":  "New-Model-2.0",
		"":               "Unknown",
		"   ":            "Unknown",
		"some/unknown-x": "Unknown-X",
	}
	for raw, want := range cases {
		if got := n.Resolve(raw); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestUserAliasOverridesBuiltin(t *testing.T) {
	n := New()
	n.Add("My-GLM", "gmodel")
	if got := n.Resolve("gmodel"); got != "My-GLM" {
		t.Fatalf("override failed, got %q", got)
	}
}

func TestAliasTableHasNoCollisions(t *testing.T) {
	seen := map[string]string{}
	for canonical, aliases := range builtinAliases {
		for _, a := range append([]string{canonical}, aliases...) {
			k := Key(a)
			if prev, ok := seen[k]; ok && prev != canonical {
				t.Errorf("alias %q claimed by both %q and %q", a, prev, canonical)
			}
			seen[k] = canonical
		}
	}
}
