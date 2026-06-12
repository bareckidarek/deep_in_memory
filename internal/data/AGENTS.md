# AGENTS.md

This directory inherits the repository-root [AGENTS.md](../../AGENTS.md).

## Scope

`internal/data` contains process/state models and parsers for `/proc`, pagemap lookups, range caching, and dataset assembly.

## Working Notes

- Keep parsing code deterministic and testable; prefer pure transformations over side effects.
- Preserve the distinction between process-level data and address/page-level data.
- Any change to `/proc` parsing may affect `dataset.go`, `process_ranges.go`, `range_cache.go`, and `proc/*` together.

## Testing

- Prefer focused package tests: `go test ./internal/data/...`
- Existing tests live in `data_test.go` and `proc/smaps_test.go`.

## Change Rules

- Avoid embedding UI concerns here; this package should stay focused on data normalization and parsing.
- Keep time and ordering logic stable, especially in dataset assembly and process sorting.