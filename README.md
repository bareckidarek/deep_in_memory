# deep_in_memory

A terminal UI for exploring Linux process memory activity with live eBPF signals.

## What it does
- Streams `sched_process_fork` and `sched_process_exit` events via eBPF (build with `-tags ebpf`).
- Tracks processes (name, pid, start time UTC) and their virtual/physical page activity.
- Stores incoming process records in a local BoltDB so state survives restarts.
- Presents an interactive Bubble Tea/Lipgloss UI: left pane lists processes, right pane shows per-process virtual/physical summaries.
- Supports mouse wheel scrolling in the details panes; process selection happens on Enter.

## Layout
- Left: process list (`name (pid)`, start time UTC, selection on Enter).
- Right top: virtual view with summary and per-page visualization.
- Right bottom: physical PFN view.

## Build & run
```sh
# generate eBPF assets (requires clang/llvm and kernel headers)
go generate ./internal/ebpf/sched

# build with eBPF support
go build -tags ebpf ./cmd/gui

# run
./gui
```

If you want a stub build without eBPF, omit `-tags ebpf` (the collector will report a stub error).

## Data persistence
- BoltDB file `deep_in_memory.db` in the workspace directory.
- One document per process, keyed by PID.
- UI periodically reloads from BoltDB; it does not rely on live event ordering.

## Requirements
- Go 1.21+.
- Clang/LLVM and kernel headers for eBPF generation.
- Linux (eBPF, tracepoints).

## Notes
- Start times are stored/rendered in UTC.
- Exited processes are removed from both UI and DB on exit events.
- Target PID for deeper probes is written to a single-element eBPF map when you press Enter on a process.
