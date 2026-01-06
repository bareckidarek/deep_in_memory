# deep_in_memory

A terminal UI for exploring Linux process memory activity with live eBPF signals.

## What it does
- Streams `sched_process_fork` and `sched_process_exit` events via eBPF (build with `-tags ebpf`).
- Tracks `mmap` and `munmap` syscalls to monitor virtual memory allocations.
- Tracks processes (name, pid, start time UTC) and their virtual/physical page activity.
- Stores incoming process records in a local BoltDB (cleared on startup for fresh state).
- Presents an interactive Bubble Tea/Lipgloss UI: left pane lists processes, right pane shows per-process virtual/physical summaries.
- Supports mouse wheel scrolling in the details panes; process selection happens on Enter.
- Arrow key navigation preserves cursor position during automatic UI refreshes.

## Layout
- Left: process list (`name (pid)`, start time UTC, selection on Enter).
- Right top: virtual view with summary and per-thread memory ranges (mmap allocations shown as gray bars).
- Right bottom: physical PFN view.

## Build & run
```sh
# generate eBPF assets (requires clang/llvm and kernel headers)
go generate ./internal/ebpf/sched
go generate ./internal/ebpf/mmap

# build with eBPF support
go build -tags ebpf ./cmd/gui

# run
./gui
```

If you want a stub build without eBPF, omit `-tags ebpf` (the collectors will report stub errors).

## Data persistence
- BoltDB file `deep_in_memory.db` in the workspace directory.
- Database is **cleared on application startup** to ensure clean state.
- One document per process, keyed by PID.
- UI periodically reloads from BoltDB (every second); cursor position is preserved during navigation.

## Requirements
- Go 1.21+.
- Clang/LLVM and kernel headers for eBPF generation.
- Linux (eBPF, tracepoints).

## Notes
- Start times are stored/rendered in UTC.
- Exited processes are removed from both UI and DB on exit events.
- Target PID for deeper probes is written to a single-element eBPF map when you press Enter on a process.
- Virtual memory ranges from `mmap` are displayed as gray bars until page status events are tracked.
