# deep_in_memory

See what is really happening inside process memory while your application is running under load.

deep_in_memory is an interactive Linux TUI that combines eBPF tracing with /proc insights to show, in real time:

- process and thread lifecycle
- mmap and munmap activity, plus heap changes through brk
- lazy page allocation, page-out to swap, and page-in from swap
- virtual address to PFN lookup with /proc/iomem correlation

## Why use it

- End-to-end visibility: from kernel hook to human-readable terminal output.
- Practical debugging: quickly spot mmap churn, heap growth patterns, and swap pressure.
- Local and lightweight: no GUI dependency, no telemetry, works well over SSH.
- Reliable event flow: collector output is batched and timestamp-sorted to reduce interpretation races.

## Preview

![TUI preview](docs/preview.png)

## What you see in the UI

1. Process list
- Live process lifecycle with target PID selection.

2. Virtual memory panel
- mmap and munmap ranges plus heap brk updates, with range pagination and status counters.
- Default filter shows Heap plus Empty ranges; press one key to reveal all label types.

3. Physical memory panel
- Enter a virtual address, read pagemap PFN data, and match it against /proc/iomem ranges.

4. Logs panel
- Aggregated stream of mmap, munmap, brk, clone, init, swap_write, and swap_read events with counts and addresses.

## Quick start

Requirements:

- Linux on amd64
- Kernel 5.8 or newer with BTF support at /sys/kernel/btf/vmlinux
- Root privileges or CAP_BPF plus CAP_PERFMON

Build and run:

```bash
# Debian/Ubuntu
apt-get install -y bpftool clang llvm libbpf-dev

make prepare
make generate
make build

sudo ./deep_in_memory
```

Optional profiling:

```bash
DEBUG=true sudo ./deep_in_memory
```

## Keyboard shortcuts

| Key | Action |
|---|---|
| Up / Down | Navigate process list |
| Enter | Select process and start tracing |
| n / p | Next or previous virtual memory page |
| Tab | Focus physical memory input |
| Enter in input | Run pagemap lookup for current address |
| j / k | Scroll /proc/iomem matches |
| w | Toggle log line wrapping |
| f | Toggle range filter: heap+empty or all |
| q / Ctrl+C | Quit |

## Traced events

| Kernel hook | Event | Meaning |
|---|---|---|
| sched_process_exec | ProcessExec | Process started |
| sched_process_exit | ProcessExit | Process exited |
| sys_enter and sys_exit for clone and clone3 | ProcessClone | Thread or child process creation |
| sys_enter and sys_exit for mmap | ProcessMmap | New virtual memory range |
| sys_enter and sys_exit for munmap | ProcessMunmap | Range unmapped |
| sys_enter and sys_exit for brk | ProcessBrk | Heap boundary change |
| do_anonymous_page | INIT | First page allocation fault |
| swap_writepage | SWAP | Page evicted to swap |
| do_swap_page | UNSWAP | Page read back from swap |

## Architecture in 20 seconds

```text
eBPF collectors (sched/clone/mmap/brk/paging)
        -> internal/event domain types
        -> CentralEventCollector (batch + timestamp sort)
        -> app.Model (event routing)
        -> AsyncDBWriter (single goroutine for all writes)
        -> deep_in_memory.db (ProcessData)
        -> deep_in_memory_addresses.db (AddressData)
        -> data.BuildDataset -> TUI refresh
```

Most important behavior: events from different collectors are ordered by timestamp before processing, and all BoltDB writes are serialized through one queue. The canonical event types live in `internal/event` and are re-exported by `internal/ui/msg` for the Bubble Tea boundary.

## Docker

```bash
docker build -t deep_in_memory .
```

The container still needs eBPF privileges and host kernel data access.

## Development

```bash
make fmt
make test
make build
```

Tests do not require the ebpf build tag or root:

```bash
go test ./...
```

Contributor and agent details: [AGENTS.md](AGENTS.md)
