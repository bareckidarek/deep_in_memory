# AGENTS.md — internal/ebpf

## Package Overview

`internal/ebpf` and its sub-packages implement all kernel-level event collection via eBPF.
The root package provides shared infrastructure (`CentralEventCollector`, `SharedState`, `ProcessEventsBatched`) and works with domain events defined in `internal/event`.
Each sub-package attaches its own tracepoints or kprobes and streams typed events to the collector.

**Package layout:**

| Package | Kernel hook | Events produced |
|---------|------------|----------------|
| `internal/ebpf` | — | Infrastructure only |
| `internal/ebpf/sched` | `sched_process_exec`, `sched_process_exit` tracepoints | `ProcessExec`, `ProcessExit` |
| `internal/ebpf/clone` | `sys_enter_clone`, `sys_exit_clone`, `sys_enter_clone3`, `sys_exit_clone3` tracepoints | `ProcessClone` |
| `internal/ebpf/mmap` | `sys_enter_mmap`, `sys_exit_mmap`, `sys_enter_munmap`, `sys_exit_munmap` tracepoints | `ProcessMmap`, `ProcessMunmap` |
| `internal/ebpf/brk` | `sys_enter_brk`, `sys_exit_brk` tracepoints | `ProcessBrk` |
| `internal/ebpf/paging` | `do_anonymous_page`, `do_swap_page`, `swap_writepage` kprobe+kretprobe pairs | `ProcessInitSwapUnswap` |

---

## Build tags

Every real collector file has `//go:build ebpf` at the top. Each sub-package also ships a
`collector_stub.go` with `//go:build !ebpf` that provides a `Start` stub returning an error.
This lets the entire test suite compile and run without the `ebpf` tag or root privileges.

```
internal/ebpf/mmap/collector.go        // //go:build ebpf
internal/ebpf/mmap/collector_stub.go   // //go:build !ebpf
```

**Never** add eBPF-specific imports or `link.*` calls outside `collector.go` files.

---

## Generated files — do not hand-edit

Each sub-package contains generated Go bindings produced by `go generate`:

```
*_bpf.go          ← go:generate directive + Load* functions
*prog_bpfeb.go    ← big-endian eBPF byte blob
*prog_bpfel.go    ← little-endian eBPF byte blob (used on amd64)
```

After changing any `.c` source, regenerate:

```bash
make prepare   # bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h
make generate  # go generate ./...
```

---

## CentralEventCollector

Defined in `event_collector.go`. Collects events from all sub-collectors and flushes them
in **chronological order** (sorted by `TimestampedEvent.GetTimestamp()`) to the app-layer adapter.

```go
collector := ebpf.NewCentralEventCollector(
        batchSend,
    constants.EventBatchSize,          // max events before forced flush
    constants.EventBatchFlushInterval, // periodic flush interval
)
```

- `AddEvent(event TimestampedEvent)` — called from each sub-collector goroutine; thread-safe.
- Flush sends a typed `[]event.TimestampedEvent` batch to the app-layer adapter. Bubble Tea stays at
    the wiring boundary in `cmd/deep_in_memory`.
- `Stop()` — cancels the flush loop and performs a final flush.

**Critical:** Do not bypass `CentralEventCollector` by wiring secondary collectors directly to UI logic.
Only `sched` sends individual domain events directly to the app-layer adapter (exec/exit events are not batched through the collector).

---

## SharedState

Defined in `collector.go`. Wraps the shared eBPF `target_pids` map used for kernel-side PID filtering.
All secondary collectors share this map via `MapReplacements` at load time — one eBPF map, one
kernel filter for all collectors simultaneously.

```go
func (s *SharedState) SetTargetPID(pid int) error // inserts pid key into the shared map
func (s *SharedState) GetEbpfMap() *ebpf.Map      // returns map for sharing
```

The `sched` collector creates the `SharedState` and returns it. `clone`, `mmap`, `brk`, and `paging`
receive it as an argument and inject their map replacement during collection spec loading.

---

## ProcessEventsBatched generic helper

Defined in `helpers.go`. Used by `clone`, `mmap`, `brk`, and `paging` collectors to avoid code duplication:

