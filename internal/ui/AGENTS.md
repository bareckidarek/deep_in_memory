# AGENTS.md — internal/ui

## Package Overview

`internal/ui` contains the Bubble Tea TUI implementation split into three sub-packages:

| Package | Responsibility |
|---------|----------------|
| `internal/ui/model` | Stateful Bubble Tea model — owns layout, keyboard handling, viewport management, pagemap lookups |
| `internal/ui/render` | **Side-effect-free** render helpers — pure functions that produce styled strings |
| `internal/ui/msg` | All Bubble Tea message types and the `TimestampedEvent` interface |

---

## msg — message types

`msg.go` defines every message that flows through the Bubble Tea program.

### eBPF-originated events (implement `TimestampedEvent`)

| Type | Source | Fields |
|------|--------|--------|
| `ProcessExec` | sched collector | `PID`, `Comm`, `Timestamp` |
| `ProcessExit` | sched collector | `PID`, `Comm`, `Timestamp` |
| `ProcessClone` | clone collector | `PID`, `TID`, `CloneFlags`, `TLS`, `Comm`, `Timestamp` |
| `ProcessMmap` | mmap collector | `PID`, `TID`, `Addr`, `Len`, `Timestamp` |
| `ProcessMunmap` | mmap collector | `PID`, `TID`, `Addr`, `Len`, `Timestamp` |
| `ProcessBrk` | brk collector | `PID`, `TID`, `Brk`, `Ret`, `Timestamp` |
| `ProcessInitSwapUnswap` | paging collector | `PID`, `TID`, `Address`, `Type` (0=INIT/1=SWAP/2=UNSWAP), `ErrorCode`, `Timestamp` |

### UI-to-app requests

| Type | Purpose |
|------|---------|
| `LoadSmapsRequest{PID}` | Triggers `/proc/PID/smaps` read and DB upsert |
| `GetLogsMsg{}` | Periodic tick — causes `LogAggregator.Flush` + log refresh |
| `LogsResponseMsg{Logs []string}` | Response from app to UI with formatted log lines |
| `IomemRangeMsg` | Carries parsed `/proc/iomem` ranges to the UI model |
| `HideModalMsg{}` | Dismisses the notification modal after `constants.ModalDisplayDuration` |
| `ClearPhysicalMemoryMsg{PID}` | Resets physical memory panel when a process exits |
| `ApplicationError{Message, Severity}` | Displays a warning or error modal; severity: `"warning"` or `"error"` |

---

## model — TUI state machine

### Layout

The screen is divided into three columns at runtime:

```
┌──────────────────┬──────────────────────────┬────────────────────────────┐
│  [processes]     │  [virtual memory]  55%    │  [physical memory] 45%     │
│  scrollable list │  viewport: ranges/summary │  viewport: pagemap/iomem   │
│                  ├──────────────────────────┴────────────────────────────┤
│                  │  [logs]  (bottom strip)                                │
└──────────────────┴───────────────────────────────────────────────────────┘
```

Heights: `VirtualMemoryPercent = 55%`, `PhysicalMemoryPercent = 45%` of available height (from `constants`).

### Key files

| File | Contents |
|------|----------|
| `model.go` | `Model` struct, `New`, `Update`, `View`, `SetDataset`, `SetTargetPIDSetter` |
| `model_keys.go` | `keyHandlers()` — `q`, `ctrl+c`, `enter`, `n`/`p` (page), `tab`, `j`/`k`, `w`, `f` |
| `model_layout.go` | `syncLayout`, `layoutMetrics`, `updateStyles`, `updateViewports` |
| `model_process.go` | List delegate rendering, `refreshList`, `renderListItem` |
| `model_physical.go` | Physical memory input handling, pagemap lookup, suggestions dropdown |

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate process list |
| `Enter` | Select process → sets target PID in eBPF, loads smaps, shows info modal |
| `n` / `p` | Next/previous page of virtual memory ranges |
| `tab` | Toggle focus to/from physical memory address input |
| `j` / `k` | Scroll `/proc/iomem` matches in the physical memory panel |
| `w` | Toggle word-wrap in logs panel |
| `f` | Toggle virtual-range `LabelType` filter: `Heap`+`Empty` only vs. all label types |
| `q` / `ctrl+c` | Quit |
| `esc` | Blur physical memory input |

