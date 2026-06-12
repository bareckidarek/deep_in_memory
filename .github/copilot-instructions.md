# Copilot Instructions

## Build, test, and format commands

- `make prepare` regenerates `vmlinux.h` from `/sys/kernel/btf/vmlinux`.
- `make generate` runs `go generate ./...` for eBPF bindings and already depends on `prepare`.
- `make build` builds the Linux binary with eBPF support: `go build -o deep_in_memory -tags ebpf ./cmd/deep_in_memory`.
- `make test` runs the full Go test suite with coverage: `go test -v ./... -coverpkg=./... -coverprofile=coverage.out`.
- Run focused package tests when changing a single layer:
  - `go test ./internal/app/...`
  - `go test ./internal/data/...`
  - `go test ./internal/ebpf/...`
  - `go test ./internal/ui/...`
- Run a single test with `-run`, for example:
  - `go test ./internal/app -run TestHandleMmapEvent_ProcessExists`
  - `go test ./internal/data/proc -run TestExtractLabel_BracketedLabel`
- `make fmt` runs `go fmt ./...`.

## High-level architecture

- `cmd/deep_in_memory/main.go` wires the application together. On startup it deletes `deep_in_memory.db` and `deep_in_memory_addresses.db`, opens fresh BoltDB-backed stores, creates the Bubble Tea program, starts the eBPF collectors, and installs UI-facing error callbacks.
- Runtime flow is: eBPF collectors emit `internal/ui/msg` events -> `internal/ebpf/CentralEventCollector` batches and sorts them by timestamp -> `internal/app.Model` routes them -> `internal/app.AsyncDBWriter` serializes all DB mutations -> periodic reloads rebuild a `data.Dataset` for the UI.
- Persistence is intentionally split across two stores:
  - `internal/data.ProcessData` in the main DB keeps lightweight process, thread, range, and status-count data for rendering.
  - `internal/data.AddressData` in the address DB keeps page-offset detail per status so the UI does not need to render or reload large per-page payloads from the main store.
- `internal/app/db_helper.go` is the write-side translation layer from traced events into persisted process/range state. It updates both DBs and mirrors page-status counts from `AddressData` back into `ProcessData`.
- `internal/data` owns normalization and lookup logic for `/proc` data, range mutation, dataset assembly, and cached address-to-range lookup. `BuildDataset` sorts processes by start time, then PID.
- `internal/ui` is a Bubble Tea TUI with a stateful model and presentation-only render helpers. The app layer, not the render layer, owns timers, DB reloads, log flushes, and `/proc/iomem` loading.

## Key repository-specific conventions

- Keep all BoltDB writes behind `internal/app.AsyncDBWriter`. `Update()` in the Bubble Tea path should enqueue work rather than writing synchronously.
- Preserve event ordering across collectors. Cross-collector ordering depends on `CentralEventCollector` sorting batched events by timestamp before the app processes them.
- Preserve process-exit semantics. `AsyncDBWriter.EnqueueExit` marks a PID as exited before queueing deletion so late events do not recreate dead processes.
- `targetPID` is the gate for traced-event handling and log aggregation. Background process lifecycle events can be recorded broadly, but mmap/brk/paging handling is intentionally filtered to the selected PID.
- Treat eBPF startup failures as degraded mode, not fatal startup errors. The TUI should still run and surface warnings through UI messages.
- Do not hand-edit generated eBPF artifacts such as `*_bpf.go` or `*prog_bpf*.go`. If eBPF C sources or generation inputs change, rerun `make prepare && make generate`.
- Keep the split between process-level state and page-level state. Counts belong in `ProcessData`; page offsets belong in `AddressData`.
- Keep `internal/data` free of UI concerns and keep `internal/ui/render` side-effect free. Shape data in `internal/app`, `internal/data`, or the UI model, not in render helpers.
