# deep_in_memory

`deep_in_memory` is a Linux TUI for observing how a selected process uses virtual and physical memory.
It combines eBPF tracing, `/proc` inspection, and a local BoltDB-backed state store to show:

- process lifecycle events (`exec`, `exit`)
- virtual memory changes (`mmap`, `munmap`, `brk`)
- page activity (`INIT`, `SWAP`, `UNSWAP`)
- pagemap lookups for a chosen virtual address
- `/proc/iomem` ranges correlated with resolved physical frames
- aggregated event logs for the currently selected PID

The interface is implemented with Bubble Tea and is intended to run on Linux with eBPF support enabled.

![Layout](playground/layout.png)

## What The Application Does

At startup the application:

1. opens two BoltDB databases for process metadata and address/page metadata
2. starts the Bubble Tea TUI
3. starts a `sched` eBPF collector and, if available, secondary collectors for `mmap`, `brk`, and paging events
4. batches and sorts traced events chronologically before applying them
5. periodically reloads the dataset from BoltDB and refreshes the UI

The main workflow is:

- choose a process from the left pane
- press `Enter` to make it the traced target PID
- inspect virtual ranges, page status counts, logs, and physical mapping details in the right pane

## Main Features

- Live process list with per-process detail panes
- Virtual memory view populated from `smaps` and updated by eBPF events
- Physical memory pane with pagemap lookup and `/proc/iomem` correlation
- Aggregated logs grouped by event type and address ranges
- Event ordering preserved across multiple eBPF collectors
- Async database writes to avoid blocking the TUI event loop
- Graceful degradation when eBPF collectors cannot be attached

## Requirements

Runtime requirements:

- Linux
- access to eBPF tracepoints supported by the running kernel
- permission to read `/proc/<pid>/smaps` and `/proc/<pid>/pagemap`
- root or sufficient capabilities for eBPF attachment; `/proc/iomem` is only useful with real addresses when run with root or `CAP_SYS_RAWIO`

Build requirements:

- Go `1.26.4`
- `bpftool`
- `clang`
- `llvm`
- `libbpf-dev`
- kernel BTF available at `/sys/kernel/btf/vmlinux`

## Build

Prepare the generated eBPF headers and Go bindings:

```bash
make prepare
make generate
```

Build the binary with eBPF support:

```bash
make build
```

This produces the executable:

```bash
./deep_in_memory
```

If you build without the `ebpf` tag, the application can still start, but the eBPF collectors fall back to stubs and tracing is unavailable.

## Run

Typical local run:

```bash
sudo ./deep_in_memory
```

You can also build the container image:

```bash
docker build -t deep_in_memory .
```

Notes:

- on every startup the application removes `./deep_in_memory.db` and `./deep_in_memory_addresses.db`, so runtime state is intentionally ephemeral
- logs are written to `./deep_in_memory.log`
- if a collector fails to attach, the UI shows a warning and continues running with reduced functionality

## TUI Controls

- `Up` / `Down`: move through the process list
- `Enter`: select the highlighted process and set it as the traced target PID
- `n`: next page of virtual ranges
- `p`: previous page of virtual ranges
- `Tab`: focus the physical-memory virtual address input
- `Enter` in the physical-memory input: run pagemap lookup for the current address
- `Up` / `Down` in the physical-memory input: navigate address suggestions
- `Esc` or `Tab` in the physical-memory input: leave input focus
- `j` / `k`: scroll the `/proc/iomem` column when the physical-memory input is not focused
- `w`: toggle word wrapping in logs
- `q` or `Ctrl+C`: quit

## Project Layout

- `cmd/deep_in_memory`: application entrypoint
- `internal/app`: orchestration, async DB writer, log aggregation, DB update helpers
- `internal/data`: process model, `smaps`, `pagemap`, and `iomem` parsing
- `internal/ebpf`: shared collector plumbing and individual eBPF collectors
- `internal/ui`: Bubble Tea model, messages, and rendering
- `internal/thirdparty/db`: BoltDB wrapper used by the application
- `playground`: test programs, screenshots, and notes for exercising memory events

## Example Workflow With playground

The `playground` directory contains small C programs that generate predictable memory events:

- `test_mmap.c`: large allocations that typically go through `mmap`/`munmap`
- `test_brk.c`: smaller allocations that tend to exercise heap growth paths
- `test_threads.c`: multi-threaded allocation scenario
- `test_all.c`: guided multi-scenario flow with prompts between steps

Example:

```bash
gcc -O2 -pthread -o playground/test_all playground/test_all.c
./playground/test_all
```

Suggested workflow:

1. start `deep_in_memory` with sufficient privileges
2. launch one of the `playground` binaries in a second terminal
3. select that process in the TUI and press `Enter`
4. step through the test program and observe range creation, page faults, swaps, and log output

## Test

Run the Go test suite:

```bash
make test
```

This executes:

```bash
go test -v ./... -coverpkg=./... -coverprofile=coverage.out
```

## Architecture Notes

- traced events are persisted in BoltDB, then periodically reloaded into the UI dataset
- address-level page offsets are stored separately from the lighter process model used for rendering
- an async writer serializes DB mutations so the UI thread does not block on storage
- a central event collector batches and sorts events by timestamp to preserve cross-collector order

## Known Constraints

- Linux-only
- no persisted historical session across launches because database files are deleted on startup
- physical frame and `/proc/iomem` correlation depends on permissions and kernel exposure
- tracing only applies to the PID currently selected in the UI