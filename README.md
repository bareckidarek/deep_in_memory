# deep_in_memory

A terminal UI for exploring Linux process memory activity with live eBPF tracing.

## Overview

`deep_in_memory` provides real-time visibility into process memory behavior using eBPF tracepoints. It tracks virtual memory allocations, page faults, swap events, and heap usage, presenting an interactive terminal interface with visual memory range bars and detailed per-process statistics.

## Features

### Process Tracking
- **Process lifecycle**: Captures `sched_process_exec` (when processes execute new programs) and `sched_process_exit` events
- **Process list**: Displays active processes with name, PID, and start time (UTC)
- **Target selection**: Press Enter on a process to enable detailed tracing for that specific PID

### Memory Event Tracing
- **Virtual memory allocation**: Tracks `mmap` and `munmap` syscalls showing address ranges and sizes
- **Heap management**: Monitors `brk` syscall for heap expansion/contraction  
- **Page faults**: Captures user-space page faults (`exceptions/page_fault_user`) with fault addresses
- **Swap activity**: Tracks page swap-out (`swap_writepage`) and swap-in (`do_swap_page`) events

### Visual Memory Display
- **Virtual memory ranges**: Color-coded bars showing allocated ranges per thread
  - Gray: unmapped/free regions
  - Green: initialized pages (INIT - page faulted)
  - Orange: swapped pages (SWAP)
  - Blue: recovered from swap (UNSWAP)
- **Range information**: Each bar displays `start-end (100%=Xp [size])` where:
  - `size`: total range size in human-readable units (kB, MB, GB)
  - `[Xp]`: number of pages in range
- **Per-thread breakdown**: Shows separate memory ranges for each thread (TID)
- **Special labels**: Identifies ranges like `[heap]`, `[stack]`, and other named regions from `/proc/PID/smaps`

### Summary Statistics
- **Virtual pages**: Total count of allocated pages across all ranges
- **Virtual usage**: Total memory allocated (automatically formatted: kB, MB, GB, TB, etc.)
- **Page size**: Automatically detected from system configuration via `getpagesize()`
- **Status counters**: Per-range counts of INIT, SWAP, and UNSWAP pages

## User Interface

### Layout
```
┌─────────────────────────┬────────────────────────────────────────┐
│  Process List           │  Virtual Memory Details                │
│  (name, PID, start)     │  - Process summary                     │
│  ► selected             │  - Per-thread memory ranges (bars)     │
│  • cursor focus         │  - Page/range visualization            │
│                         │  - Pagination for large range lists    │
├─────────────────────────┼────────────────────────────────────────┤
│                         │  Physical Memory (PFNs)                │
│                         │  - Physical frame numbers              │
│                         │  - (coming soon: full PFN tracking)    │
└─────────────────────────┴────────────────────────────────────────┘
```

### Navigation
- **Arrow keys**: Move cursor up/down in process list
- **Enter**: Select process and enable detailed tracing (sets target PID in eBPF map)
- **Mouse wheel**: Scroll through memory details panes
- **Page Up/Down**: Navigate through pages of memory ranges (30 ranges per page)
- **q / Ctrl+C**: Quit application

### Display Features
- **Auto-refresh**: UI updates every second from database while preserving cursor position
- **Selection markers**: 
  - `►` indicates selected process (detailed tracing active)
  - `•` shows cursor position
- **Color coding**: 
  - Green + bold for selected process
  - Blue for cursor focus
  - Status-based colors in memory bars
- **Responsive layout**: Automatically adjusts to terminal size

## Build & Run

### Prerequisites
- Go 1.21 or later
- Clang/LLVM toolchain
- Linux kernel headers (for your running kernel)
- Linux system with eBPF support

### Generate eBPF Assets
```sh
# Generate all eBPF collectors
go generate ./internal/ebpf/sched
go generate ./internal/ebpf/mmap
go generate ./internal/ebpf/brk
go generate ./internal/ebpf/page_fault
go generate ./internal/ebpf/swap
```

### Build
```sh
# Full build with eBPF support
go build -tags ebpf -o deep_in_memory ./cmd/gui

# Stub build (no eBPF, for testing UI only)
go build -o deep_in_memory ./cmd/gui
```

