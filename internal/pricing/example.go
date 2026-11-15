package pricing

import (
	"bytes"
	"encoding/json"
)

// ExampleJSON returns the built-in table pretty-printed, so `mtok config --init`
// can drop a fully editable pricing.json next to the config.
func ExampleJSON() []byte {
	var buf bytes.Buffer
	if err := json.Indent(&buf, builtinJSON, "", "  "); err != nil {
		return builtinJSON
	}
	buf.WriteByte('\n')
	return buf.Bytes()
}
