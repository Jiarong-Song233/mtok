// Package config resolves file locations and loads user overrides.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// BudgetConfig holds optional spend limits in the configured currency.
type BudgetConfig struct {
	Daily   *float64 `yaml:"daily"`
	Monthly *float64 `yaml:"monthly"`
}

// Config mirrors config.yaml. Pointers keep "unset" distinguishable from zero
// so file values only override defaults that were actually written.
type Config struct {
	Width       int             `yaml:"width"`
	Top         int             `yaml:"top"`
	Currency    string          `yaml:"currency"`
	Rate        float64         `yaml:"rate"`
	Magpie      *bool           `yaml:"magpie"`
	MagpieURL   string          `yaml:"magpie_url"`
	MagpieUsage string          `yaml:"magpie_usage_file"`
	Zstd        *bool           `yaml:"zstd"`
	Agents      map[string]bool `yaml:"agents"`
	PricingFile string          `yaml:"pricing_file"`
	DataDir     string          `yaml:"data_dir"`
	Budget      BudgetConfig    `yaml:"budget"`
	// Agents lists the built-in adapters that should stay enabled; a false
	// value disables the adapter for this machine.
}

// Settings is the resolved configuration used by the CLI.
type Settings struct {
	Width         int
	Top           int
	Currency      string
	Rate          float64
	Magpie        bool
	MagpieURL     string
	MagpieUsage   string // proxy call journal (~/.config/magpie/usage.jsonl)
	Zstd          bool
	BudgetDaily   float64 // zero means unset
	BudgetMonthly float64 // zero means unset
	Disabled      map[string]bool
	Pricing       string // absolute path of the pricing file to load
}

// Paths holds every location mtok touches.
type Paths struct {
	Home       string
	ConfigDir  string
	DataDir    string
	DB         string
	QuotaCache string
	Config     string
	Aliases    string
	Pricing    string
	Adaptersd  string
}

// Resolve computes the paths, honouring MTOK_CONFIG_DIR / MTOK_DATA_DIR / MTOK_DB.
func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	cfgDir := os.Getenv("MTOK_CONFIG_DIR")
	if cfgDir == "" {
		cfgDir = filepath.Join(home, ".config", "mtok")
	}
	dataDir := os.Getenv("MTOK_DATA_DIR")
	if dataDir == "" {
		if runtime.GOOS == "windows" {
			if la := os.Getenv("LOCALAPPDATA"); la != "" {
				dataDir = filepath.Join(la, "mtok")
			}
		}
		if dataDir == "" {
			dataDir = filepath.Join(home, ".local", "share", "mtok")
		}
	}
	db := os.Getenv("MTOK_DB")
	if db == "" {
		db = filepath.Join(dataDir, "mtok.db")
	}
	return Paths{
		Home:       home,
		ConfigDir:  cfgDir,
		DataDir:    dataDir,
		DB:         db,
		QuotaCache: filepath.Join(dataDir, "quota-cache.json"),
		Config:     filepath.Join(cfgDir, "config.yaml"),
		Aliases:    filepath.Join(cfgDir, "aliases.yaml"),
		Pricing:    filepath.Join(cfgDir, "pricing.json"),
		Adaptersd:  filepath.Join(cfgDir, "adapters.d"),
	}, nil
}

// Defaults are what mtok uses with no config file at all.
func Defaults() Settings {
	return Settings{
		Width:     72,
		Top:       8,
		Currency:  "USD",
		Rate:      7.2, // USD -> CNY reference, used only when currency is CNY
		Magpie:    true,
		MagpieURL: "http://127.0.0.1:3425",
		Zstd:      true,
		Disabled:  map[string]bool{},
	}
}