```go
func ProcessEventsBatched[T any](
    ctx context.Context,
    reader *ringbuf.Reader,
    processor EventProcessor[T],   // func(T) event.TimestampedEvent
    collector *CentralEventCollector,
) {
    // reads ring buffer, deserializes T, calls processor, adds to collector
}
```

Handles:
- `ringbuf.ErrClosed` → clean exit.
- Binary deserialization errors → skip and continue (prevents one corrupt event from stopping the collector).
- Transient errors → `time.Sleep(constants.EbpfRetryDelay)`.

---

## Sub-collector pattern

Every secondary collector (`clone`, `mmap`, `brk`, `paging`) follows the same structure:

```go
func Start(ctx context.Context, collector *ebpfshared.CentralEventCollector, sharedState *ebpfshared.SharedState) (func() error, error) {
    // 1. Load BPF spec (LoadXxxProg)
    // 2. Inject shared target_pids map via MapReplacements
    // 3. Attach links (tracepoints or kprobes)
    // 4. Create ringbuf reader
    // 5. Launch goroutine: ProcessEventsBatched(ctx, reader, processor, collector)
    // 6. Return cleanup func that closes all links + objects
}
```

The `sched` collector differs: it sends events directly to the app-layer adapter (not through
`CentralEventCollector`) because exec/exit events must reach the app immediately and do not need
timestamp-ordering with paging events.

The `clone` collector correlates `sys_enter_clone` and `sys_exit_clone` via its `clone_entries` map and emits
`uimsg.ProcessClone` with the returned thread/process ID in the `TID` field.

---

## eBPF C sources

| File | Kernel hooks | Map types used |
|------|-------------|----------------|
| `sched/sched_bpf.c` | `sched_process_exec`, `sched_process_exit` | `BPF_MAP_TYPE_RINGBUF`, `BPF_MAP_TYPE_HASH` (target_pids) |
| `clone/clone_bpf.c` | `sys_enter/exit_clone`, `sys_enter/exit_clone3` | `RINGBUF`, `HASH` (target_pids), `HASH` (enter→exit correlation) |
| `mmap/mmap_bpf.c` | `sys_enter/exit_mmap`, `sys_enter/exit_munmap` | `RINGBUF`, `HASH` (target_pids), `HASH` (enter→exit correlation) |
| `brk/brk_bpf.c` | `sys_enter/exit_brk` | `RINGBUF`, `HASH` (target_pids), `HASH` (enter→exit correlation) |
| `paging/paging_bpf.c` | `do_anonymous_page`, `do_swap_page`, `swap_writepage` kprobes/kretprobes | `RINGBUF`, `HASH` (target_pids), `HASH` (enter→exit correlation) |

All C sources include `vmlinux.h` (generated by `make prepare`) and use CO-RE (`bpf_core_read.h`).
Timestamps use `bpf_ktime_get_tai_ns()` in sched/clone/mmap/brk and `bpf_ktime_get_ns()` in paging.

---

## Startup failure handling

eBPF collectors must not crash the application. Treat any `Start()` error as degraded mode:

In `cmd/deep_in_memory/main.go`, startup errors are turned into `uimsg.ApplicationError` messages and
the TUI continues in degraded mode.

Secondary collectors (`clone`, `mmap`, `brk`, `paging`) are started in a loop in `startCollectors`; each
failure is independently reported as a warning, and successful collectors continue running.

---

## Testing

Tests run without `ebpf` build tag — stub files provide no-op implementations:

```bash
go test ./internal/ebpf/...
```

There are no unit tests for eBPF C programs (those require kernel execution). Integration-level
verification is done by running the binary on a live kernel. Logic in `helpers.go` and
`event_collector.go` can be unit-tested normally since they contain no eBPF imports.

---

## Adding a new collector

1. Create `internal/ebpf/mynew/` directory.
2. Write `mynew_bpf.c` (attach tracepoint or kprobe, emit events to ring buffer with timestamps).
3. Add `//go:generate` directive in `mynew_bpf.go` pointing to the C source.
4. Write `collector.go` (`//go:build ebpf`) following the sub-collector pattern above.
5. Write `collector_stub.go` (`//go:build !ebpf`) with a no-op `Start`.
6. Register in `cmd/deep_in_memory/main.go`'s `startCollectors` slice.
7. Run `make prepare && make generate`.
