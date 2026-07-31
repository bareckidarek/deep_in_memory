# AGENTS.md — internal/data

## Package Overview

`internal/data` is the pure data layer — no UI imports, no eBPF imports, no side effects.
It defines all persistent data types, provides mutation helpers for virtual address ranges,
implements cached address lookup, and assembles `Dataset` objects for the UI.

`internal/data/proc` is a sub-package that reads Linux `/proc` virtual filesystem entries:
`/proc/PID/smaps`, `/proc/PID/pagemap`, and `/proc/iomem`.

**Files:**

| File | Responsibility |
|------|----------------|
| `types.go` | Core data types: `ProcessData`, `Process`, `ThreadData`, `AddrRange`, `LabelType`, `StatusCount`, `AddressData` |
| `dataset.go` | `BuildDataset` — assembles and sorts `Dataset` from a flat slice of `ProcessData` |
| `process_ranges.go` | Range mutation: `AddMmapRange`, `ExtendBrkRange`, `RemoveMmapRange` |
| `range_cache.go` | `RangeCache` — binary-search cache for fast `FindRangeForAddr` lookups |
| `range_status.go` | `SortRanges` — stable sort by `StartVal` |
| `address_data.go` | `AddressData` / `AddPageOffset` — page-offset detail stored in the address DB |
| `proc/smaps.go` | `LoadSmapsRanges` — parses `/proc/PID/smaps` |
| `proc/pagemap.go` | `ReadPagemap` — reads a single `/proc/PID/pagemap` entry |
| `proc/iomem.go` | `LoadIomem` — parses `/proc/iomem` |

---

## Data model

### Two-database split

| DB | Type | Content |
|----|------|---------|
| Main (`deep_in_memory.db`) | `ProcessData` | Process metadata, thread list, address ranges, **page counts** (`StatusCount`) |
| Address (`deep_in_memory_addresses.db`) | `AddressData` | Per-page offset lists per status per range — **never** loaded into the UI directly |

`StatusCount.Count` in `ProcessData` is always a mirror of `len(StatusOffsets.Value)` in `AddressData`.
`db_helper.go` in `internal/app` is responsible for keeping them in sync.

### Status codes used in maps

| Key | Meaning |
|-----|---------|
| `"INIT"` | Page anonymously faulted in (`do_anonymous_page`) |
| `"SWAP"` | Page written to swap (`swap_writepage`) |
| `"UNSWAP"` | Page read back from swap (`do_swap_page`) |

### LabelType classification on ranges

`AddrRange` stores both the raw `Label` string and a normalized `LabelType` enum.

Current `LabelType` values are:
- `FileBacked`
- `Heap`
- `Anonymous`
- `Stack`
- `Empty`
- `Other`

The UI's default virtual-memory filter shows only `Heap` and `Empty` ranges. Live mmap-created ranges are initialized with `LabelType=Empty`; heap ranges created by `brk` use `LabelType=Heap`.

---

## Range mutation API

All three methods live on `*ProcessData` in `process_ranges.go` and invalidate the `RangeCache`
for the affected TID after mutation.

### `AddMmapRange(tid int, addr uint64, length uint64)`

Adds a new `AddrRange` for `mmap` allocations. Computes `Size = length / PageSizeBytes`, formats
hex start/end strings, appends, and calls `SortRanges`.

### `ExtendBrkRange(tid int, brk uint64, ret uint64)`

Manages a special `[heap]` range:
- `brk == 0` → query: creates the initial `[heap]` range with `StartVal = EndVal = ret`.
- `brk != 0` → extend/shrink: adjusts `EndVal` and `Size` of the existing `[heap]` range.

### `RemoveMmapRange(tid int, addr uint64, length uint64)`

Trims or splits ranges that overlap with `[addr, addr+length)` (inclusive start / exclusive end). It preserves
non-overlapping segments and clears page-status counts for newly trimmed segments because old counts are no longer exact.

---

## RangeCache — fast address lookup

`FindRangeForAddr(tid int, addr uint64) *AddrRange` is called for every paging event. It uses a
binary-search over a sorted `StartVals []uint64` slice to achieve O(log n) lookups.