### Run
```sh
# Run with eBPF (requires root for eBPF operations)
sudo ./deep_in_memory
```

## Architecture

### eBPF Collectors
Each collector attaches to specific kernel tracepoints and streams events via ringbuffers:

- **sched**: Process lifecycle (`sched_process_exec`, `sched_process_exit`)
- **mmap**: Memory mapping (`sys_enter/exit_mmap`, `sys_enter/exit_munmap`) - filtered by target PID
- **brk**: Heap management (`sys_enter/exit_brk`) - filtered by target PID  
- **page_fault**: User page faults (`exceptions/page_fault_user`) - filtered by target PID
- **swap**: Swap events (`swap_writepage`, `do_swap_page` kprobes) - filtered by target PID

### Target PID Filtering
- Sched collector maintains a shared `target_pid` eBPF map (single-element array)
- When user selects a process (Enter key), the PID is written to this map
- Other collectors (mmap, brk, page_fault, swap) read from this map to filter events
- This minimizes performance overhead by only tracking details for the selected process

### Data Flow
```
eBPF Tracepoints → Ringbuffers → Go Collectors → Tea Messages → 
   App Model → BoltDB → Dataset → UI Model → Terminal Rendering
```

### Data Persistence
- **BoltDB** database (`deep_in_memory.db`) in current directory
- **Cleared on startup** to ensure clean state each run
- One document per process, keyed by PID (string)
- Process records include:
  - Process metadata (PID, name, start time)
  - Per-thread virtual address ranges with status counters
  - Virtual page lifecycle tracking (future: full PFN mapping)

### Page Size Handling
- Dynamically retrieved from OS using `syscall.Getpagesize()`
- Supports systems with non-4KB pages (e.g., ARM64 with 16KB/64KB pages)
- Used for all size calculations and conversions

## Technical Details

### Memory Range Tracking
- Ranges created on `mmap` success with size calculated in pages
- Heap ranges marked with `[heap]` label, tracked via `brk` syscalls
- Page fault events increment INIT counter for containing range
- Swap events adjust SWAP/UNSWAP/INIT counters:
  - `swap_writepage`: SWAP++, INIT--
  - `do_swap_page`: UNSWAP++, SWAP--, (conditionally INIT--)
- `munmap` removes overlapping ranges

### smaps Integration
- On process selection, `/proc/PID/smaps` is parsed
- Merges with eBPF-tracked ranges
- Adds labels like `[stack]`, `[heap]`, `[vdso]`, anonymous regions, mapped files
- Provides complete memory map even for pre-existing allocations

### Status Visualization
- Status bars show proportional colored segments for INIT/SWAP/UNSWAP pages
- Percentages calculated relative to total range size
- Empty portions shown as gray
- Text overlay displays addresses and size at bar ends

## Limitations & Future Work

- Physical memory (PFN) tracking is placeholder (infrastructure ready, full tracking pending)
- Swap tracking uses kprobes (may be kernel-version sensitive)
- High-frequency events (many page faults) may cause performance overhead
- Database cleared on startup (no persistent history across runs)

## License

Dual BSD/GPL (see source files)

## Requirements Summary
- **OS**: Linux with eBPF support (kernel 5.8+recommended)
- **Go**: 1.21 or later
- **Build tools**: clang, llvm, kernel headers
- **Runtime**: Root/CAP_BPF privileges for eBPF operations

## Separation: collector vs UI

The repository now implements an explicit separation between eBPF event collection/persistence and UI rendering inside the same process (code-level separation using goroutines):

- `internal/collector` — attaches eBPF programs, reads ringbuffers, converts raw events to application messages and persists state via `internal/persistence`.
- `internal/persistence` — thin adapter over the existing BoltDB code in `internal/thirdparty/db`; exposes `Writer` and `Reader` interfaces used by collector and UI.
- `internal/ui` — Bubble Tea UI runner that reads data only through `persistence.Reader` and renders the dataset.

Communication is strictly via BoltDB (writer = collector, reader = UI). A lightweight `LogAggregator` is passed from collector to UI for low-latency log lines; BoltDB remains the canonical source of truth.

Run and test instructions for the separated flow are included in the Build & Run section above.
