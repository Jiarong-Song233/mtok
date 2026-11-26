# Changelog

## v0.3.7 — 2026-11-25

- Read Gemini token usage from the newer `~/.gemini/antigravity-cli/brain` transcripts, not just the original `antigravity` directory. The CLI records `model` on every response; Gemini's trailing thinking-level segment (`-high`, `-medium`, ...) is stripped so sessions merge by model. Gemini now appears in the model table with its own token totals.

## v0.3.6 — 2026-11-25

- Read 5-hour and weekly allowances from the local T3 gateway caches (`~/.t3/caches/*.json`), so Gemini, Claude Code and Grok quotas appear alongside Codex and magpie. Codex keeps using its session-log source. The "use each CLI's /status" hint now shows only when no local reading exists.

## v0.3.5 — 2026-11-18

- Merge cache read and cache write into a single `缓存` report column. Internal storage, JSON output, and pricing still keep the two rates separate.

## v0.3.4 — 2026-11-18

- Refresh all Claude and GPT-6.2 built-in USD prices from LiteLLM, including separate cache read/write rates.
- Add aliases and prices for GPT-6.2-Astra-Law, Claude Opus 4.5/4.7, Sonnet 4.5/4.6/5, Fable 5, and dated Claude snapshots.
- Publish the first GitHub Release with Windows/Linux/macOS binaries and SHA256 checksums; add CI, release workflow, issue templates, `docs/caliber.md`, `CONTRIBUTING.md`, and a v0.3.4 Scoop manifest.

## v0.3.3 — 2026-11-18

- Read Anthropic's `cache_creation_input_token_cost` as cache-write price during `--fetch`; this removes stale Claude cache-write rates.
- Update built-in Claude prices to the current USD reference rates.

## v0.3.2 — 2026-11-18

- Split cache read and cache write into separate report columns and price them with separate rates.
- Add `mtok pricing --fetch` to pull community prices from LiteLLM on demand; every fetch stays explicit and local writes are USD-based.
- Simplify the report's closing notes.

## v0.3.1 — 2026-11-17

- Reference cost now settles in USD by default; CNY remains available with `currency: CNY` and a configurable `rate`.
- Token counts use `B` for billions; the report adds a `总和` column after `缓存` (input + output + cache read/write).
- Add `mtok pricing` and `mtok pricing --update` to inspect the price source and rebuild `pricing.json` from the embedded table locally, with no network calls.
- Add built-in pricing and aliases for `Hy4-Flash` (`hy4flash` / `hy4-flash-free`).

## v0.3.0 — 2026-11-17

- Add a local Codex allowance reader for `~/.codex/sessions/**/rollout-*.jsonl[.zst]` events (`event_msg` / `token_count` / `rate_limits`). It reports 5h and 7-day used/remaining percentages and reset times without calling any vendor API.
- Add `mtok quota` and include Codex + magpie allowances in the default report's `剩余额度` block. A local Codex reading overrides magpie's Codex entry.
- Cache Codex quota data in `%LOCALAPPDATA%\mtok\quota-cache.json`. The first scan reads archives; later runs only check files modified since the last read (~50ms cached).
- Add `budget.daily` and `budget.monthly` to `config.yaml`. `mtok --check` exits 3 when a limit is exceeded, and the normal report highlights breaches.
- Add `schema_version: "1.0"` to `--json` and a `mtok schema` command describing fields, sources, and exit codes.
- `--no-magpie` now skips proxy usage and proxy quotas while keeping local Codex quota data.
- Add unit tests for Codex parsing, zstd archives, cache fallback, quota/budget JSON, and CLI flags.

## v0.2.1 — 2026-11-16

- Ship default binary version 0.2.1, add `docs/ROADMAP.md`, and sync the scoop manifest.
