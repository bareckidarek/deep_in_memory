# AGENTS.md

## Project Overview

`deep_in_memory` is a Linux-only TUI for observing how a selected process uses virtual and physical memory. It combines Go, Bubble Tea, eBPF collectors, `/proc` parsing, and BoltDB-backed state to show process lifecycle events, memory mappings, page activity, and physical frame correlation.

## Setup Commands

- Go toolchain: `go 1.26.4`
- Prepare the local eBPF header from kernel BTF: `make prepare`
- Generate eBPF Go bindings: `make generate`
- Build the binary with eBPF support: `make build`
- Run the test suite: `make test`

Required system tools and libraries:

- `bpftool`
- `clang`
- `llvm`
- `libbpf-dev`
- kernel BTF available at `/sys/kernel/btf/vmlinux`

## Development Workflow

- Use `make prepare && make generate` after changing eBPF C sources or anything that affects generated bindings.
- The main application entrypoint is `cmd/deep_in_memory`.
- The app is intended to run with elevated privileges when attaching eBPF programs; `sudo ./deep_in_memory` is the usual local run.
- At startup the app deletes `./deep_in_memory.db` and `./deep_in_memory_addresses.db`, so runtime state is ephemeral by design.
- Logs are written to `./deep_in_memory.log`.

## Testing Instructions

- Run all Go tests with coverage collection: `make test`
- `make test` expands to `go test -v ./... -coverpkg=./... -coverprofile=coverage.out`
- Test files use the standard Go naming pattern `*_test.go`
- Focused debugging is usually easiest by running package-level tests with `go test ./internal/...` or a specific package path when needed

## Code Style

- Follow standard Go formatting with `gofmt`.
- Keep packages small and focused; the current layout is `internal/app`, `internal/data`, `internal/ebpf`, `internal/ui`, and `internal/thirdparty/db`.
- Preserve the existing split between orchestration, data parsing, collectors, and rendering.
- Generated eBPF files are part of the workflow; do not hand-edit generated artifacts unless the generation pipeline itself is being changed.

## Build And Run

- Production build: `make build`
- Local binary output: `./deep_in_memory`
- Container image build: `docker build -t deep_in_memory .`
- The build assumes Linux and eBPF-capable kernel support; the UI can start in degraded mode if collectors fail to attach, but tracing will be limited.

## Project Notes

- The UI is Bubble Tea-based and reacts to ordered event batches from the collectors.
- The application persists traced data in BoltDB, then periodically reloads it into the UI model.
- The `playground/` directory contains small C programs for generating predictable memory events during manual testing.
- For manual validation, run the app and a playground program in separate terminals, then select the process in the TUI and trace it.

## Pull Request Guidelines

- Keep changes focused on the affected layer and avoid unrelated formatting churn.
- Run `make test` before submitting code changes.
- If a change touches eBPF sources or generated bindings, rerun `make prepare && make generate` and confirm the repo still builds.

## Troubleshooting

- If BTF is missing, `make prepare` will fail because it reads `/sys/kernel/btf/vmlinux`.
- If collectors cannot attach, check kernel support and privileges first; the app should still start with reduced functionality.
- If physical-address correlation looks empty, verify that the process can read the relevant `/proc/<pid>/pagemap` and `/proc/iomem` data under the current permissions.