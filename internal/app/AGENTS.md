# AGENTS.md

This directory inherits the repository-root [AGENTS.md](../../AGENTS.md).

## Scope

`internal/app` owns the application coordinator that connects Bubble Tea, BoltDB writes, eBPF event handling, periodic refresh, and log aggregation.

## Working Notes

- Keep UI updates non-blocking; database work should stay behind the async writer.
- Preserve the ordering between process lifecycle events, traced events, and periodic reloads.
- Treat `targetPID` as the filter for all traced-event handling.
- When changing app behavior, check `db_helper.go`, `async_db_writer.go`, and `log_aggregator.go` together.

## Testing

- Prefer focused package tests when available: `go test ./internal/app/...`
- Update or add tests alongside behavior changes in `db_helper_test.go` and `log_aggregator_test.go`.

## Change Rules

- Do not block the Bubble Tea event loop with synchronous BoltDB work.
- Keep error handling user-visible where possible instead of silently dropping failures.