### dirty flag and view caching

`Model.dirty` is set to `true` whenever state changes. `View()` checks `dirty`:
- If `true`: re-renders all panels, caches result in `cachedView`, clears `dirty`.
- If `false`: returns `cachedView` directly (avoids re-rendering on every Bubble Tea tick).

Similarly, `cachedVirtual`, `cachedPhysical`, `cachedLogs` cache individual viewport content strings.
Styles (`virtualStyle`, `physicalStyle`, `logsStyle`) are only recreated when layout dimensions change
(`lastAvailable`, `lastRowHeight`, `lastSecondRowHeight`).

### Physical memory panel

The physical input accepts a hex address (e.g. `0x7f00001000`). On `Enter`:
1. Reads `/proc/PID/pagemap` for the address.
2. Resolves the PFN.
3. Matches the PFN against `iomemRanges` loaded at startup.

Typing in the input triggers `updateSuggestions()` which matches the partial hex prefix against
known range start addresses in the selected process and displays a dropdown.

---

## render — side-effect-free helpers

**Invariant:** render helpers must not call the OS, access DBs, start timers, or produce side effects.
They receive data and return styled strings only.

### render.go

| Function | Purpose |
|----------|---------|
| `CalculateStatusStats(pd)` | Aggregates INIT/SWAP/UNSWAP page counts across all threads |
| `HumanBytes(b int64)` | Formats byte count as human-readable (B / KiB / MiB / GiB) |
| `StyleTitle(s)` | Applies standard section-title styling |

### render_processes.go

| Function | Purpose |
|----------|---------|
| `FormatListItem(proc, pid)` | Returns `(title, description)` strings for the list |
| `RenderProcessListItem(title, desc, isCursor, isSelected)` | Applies cursor/selection markers and colors |

Selection markers:
- `►` + bold + selected color → selected process (target PID)
- `•` + focus color → cursor (not yet selected)
- Two spaces → neither

### render_virtual.go

| Function | Purpose |
|----------|---------|
| `RenderVirtualEmptyContent()` | Placeholder text when no process is selected |
| `RenderSummary(proc, pd)` | Bordered table: reservation / allocation / page-out / page-in (pages + human bytes) |
| `RenderVirtualSectionContent(summary, pd, barWidth, page, viewportWidth, showAll)` | Summary + paginated ranges + filter legend |

Virtual-range filtering is driven by `AddrRange.LabelType`:
- Default view (`showAll=false`) renders only `Heap` and `Empty` ranges.
- Pressing `f` switches to `[all]`, which exposes the remaining label types without changing the stored data.
- The viewport shows a filter legend line: `Filter: [heap+empty]` or `Filter: [all]`.

Status bar colors (from `constants`):
- Gray → virtual reservation
- Green → INIT (physical allocation)
- Red → SWAP (page-out)
- Blue → UNSWAP (page-in)

### render_physical.go

Renders the right column: pagemap lookup result, PFN, iomem match, address suggestions dropdown.

### render_logs.go

`RenderLogsSectionContent(logs, viewportWidth, logsWrap)` — builds the logs panel content with optional
word-wrap using `ansi.Wordwrap`.

### render_modal.go

`RenderWithModal(baseContent, modalType, modalMessage, width, height)` — overlays a colored 3-line
notification bar at `constants.ModalTopOffset` rows from the top. Colors: info=blue(12), warn=orange(214),
error=red(9).

---

## Testing

The render and msg packages contain no test files (pure functions / type definitions).
The model package has no dedicated unit tests — model behavior is covered by integration in `internal/app`.

```bash
go test ./internal/ui/...
```

---

## Adding a new UI message

1. Add the type to `internal/ui/msg/msg.go`.
2. If it carries eBPF event data, implement `GetTimestamp() time.Time`.
3. Handle it in `internal/app/app.go` `Update()`.
4. If the UI model needs to react, also handle it in `internal/ui/model/model.go` `Update()`.

## Adding a new render helper

1. Add a pure function to the appropriate `render_*.go` file.
2. Do not add imports from `internal/app`, `internal/ebpf`, or any OS I/O.
3. Accept `data.*` types as parameters; return `string`.
4. Call it from `internal/ui/model` (not from `internal/app`).