Cache is rebuilt when:
- A range is added/removed (explicit `invalidateRangeCache(tid)`).
- The cache is empty.
- More than `constants.RangeCacheRefreshInterval` (100 ms) has passed since last rebuild.

The cache lives inside `ProcessData.rangeCache map[int]*RangeCache` (unexported), so it is **not**
persisted to BoltDB. It is always rebuilt on first use after deserialization.

---

## BuildDataset

```go
func BuildDataset(processes []ProcessData) Dataset
```

Creates `Dataset{Processes map[int]*ProcessData, Order []int}`. Sorts `Order` by `Process.StartTime`
(ascending), using PID as a tiebreaker. Called:
- On startup with an empty slice.
- After each periodic DB reload in `app.go`.

---

## proc sub-package

### LoadSmapsRanges(pid int) ([]ThreadData, error)

Reads `/proc/PID/smaps`. All ranges are assigned `TID = PID` (smaps has no thread granularity).
Extracts hex address range, label (file path, `[heap]`, `[stack]`, `[anon: ...]`, etc.), `LabelType`, and
page counts from `Rss:` / `Swap:` lines via `parseKBToPages`.

### LoadSmapsRangesForThread(pid int, tid int) ([]AddrRange, error)

Reads `/proc/TID/smaps` for a specific task created through `clone(CLONE_THREAD, ...)`.
Used by `internal/app.DBHelper.HandleCloneEvent` to append a new thread view to an existing process.

**Label extraction rules (`extractLabel`):**
- Fewer than 6 fields → `LabelType=Empty`, `Label=""`.
- Empty remainder → `LabelType=Empty`, `Label=""`.
- Contains `[heap]` → `LabelType=Heap`.
- Contains `[stack]` → `LabelType=Stack`.
- Contains `anon` (including bracketed forms like `[anon: ...]`) → `LabelType=Anonymous`.
- Any other `[`-prefixed label → `LabelType=Other`.
- `/`-prefixed label → `LabelType=FileBacked`.
- Numeric-only remainder → `LabelType=Anonymous`.
- Fallback → `LabelType=Other`.

### ReadPagemap(pid int, vaddr uint64) (*PagemapEntry, error)

Reads a single 8-byte entry from `/proc/PID/pagemap`. Bit layout:

| Bits | Field |
|------|-------|
| 63 | Present |
| 62 | Swapped |
| 61 | File/shared page |
| 56 | Exclusively mapped |
| 55 | Soft-dirty |
| 54–0 | PFN (if present) or swap type/offset |

Requires `CAP_SYS_ADMIN` (or ptrace capability) for accurate PFN values.

### LoadIomem() ([]IomemRange, error)

Reads `/proc/iomem`. Parses lines of the form `  start-end : Name`.
Returns `[]IomemRange{Start, End, Name, Level}` where `Level` is determined by leading-space indentation
(2 spaces per level). Requires root or `CAP_SYS_RAWIO` for non-zero addresses.

---

## Testing

```bash
go test ./internal/data/...
go test ./internal/data/proc -run TestExtractLabel_BracketedLabel
```

Tests are in:
- `internal/data/data_test.go` — covers `ProcessData` helpers, range mutation, `BuildDataset`.
- `internal/data/proc/smaps_test.go` — covers `extractLabel`, `parseKBToPages`.

Use `testify/assert` and `require`. Prefer table-driven tests (see `TestParseKBToPages` as a model).

---

## Key constraints

- **No UI imports.** `internal/data` must never import `internal/ui/*`.
- **No eBPF imports.** Data types are populated by `internal/app` — not by eBPF packages directly.
- `PageSizeBytes` is initialized in `init()` via `syscall.Getpagesize()`. Do not hardcode 4096.
- `RangeCache` is unexported and never serialized — always rebuild after deserialization.
- `AddressData` is only read by the physical memory lookup in `internal/ui/model/model_physical.go`;
  it is **never** part of `Dataset` and is never passed to render helpers.
