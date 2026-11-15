// Package magpie talks to the optional local magpie proxy. It is never
// required: mtok works from local logs alone, and a missing proxy only means
// the quota block is omitted.
package magpie

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// DefaultURL is where magpie listens by default.
const DefaultURL = "http://127.0.0.1:3425"

// Quota is one provider's remaining allowance.
type Quota struct {
	Provider  string  `json:"provider"`
	Name      string  `json:"name"`
	Remaining float64 `json:"remaining"`
	Detail    string  `json:"detail,omitempty"`
	accounts  int
}

// Accounts reports how many accounts were merged into this quota.
func (q Quota) Accounts() int { return q.accounts }

// Usage is the proxy's own accounting, used by `mtok doctor` to cross-check
// what the log adapters counted.
type Usage struct {
	Period     string  `json:"period"`
	Since      string  `json:"since"`
	Calls      int64   `json:"calls"`
	Input      int64   `json:"input"`
	Output     int64   `json:"output"`
	CacheRead  int64   `json:"cache_read"`
	CacheWrite int64   `json:"cache_write"`
	Cost       float64 `json:"cost"`
	Unpriced   int64   `json:"unpriced"`
}

// Client is a short-timeout reader.
type Client struct {
	BaseURL string
	Timeout time.Duration
	HTTP    *http.Client
}

// New returns a client for baseURL.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Timeout: 900 * time.Millisecond,
		HTTP:    &http.Client{Timeout: 900 * time.Millisecond},
	}
}

func (c *Client) get(path string, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type window struct {
	Name      string   `json:"name"`
	Used      *float64 `json:"used"`
	Remaining *float64 `json:"remaining"`
	// Text is sometimes a string and sometimes a bool in magpie's payload, so
	// it is read as an opaque value and ignored.
	Text any `json:"text"`
}

type entry struct {
	Provider string   `json:"provider"`
	Name     string   `json:"name"`
	Error    string   `json:"error"`
	Windows  []window `json:"windows"`
}

type quotasResp struct {
	Data []entry `json:"data"`
}

// Quotas returns remaining percentages per provider, best-effort. Providers
// without a numeric window are dropped.
func (c *Client) Quotas() ([]Quota, error) {
	var resp quotasResp
	// Two attempts like the PowerShell version: magpie answers partially while
	// its providers are still refreshing.
	var err error
	best := -1
	for i := 0; i < 2; i++ {
		var cur quotasResp
		if e := c.get("/v1/magpie/quotas", &cur); e != nil {
			err = e
			time.Sleep(120 * time.Millisecond)
			continue
		}
		n := 0
		for _, d := range cur.Data {
			if d.Error == "" && len(d.Windows) > 0 {
				n++
			}
		}
		if n > best {
			best, resp = n, cur
		}
		if n >= 5 {
			break
		}
		time.Sleep(120 * time.Millisecond)
	}
	if best <= 0 {
		return nil, err
	}
	var out []Quota
	for _, d := range resp.Data {
		if d.Error != "" || len(d.Windows) == 0 {
			continue
		}
		q, ok := summarize(d)
		if ok {
			out = append(out, q)
		}
	}
	// Several accounts can share one provider; show the tightest allowance and
	// how many accounts contribute to it.
	groups := map[string][]Quota{}
	var order []string
	for _, q := range out {
		if _, ok := groups[q.Provider]; !ok {
			order = append(order, q.Provider)
		}
		groups[q.Provider] = append(groups[q.Provider], q)
	}
	merged := make([]Quota, 0, len(order))
	for _, p := range order {
		list := groups[p]
		best := list[0]
		for _, q := range list[1:] {
			if q.Remaining < best.Remaining {
				best = q
			}
		}
		best.accounts = len(list)
		merged = append(merged, best)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Provider < merged[j].Provider })
	return merged, nil
}

func summarize(d entry) (Quota, bool) {
	if d.Provider == "codex" {
		var segs []string
		min := 100.0
		for _, w := range d.Windows {
			n := strings.ToLower(w.Name)
			if w.Remaining == nil || !(strings.Contains(n, "5 hour") || strings.Contains(n, "7 day")) {
				continue
			}
			label := "5h"
			if strings.Contains(n, "7 day") {
				label = "7d"
			}
			segs = append(segs, fmt.Sprintf("%s%.0f%%", label, *w.Remaining))
			if *w.Remaining < min {
				min = *w.Remaining
			}
		}
		if len(segs) == 0 {
			return Quota{}, false
		}
		return Quota{Provider: "codex", Name: d.Name, Remaining: min, Detail: strings.Join(segs, " ")}, true
	}
	var total, min *float64
	for _, w := range d.Windows {
		if w.Remaining == nil {
			continue
		}
		n := strings.ToLower(w.Name)
		if strings.Contains(n, "image generation") {
			continue
		}
		if strings.Contains(n, "total") {
			if total == nil || *w.Remaining < *total {
				total = w.Remaining
			}
		}
		if min == nil || *w.Remaining < *min {
			min = w.Remaining
		}
	}
	pick := min
	if total != nil {
		pick = total
	}
	if pick == nil {
		return Quota{}, false
	}
	return Quota{Provider: d.Provider, Name: d.Name, Remaining: *pick}, true
}

// Usage reads the proxy's own counters for a period (today, 7d, ...).
func (c *Client) Usage(period string) (*Usage, error) {
	if period == "" {
		period = "today"
	}
	var u Usage
	if err := c.get("/v1/magpie/usage?period="+period, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
