# AGENTS.md

This directory inherits the repository-root [AGENTS.md](../../AGENTS.md).

## Scope

`internal/ui` contains the Bubble Tea model, UI messages, and rendering code for the process list, virtual memory, physical memory, and log panes.

## Working Notes

- Keep model state changes and render code coordinated so cached viewport content stays valid.
- Preserve keyboard handling, focus transitions, and modal behavior when editing the model.
- Treat render helpers as presentation-only; keep data shaping in the model or upstream packages.
- When changing layout or styling, check `model/*`, `msg/*`, and `render/*` together.

## Testing

- Prefer focused package tests when available: `go test ./internal/ui/...`
- UI changes should be checked with existing package tests such as `go test ./internal/ui/...` or the narrower package that changed.

## Change Rules

- Avoid coupling the UI directly to persistence or parsing logic.
- Keep render functions side-effect free and deterministic for easier testing.