// Load reads config.yaml if present and applies it over the defaults.
func Load(p Paths) (Settings, Config, error) {
	s := Defaults()
	var raw Config
	data, err := os.ReadFile(p.Config)
	if err != nil {
		if os.IsNotExist(err) {
			s.Pricing = p.Pricing
			return s, raw, nil
		}
		return s, raw, err
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return s, raw, fmt.Errorf("config.yaml: %w", err)
	}
	if raw.Width >= 40 && raw.Width <= 400 {
		s.Width = raw.Width
	}
	if raw.Top > 0 && raw.Top <= 100 {
		s.Top = raw.Top
	}
	if raw.Currency != "" {
		s.Currency = raw.Currency
	}
	if raw.Rate > 0 {
		s.Rate = raw.Rate
	}
	if raw.Budget.Daily != nil && *raw.Budget.Daily > 0 {
		s.BudgetDaily = *raw.Budget.Daily
	}
	if raw.Budget.Monthly != nil && *raw.Budget.Monthly > 0 {
		s.BudgetMonthly = *raw.Budget.Monthly
	}
	if raw.Magpie != nil {
		s.Magpie = *raw.Magpie
	}
	if raw.MagpieURL != "" {
		s.MagpieURL = raw.MagpieURL
	}
	if raw.MagpieUsage != "" {
		s.MagpieUsage = Expand(raw.MagpieUsage, p.Home)
	}
	if raw.Zstd != nil {
		s.Zstd = *raw.Zstd
	}
	for name, on := range raw.Agents {
		if !on {
			s.Disabled[name] = true
		}
	}
	s.Pricing = p.Pricing
	if raw.PricingFile != "" {
		s.Pricing = Expand(raw.PricingFile, p.Home)
	}
	return s, raw, nil
}

// Expand resolves ~ and environment variables in a configured path.
func Expand(p, home string) string {
	if p == "" {
		return p
	}
	if len(p) > 1 && p[0] == '~' {
		p = filepath.Join(home, p[1:])
	}
	return os.ExpandEnv(p)
}

// Aliases reads aliases.yaml: raw identifier -> canonical display name.
func Aliases(p Paths) (map[string]string, error) {
	out := map[string]string{}
	data, err := os.ReadFile(p.Aliases)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	var raw map[string]string
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("aliases.yaml: %w", err)
	}
	for k, v := range raw {
		if k == "" || v == "" || k[0] == '#' {
			continue
		}
		out[k] = v
	}
	return out, nil
}

// ExampleConfig is written by `mtok config --init`.
const ExampleConfig = `# mtok configuration (all keys optional)
width: 72            # max output width in terminal columns
top: 8               # models shown in the table
currency: USD        # USD renders $, CNY renders ¥
rate: 7.2            # USD -> CNY conversion, used only when currency is CNY
# budget:            # spend limits in the chosen currency; mtok --check exits 3 when exceeded
#   daily: 30
#   monthly: 600
magpie: true         # read the local magpie journal + quotas (false = local logs only)
magpie_url: http://127.0.0.1:3425
# magpie_usage_file: ~/.config/magpie/usage.jsonl   # override the call journal path
zstd: true           # parse .jsonl.zst archives (codex / dsh)
# pricing_file: ~/.config/mtok/pricing.json
# agents:            # disable a built-in adapter
#   cursor: false
`

// ExampleAliases is written by `mtok config --init`.
const ExampleAliases = `# Map any raw model identifier to the canonical name mtok should show.
# Keys are matched ignoring case and separators, so all spellings below are equal:
#   glm-5.3 / GLM_5_3 / glm53 / [cc]glm-5.3@1k
#
# "gmodel": "GLM-5.3"
# "my-internal-alias": "Claude-Opus-5.5"
`

// ExampleAdapter is dropped into adapters.d on --init and documents the schema.
const ExampleAdapter = `# Declarative adapter: no Go code, no rebuild.
# Files here are loaded from ~/.config/mtok/adapters.d/*.yaml
#
# name: myagent
# agent: myagent                    # label shown by --by-agent (defaults to name)
# glob: ~/.myagent/logs/**/*.jsonl
# format: jsonl                     # jsonl (default) or regex
# contains: "\"usage\""             # cheap prefilter: skip lines without this text
# record:
#   filter:                         # all conditions must match
#     - path: type
#       equals: assistant
#   time:
#     path: timestamp               # RFC3339 string, or use unix_ms / unix_s
#   model:
#     path: message.model
#   input:
#     path: usage.input_tokens      # dot path, a[0].b indexes arrays
#   output:
#     path: usage.output_tokens
#   cache_read:
#     path: usage.cache_read_input_tokens
#   cache_write:
#     path: usage.cache_creation_input_tokens
#   # input_includes_cache: true    # set when input already counts cached tokens
#
# Regex form (for line-oriented logs without JSON):
# name: mylog
# glob: ~/.mylog/*.log
# format: regex
# regex: '^(?P<time>\S+) (?P<model>\S+) in=(?P<input>\d+) out=(?P<output>\d+)$'
# time_layout: "2006-01-02T15:04:05Z07:00"
`
