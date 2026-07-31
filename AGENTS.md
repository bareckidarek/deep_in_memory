# AGENTS.md

## Project Overview

`deep_in_memory` is a Linux terminal UI (TUI) tool for real-time tracing and visualization of process lifecycle
and memory operations (clone, mmap, brk, page faults, swap) for a selected process. It uses eBPF for kernel-level event collection and
presents results in an interactive Bubble Tea terminal interface.

**Key technologies:**

- **Go 1.26.5** — primary language
- **eBPF** ([cilium/ebpf](https://github.com/cilium/ebpf)) — kernel tracing via tracepoints and kprobes
- **Bubble Tea v2** (`charm.land/bubbletea/v2`) — TUI framework
- **Bubbles v2** (`charm.land/bubbles/v2`) — TUI component library
- **Lipgloss v2** (`charm.land/lipgloss/v2`) — TUI styling
- **BoltDB** ([bbolt](https://github.com/etcd-io/bbolt) + [bolthold](https://github.com/timshannon/bolthold)) — embedded key-value persistence
- **msgpack v5** — BoltDB value serialization

The binary requires Linux with BTF support, kernel headers, and `CAP_BPF` / root privileges at runtime.

## Additional layer AGENTS.md
- internal/app/AGENTS.md
- internal/data/AGENTS.md
- internal/ebpf/AGENTS.md
- internal/ui/AGENTS.md

---

## Setup Commands

### Host build (requires Linux with BTF support)

```bash
# Install build dependencies (Debian/Ubuntu)
apt-get install -y bpftool clang llvm libbpf-dev

# 1. Generate vmlinux.h from running kernel BTF data
make prepare          # bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h

# 2. Generate eBPF Go bindings (runs go generate ./..., depends on prepare)
make generate

# 3. Build the binary
make build            # go build -o deep_in_memory -tags ebpf ./cmd/deep_in_memory
```

### Docker build (handles all dependencies automatically)

```bash
docker build -t deep_in_memory .
```

The multi-stage Dockerfile installs `bpftool`, `clang`, `llvm`, `libbpf-dev` in the build stage
and produces a minimal `distroless/static-debian12` runtime image.

---

## Development Workflow

- **Format code:** `make fmt`  (`go fmt ./...`)
- **Regenerate eBPF bindings after modifying `.c` files:**
  ```bash
  make prepare && make generate
  ```
- **Run the binary (requires root/CAP_BPF):**
  ```bash
  sudo ./deep_in_memory
  ```
- **Enable pprof profiling server (port 8080):**
  ```bash
  DEBUG=true sudo ./deep_in_memory
  # Profiling endpoint: http://localhost:8080/debug/pprof/
  ```
- **Log output:** Written to `deep_in_memory.log` in the working directory.
- **Databases:** `deep_in_memory.db` and `deep_in_memory_addresses.db` are deleted and recreated on every startup.

---

## Testing Instructions

All tests run without eBPF or root privileges (pure Go logic only — no eBPF build tag required):

```bash
# Full test suite with coverage
make test
# Equivalent: go test -v ./... -coverpkg=./... -coverprofile=coverage.out

# Per-package focused runs
go test ./internal/app/...
go test ./internal/data/...
go test ./internal/ebpf/...
go test ./internal/ui/...

# Single test by name
go test ./internal/app -run TestHandleMmapEvent_ProcessExists
go test ./internal/data/proc -run TestExtractLabel_BracketedLabel
```

Test files are co-located with packages using the `_test.go` suffix. Mocks for BoltDB are defined
inline in `internal/app/db_helper_test.go` — use `mockDB[T]` / `pidAwareMockDB` patterns for new
app-layer tests. Use `github.com/stretchr/testify/assert` and `require` for assertions.

---

## Architecture

```
eBPF collectors (sched/clone/mmap/brk/paging)
        │  emit uimsg.TimestampedEvent
        ▼
CentralEventCollector          ← batches + sorts by timestamp across all collectors
        │  sends sorted batch to Bubble Tea
        ▼
app.Model  (internal/app/app.go)
        │  routes events → AsyncDBWriter
        ▼
AsyncDBWriter                  ← serializes ALL BoltDB writes (single background goroutine)
        │
        ├──► ProcessData store  (deep_in_memory.db)       ← lightweight process/range/count state
        └──► AddressData store  (deep_in_memory_addresses.db) ← per-page offset detail per status
                │
        periodic reload → data.BuildDataset → ui model refresh
```

### Package responsibilities

| Package | Responsibility |
|---------|----------------|
| `cmd/deep_in_memory` | Wiring: opens DBs, creates Bubble Tea program, starts collectors |
| `internal/app` | Application model, event routing, DB write coordination, log aggregation |
| `internal/app/db_helper.go` | Write-side translation from eBPF events to process/range DB state |
| `internal/app/async_db_writer.go` | Serialized BoltDB writer, FIFO channel, PID exit ordering |
| `internal/app/log_aggregator.go` | Aggregates eBPF events for the selected PID into TUI log panel |
| `internal/data` | Data types, `BuildDataset`, range mutation, `RangeCache` lookups |
| `internal/data/proc` | `/proc` readers: `smaps`, `pagemap`, `iomem` |
| `internal/event` | Domain event types shared between eBPF collectors and the application layer |
| `internal/ebpf` | `CentralEventCollector`, `SharedState` (shared eBPF map for target PID) |
| `internal/ebpf/sched` | Tracepoints: `sched_process_exec`, `sched_process_exit` |
| `internal/ebpf/clone` | Tracepoints: `sys_enter_clone`, `sys_exit_clone` |
| `internal/ebpf/mmap` | Tracepoints: `sys_enter_mmap`, `sys_exit_mmap`, `sys_enter_munmap` |
| `internal/ebpf/brk` | Tracepoints: `sys_enter_brk`, `sys_exit_brk` |
| `internal/ebpf/paging` | Kprobes: `do_anonymous_page`, `do_swap_page`, `swap_writepage` |
| `internal/ui/model` | Bubble Tea TUI model (process list, virtual/physical panels, logs) |
| `internal/ui/render` | **Side-effect-free** render helpers; no timers or DB calls |
| `internal/ui/msg` | Bubble Tea message types plus aliases for domain events from `internal/event` |
| `internal/constants` | Layout, timing, and eBPF constants |
| `internal/thirdparty/db` | Thin BoltDB wrapper (generic `Db[T]` interface) |

---

## Key Conventions

### Database writes
- **All** BoltDB writes must go through `AsyncDBWriter`. Never call `db.Upsert` / `db.Delete` directly
  from the Bubble Tea `Update()` method or eBPF event handlers.
- Use `Enqueue(pid, handler)` for normal operations; use `EnqueueExit(pid, handler)` for process exit
  to guarantee the PID is marked dead before the delete is queued.

### Event ordering
- `CentralEventCollector` sorts batched domain events by timestamp before the app processes them.
- Do not bypass it by wiring secondary collectors directly to UI logic; keep Bubble Tea at the composition root in `cmd/deep_in_memory`.

### Data layer isolation
- `internal/data` must have **no UI imports** (no `internal/ui/*` dependencies).
- `internal/ui/render` helpers must be **side-effect free** — no DB calls, no timers, no `os` writes.
- Shape data in `internal/app`, `internal/data`, or the UI model; never in render helpers.

### Page-level vs. process-level state
- Page counts (`StatusCount`) belong in `ProcessData` (main DB).
- Individual page offsets (`StatusOffsets`) belong in `AddressData` (address DB).
- `db_helper.go` mirrors count updates from `AddressData` back into `ProcessData`.

### Target PID gating
- The shared eBPF `target_pids` map and app-level `targetPIDs` slice gate traced-event handling
  and log aggregation. Background process lifecycle events (`exec`/`exit`) are recorded broadly;
  `clone`, `mmap`, `brk`, and paging handling are filtered to the currently selected PID.
- `clone` handling distinguishes tasks from subprocesses: `CLONE_THREAD` adds a thread entry to the
  selected process, while a clone without `CLONE_THREAD` creates a separate child `ProcessData` record.

### eBPF generated files
- **Do not hand-edit** `*_bpf.go`, `*prog_bpf*.go`, or `*prog_bpfeb.go`/`*prog_bpfel.go` files.
- After changing `.c` eBPF sources or generation inputs, run `make prepare && make generate`.

### eBPF startup failures
- Treat eBPF startup failures as **degraded mode**, not fatal errors.
- Send `uimsg.ApplicationError{Severity: "warning"}` via the Bubble Tea `Send` function
  and continue so the TUI still runs.

### Stub files
- Each eBPF sub-package has a `collector_stub.go` with `//go:build !ebpf` that provides
  a no-op `Start` function. This allows the test suite to compile without the `ebpf` build tag.

---

## Code Style

Follow idiomatic Go conventions as described in `.github/instructions/go.instructions.md`.

- Run `make fmt` (`go fmt ./...`) before committing.
- Error wrapping: use `fmt.Errorf("context: %w", err)`.
- Exported types and functions must have Go doc comments.
- Mutex fields must immediately precede the data they protect in struct definitions.
- Use `sync.RWMutex` when reads significantly outnumber writes.
- Keep package names short and lowercase; avoid stutter (`data.ProcessData`, not `data.DataProcessData`).
- Prefer table-driven tests for functions with multiple input/output cases.

---

## Build and Deployment

| Command | Output |
|---------|--------|
| `make build` | `./deep_in_memory` binary (Linux amd64, requires eBPF support) |
| `docker build -t deep_in_memory .` | Distroless container image |

The binary embeds compiled eBPF programs via `go:embed` (generated by `go generate`).
No external eBPF object files are needed at runtime.

Runtime requirements:
- Linux kernel ≥ 5.8 with BTF and BPF ring buffer support
- `CAP_BPF` + `CAP_PERFMON` (or root)
- Writable working directory for `deep_in_memory.log`, `deep_in_memory.db`, `deep_in_memory_addresses.db`

---

## Debugging and Troubleshooting

| Issue | Resolution |
|-------|------------|
| `vmlinux.h: No such file or directory` | Run `make prepare` to regenerate from `/sys/kernel/btf/vmlinux` |
| eBPF load error at startup | Run as root; check kernel BTF support (`ls /sys/kernel/btf/vmlinux`) |
| Generated `*_bpf.go` out of date | Run `make prepare && make generate` after changing `.c` files |
| BoltDB write contention / UI freeze | All writes must use `AsyncDBWriter`, never direct DB calls in Bubble Tea `Update()` |
| Tests fail to compile | Ensure you are **not** using `-tags ebpf` for test runs; stub files handle this automatically |
| High memory usage | Enable pprof: `DEBUG=true sudo ./deep_in_memory` → `go tool pprof http://localhost:8080/debug/pprof/heap` |

---

## Pull Request Guidelines

- Run `make fmt` and `make test` before submitting.
- Changes to eBPF C sources (`.c` files in `internal/ebpf/*/`) must include regenerated Go bindings
  (`make prepare && make generate`).
- Tests must be added or updated for any logic changes in `internal/app`, `internal/data`, or `internal/ui`.
- Do not commit `deep_in_memory.db`, `deep_in_memory_addresses.db`, `deep_in_memory.log`,
  `coverage.out`, or the `deep_in_memory` binary.
