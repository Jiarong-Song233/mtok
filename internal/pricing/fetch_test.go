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
