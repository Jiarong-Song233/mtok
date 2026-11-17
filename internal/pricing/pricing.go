// Package pricing turns token counts into a reference cost. Prices are USD per
// million tokens per direction, exactly like the pricing.json the PowerShell
// mtok already uses, with optional cache read/write rates.
package pricing

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/songjiarong/mtok/internal/model"
)

//go:embed builtin.json
var builtinJSON []byte

// Price is USD per million tokens.
type Price struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}

// USD returns the reference cost of the given token counts.
func (p Price) USD(input, output, cacheRead, cacheWrite int64) float64 {
	return float64(input)/1e6*p.Input +
		float64(output)/1e6*p.Output +
		float64(cacheRead)/1e6*p.CacheRead +
		float64(cacheWrite)/1e6*p.CacheWrite
}

type file struct {
	Currency struct {
		Rate   float64 `json:"rate"`
		Symbol string  `json:"symbol"`
	} `json:"currency"`
	Default *Price           `json:"default"`
	Models  map[string]Price `json:"models"`
}

// Table is the resolved price list.
type Table struct {
	Rate     float64
	Default  *Price
	byKey    map[string]Price
	source   string
	Currency string
}

// Load builds the table: built-in prices first, then the user file overrides.
// A missing or broken user file is reported but never fatal.
func Load(path string, rateOverride float64) (*Table, error) {
	t := &Table{byKey: map[string]Price{}, Rate: 7.2, Currency: "USD"}
	if err := t.merge(builtinJSON); err != nil {
		return t, fmt.Errorf("builtin pricing: %w", err)
	}
	t.source = "builtin"
	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := t.merge(data); err != nil {
				return t, fmt.Errorf("%s: %w", path, err)
			}
			t.source = path
		case !os.IsNotExist(err):
			return t, err
		}
	}
	if rateOverride > 0 {
		t.Rate = rateOverride
	}
	return t, nil
}

func (t *Table) merge(data []byte) error {
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	if f.Currency.Rate > 0 {
		t.Rate = f.Currency.Rate
	}
	if f.Currency.Symbol != "" {
		t.Currency = f.Currency.Symbol
	}
	if f.Default != nil {
		d := *f.Default
		t.Default = &d
	}
	for name, p := range f.Models {
		t.byKey[model.Key(name)] = p
	}
	return nil
}

// Source names where the table came from, for `mtok config`.
func (t *Table) Source() string { return t.source }

// Lookup finds the price for a canonical model name, falling back to the
// default entry. The second result is false when the model is unpriced.
func (t *Table) Lookup(name string) (Price, bool) {
	if p, ok := t.byKey[model.Key(name)]; ok {
		return p, true
	}
	if t.Default != nil {
		return *t.Default, true
	}
	return Price{}, false
}
