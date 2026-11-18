package pricing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const LiteLLMURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// litellmEntry mirrors the subset of LiteLLM's pricing JSON we need.
type litellmEntry struct {
	InputPerToken      *float64 `json:"input_cost_per_token"`
	OutputPerToken     *float64 `json:"output_cost_per_token"`
	CacheReadPerToken  *float64 `json:"cache_read_input_token_cost"`
	CacheWritePerToken *float64 `json:"cache_write_input_token_cost"`
}

// FetchedPrice is a LiteLLM price plus flags for optional cache fields.
type FetchedPrice struct {
	Price
	HasCacheRead  bool
	HasCacheWrite bool
}

// FetchLiteLLM downloads LiteLLM's community pricing table and returns a map
// keyed by model.Key-compatible names (provider prefix stripped, lowercase).
func FetchLiteLLM(url string, timeout time.Duration) (map[string]FetchedPrice, error) {
	if url == "" {
		url = LiteLLMURL
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var raw map[string]litellmEntry
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	const M = 1_000_000
	out := make(map[string]FetchedPrice, len(raw))
	priority := make(map[string]int, len(raw))
	for key, e := range raw {
		name := stripProvider(key)
		if name == "" || e.InputPerToken == nil || e.OutputPerToken == nil {
			continue
		}
		fp := FetchedPrice{Price: Price{
			Input:  *e.InputPerToken * M,
			Output: *e.OutputPerToken * M,
		}}
		if e.CacheReadPerToken != nil {
			fp.CacheRead = *e.CacheReadPerToken * M
			fp.HasCacheRead = true
		}
		if e.CacheWritePerToken != nil {
			fp.CacheWrite = *e.CacheWritePerToken * M
			fp.HasCacheWrite = true
		}
		fk := foldKey(name)
		// Bare model entries outrank provider-prefixed duplicates, whose prices
		// can be marked up by the host.
		p := 0
		if !strings.Contains(key, "/") {
			p = 1
		}
		if prior, exists := priority[fk]; exists && prior >= p {
			continue
		}
		out[fk] = fp
		priority[fk] = p
	}
	return out, nil
}

// ApplyFetched updates prices where a fetched entry matches an existing key.
// Missing cache fields leave the current cache price untouched.
// Returns the number of prices updated.
func (t *Table) ApplyFetched(fetched map[string]FetchedPrice) int {
	n := 0
	for key, existing := range t.byKey {
		fp, ok := fetched[key]
		if !ok {
			continue
		}
		existing.Input = fp.Input
		existing.Output = fp.Output
		if fp.HasCacheRead {
			existing.CacheRead = fp.CacheRead
		}
		if fp.HasCacheWrite {
			existing.CacheWrite = fp.CacheWrite
		}
		t.byKey[key] = existing
		n++
	}
	return n
}

// Export returns a copy of the table's resolved prices.
func (t *Table) Export() map[string]Price {
	out := make(map[string]Price, len(t.byKey))
	for k, v := range t.byKey {
		out[k] = v
	}
	return out
}

// stripProvider removes provider prefixes before the final model segment.
// "together_ai/Qwen/Qwen4-Max" becomes "Qwen4-Max".
func stripProvider(key string) string {
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[i+1:]
	}
	return key
}

// foldKey lowercases and keeps alphanumeric characters, matching model.Key.
func foldKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + 32)
		}
	}
	return b.String()
}
