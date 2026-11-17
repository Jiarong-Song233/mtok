# Changelog

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
