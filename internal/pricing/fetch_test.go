package pricing

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchLiteLLMAndApply(t *testing.T) {
	const payload = `{
		"gpt-6.1-sol": {
			"input_cost_per_token": 0.00000125,
			"output_cost_per_token": 0.00001,
			"cache_read_input_token_cost": 0.000000125,
			"cache_write_input_token_cost": 0.00000125
		},
		"azure/gpt-6.1-sol-dup": {
			"input_cost_per_token": 9,
			"output_cost_per_token": 9
		},
		"claude-opus-5-5": {
			"input_cost_per_token": 0.000004,
			"output_cost_per_token": 0.00002,
			"cache_read_input_token_cost": 0.0000002,
			"cache_creation_input_token_cost": 0.000005
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()
	got, err := FetchLiteLLM(srv.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := got["gpt61sol"]
	if !ok {
		t.Fatalf("expected gpt61sol, keys=%v", got)
	}
	if p.Input != 1.25 || p.Output != 10 || p.CacheRead != 0.125 || p.CacheWrite != 1.25 {
		t.Fatalf("bad conversion: %+v", p)
	}
	claude, ok := got["claudeopus55"]
	if !ok || claude.CacheWrite != 5 {
		t.Fatalf("cache_creation should map to cache_write: %+v ok=%v", claude, ok)
	}
	tab, _ := Load("", 0)
	n := tab.ApplyFetched(got)
	if n == 0 {
		t.Fatal("no model matched")
	}
	if price, ok := tab.Lookup("gpt-6.1-sol"); !ok || price.Input != 1.25 {
		t.Fatalf("ApplyFetched did not update table: %+v ok=%v", price, ok)
	}
}

func TestFetchLiteLLMHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	if _, err := FetchLiteLLM(srv.URL, 5*time.Second); err == nil {
		t.Fatal("expected HTTP error")
	}
}
