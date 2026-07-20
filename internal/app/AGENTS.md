# AGENTS.md — internal/app

## Package Overview

`internal/app` is the application layer that sits between the eBPF event collectors and the Bubble Tea TUI.
It owns the top-level `Model`, serializes all database writes, aggregates trace events into log lines, and
reads `/proc` data on behalf of the UI.

**Files in this package:**

| File | Responsibility |
|------|----------------|
| `app.go` | Top-level `Model`, `Init`, `Update` — routes every Bubble Tea message |
| `db_helper.go` | `DBHelper` — translates eBPF events into `ProcessData` / `AddressData` mutations |
| `async_db_writer.go` | `AsyncDBWriter` — single background goroutine that serializes all BoltDB writes |
| `log_aggregator.go` | `LogAggregator` — deduplicates and formats eBPF events for the TUI log panel |

---

## Key Invariants

### All DB writes go through AsyncDBWriter

Never call `db.Upsert` or `db.Delete` directly from the Bubble Tea `Update()` path or eBPF event handlers.
Always use:

```go
m.asyncWriter.Enqueue(pid, func(h *DBHelper) error { ... })      // normal events
m.asyncWriter.EnqueueExit(pid, func(h *DBHelper) error { ... })  // process exit
```

`Enqueue` is non-blocking and drops events when the internal channel (`cap=5000`) is full — this prevents
UI freezes. `EnqueueExit` marks the PID as dead **before** queueing the delete, ensuring no subsequent
event can resurrect a dead process.

### handleTracedEvent — targetPIDs gate

All `clone`/`mmap`/`munmap`/`brk`/paging events are guarded by `targetPIDs`. Only events for the currently selected
process are persisted; background processes are ignored:

```go
func (m *Model) handleTracedEvent(pid int, handler func(*DBHelper) error, ...) (tea.Model, tea.Cmd) {
    found := false
    for _, target := range m.targetPIDs {
        if target == pid {
            found = true
            break
        }
    }
    if !found {
        return m, nil
    }
    m.asyncWriter.Enqueue(pid, handler)
    m.logAggregator.Record(...)
    return m, nil
}
```

### ProcessExit ordering guarantee

```
EnqueueExit(pid, ...)   // 1. marks PID dead in exitedPIDs map — future Enqueue calls rejected
    → Enqueues delete   // 2. FIFO guarantees delete runs after all earlier ops for this PID
    → exitedPIDs.Delete // 3. clean up after delete, allows PID reuse
```

---

## DBHelper — event handlers

| Method | Triggered by | Mutates |
|--------|-------------|---------|
| `HandleProcessExec` | `uimsg.ProcessExec` | Creates `ProcessData` in main DB |
| `HandleProcessExit` | `uimsg.ProcessExit` | Deletes from both DBs |
| `HandleCloneEvent` | `uimsg.ProcessClone` | Creates a child process record or appends a thread snapshot |
| `HandleMmapEvent` | `uimsg.ProcessMmap` | Calls `AddMmapRange` on existing process |
| `HandleMunmapEvent` | `uimsg.ProcessMunmap` | Calls `RemoveMmapRange` |
| `HandleBrkEvent` | `uimsg.ProcessBrk` | Calls `ExtendBrkRange` |
| `HandleSwapEvent` | `uimsg.ProcessInitSwapUnswap` | Updates `AddressData` offsets + mirrors counts to `ProcessData` |

`HandleCloneEvent` splits behavior by `CloneFlags`:
1. No `CLONE_THREAD` bit → create a new `ProcessData` record keyed by the returned child PID/TID.
2. `CLONE_THREAD` set → read `/proc/PID/task/TID/smaps`, then `AppendThread` to the existing selected process.

`HandleSwapEvent` is the only handler that writes to **both** databases. It:
1. Calls `AddPageOffset` on `AddressData` (address DB).
2. Uses `FindRangeForAddr` on the process record to mirror the count increment back into `ProcessData.Statuses`.
3. Upserts both records.

---

## LogAggregator

Aggregates events by `(eventType, address)` within a flush interval to avoid flooding the log panel.

Supported event types: `brk`, `mmap`, `munmap`, `clone`, `init`, `swap_write`, `swap_read`.

- `Record(eventType, addr, pid, tid)` — called synchronously from `Update()`, thread-safe.
- `Flush(pids)` — called on each `uimsg.GetLogsMsg` tick (`constants.LogFlushInterval`), converts aggregated
  counts into `[]LogEntry` history (newest-first, max 30 entries).
- `GetFormattedLogs()` — returns formatted `[]string` sent to UI via `uimsg.LogsResponseMsg`.

Display names for event types:

| Internal key | Displayed as |
|---|---|
| `brk` | `virtual memory mapping [brk]` |
| `mmap` | `virtual memory mapping [mmap]` |
| `munmap` | `virtual memory unmapping [munmap]` |
| `clone` | `process/thread cloning` |
| `init` | `lazy physical allocation` |
| `swap_write` | `page_out` |
| `swap_read` | `page_in` |

---

## Testing

Tests run without eBPF build tag. All tests are in `internal/app/` using the `app` package.

```bash
go test ./internal/app/...

# Focused runs
go test ./internal/app -run TestHandleMmapEvent_ProcessExists
go test ./internal/app -run TestLogAggregator
```

### Mock patterns

`db_helper_test.go` provides two in-memory mock DB implementations:

```go
// Generic mock — Query returns all stored values regardless of filter
type mockDB[T any] struct { store map[string]*T }

// ProcessData-specific mock with PID-aware Query
type pidAwareMockDB struct { store map[string]*data.ProcessData }
```

Use `mockDB[T]` for `AddressData` and `pidAwareMockDB` for `ProcessData`. Both implement the
`storedb.Db[T]` interface.

Assertions use `github.com/stretchr/testify/assert` and `require`. Prefer table-driven tests for
functions with multiple input/output cases.

---

## Adding a new event type

1. Add a new `uimsg.SomeEvent` type to `internal/ui/msg/msg.go` (implement `TimestampedEvent`).
2. Add a handler method `DBHelper.HandleSomeEvent(ev uimsg.SomeEvent) error` in `db_helper.go`.
3. Handle the message in `app.go` `Update()` using `handleTracedEvent` (if PID-gated) or `asyncWriter.Enqueue`.
4. Add the new event type key to `LogAggregator.events` in `log_aggregator.go` if it should appear in logs.
5. Write tests for the handler in `db_helper_test.go`.

---

## Common pitfalls

| Pitfall | Correct approach |
|---------|-----------------|
| Calling `db.Upsert` inside `Update()` directly | Always use `asyncWriter.Enqueue` |
| Using `asyncWriter.Enqueue` for process exit | Use `asyncWriter.EnqueueExit` — preserves exit ordering |
| Adding a new event type only to `Update()` | Also register in `LogAggregator` if log visibility is needed |
| Holding a pointer to `ProcessData` across `Enqueue` calls | Capture by value or copy before the closure |
