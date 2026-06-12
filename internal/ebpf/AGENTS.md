# AGENTS.md

This directory inherits the repository-root [AGENTS.md](../../AGENTS.md).

## Scope

`internal/ebpf` contains the collector plumbing plus the per-event eBPF programs and generated Go bindings for `brk`, `mmap`, `paging`, and `sched`.

## Working Notes

- If you change any `*.c` source or generation inputs, rerun `make prepare && make generate`.
- Do not hand-edit generated binding files such as `*_bpf.go` or `*prog_bpf*.go` unless you are changing the generation pipeline itself.
- Keep `collector.go`, `event_collector.go`, and `helpers.go` aligned with the per-collector packages.

## Testing

- Prefer focused package tests when available: `go test ./internal/ebpf/...`
- Validate eBPF changes with `make prepare`, `make generate`, and then `make build`.

## Change Rules

- Keep collector startup and shared-state behavior deterministic across all collector implementations.
- Be careful with kernel-facing changes; failures should degrade cleanly instead of crashing the UI.