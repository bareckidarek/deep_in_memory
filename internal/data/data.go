package data

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const PageSizeBytes = 4096

// Process mirrors a process definition.
type Process struct {
	PID       int
	Name      string
	StartTime time.Time
}

// KernelEvent captures a kernel memory event.
type KernelEvent struct {
	Event          string
	PID            int
	Timestamp      time.Time
	VirtualAddress string
	PFN            string
}

// VirtualPage tracks virtual address lifecycle.
type VirtualPage struct {
	Address   string
	PFN       string
	Phase     string
	LastEvent string
	SeenAt    time.Time
	UpdatedAt time.Time
}

// PFNState tracks PFN lifecycle.
type PFNState struct {
	PFN       string
	Phase     string
	Virtuals  map[string]bool
	UpdatedAt time.Time
}

// ProcessData aggregates all data for a single process.
type ProcessData struct {
	Proc    Process
	Events  []KernelEvent
	Virtual map[string]*VirtualPage
	PFNs    map[string]*PFNState
	Threads []ThreadData
}

// ThreadData stores thread-level virtual ranges.
type ThreadData struct {
	TID    int
	Ranges []AddrRange
}

// AddrRange represents a virtual address range and statuses.
type AddrRange struct {
	Start    string
	End      string
	StartVal uint64
	EndVal   uint64
	Size     uint64
	Statuses map[string][]string
}

// Dataset is the top-level data container.
type Dataset struct {
	Processes map[int]*ProcessData
	Order     []int
}

// BuildDataset assembles a Dataset from a slice of process records.
// Processes are ordered by start time, then PID.
func BuildDataset(processes []ProcessData) Dataset {
	ds := Dataset{Processes: make(map[int]*ProcessData)}
	for i := range processes {
		p := processes[i]
		pid := p.Proc.PID
		// ensure maps are non-nil to avoid nil deref in UI
		if p.Virtual == nil {
			p.Virtual = make(map[string]*VirtualPage)
		}
		if p.PFNs == nil {
			p.PFNs = make(map[string]*PFNState)
		}
		ds.Processes[pid] = &p
		ds.Order = append(ds.Order, pid)
	}

	// Cache process data to avoid repeated map lookups in sort
	type procSort struct {
		pid       int
		startTime time.Time
	}
	sortCache := make([]procSort, len(ds.Order))
	for i, pid := range ds.Order {
		sortCache[i] = procSort{pid: pid, startTime: ds.Processes[pid].Proc.StartTime}
	}
	sort.Slice(ds.Order, func(i, j int) bool {
		a, b := sortCache[i], sortCache[j]
		if a.startTime.Equal(b.startTime) {
			return a.pid < b.pid
		}
		return a.startTime.Before(b.startTime)
	})

	return ds
}

// LoadData reads JSON input and produces a Dataset.
func LoadData(path string) (Dataset, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Dataset{}, fmt.Errorf("failed to read file %q: %w", path, err)
	}

	var raw rawData
	if err := json.Unmarshal(content, &raw); err != nil {
		return Dataset{}, fmt.Errorf("failed to parse JSON from %q: %w", path, err)
	}

	procMap := make(map[int]Process)
	threadMap := make(map[int][]ThreadData)
	for _, rp := range raw.Processes {
		procMap[rp.PID] = Process{
			PID:       rp.PID,
			Name:      rp.Name,
			StartTime: parseTime(rp.StartTime),
		}

		threads := make([]ThreadData, 0)
		if len(rp.Threads) > 0 {
			for _, rt := range rp.Threads {
				threads = append(threads, ThreadData{TID: rt.TID, Ranges: convertRanges(rt.Ranges)})
			}
		} else {
			threads = append(threads, ThreadData{TID: rp.PID, Ranges: convertRanges(rp.Ranges)})
		}
		threadMap[rp.PID] = threads
	}

	// Convert and sort events by timestamp for deterministic lifecycle tracking.
	events := make([]KernelEvent, 0, len(raw.KernelEvents))
	for _, re := range raw.KernelEvents {
		events = append(events, KernelEvent{
			Event:          re.Event,
			PID:            re.PID,
			Timestamp:      parseTime(re.Timestamp),
			VirtualAddress: re.VirtualAddress,
			PFN:            re.PFN,
		})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Timestamp.Before(events[j].Timestamp) })

	ds := Dataset{Processes: make(map[int]*ProcessData)}

	// Ensure all processes are included, even those without events.
	for pid, proc := range procMap {
		if _, exists := ds.Processes[pid]; !exists {
			ds.Processes[pid] = &ProcessData{
				Proc:    proc,
				Virtual: make(map[string]*VirtualPage),
				PFNs:    make(map[string]*PFNState),
				Threads: threadMap[pid],
			}
			ds.Order = append(ds.Order, pid)
		}
	}

	for _, ev := range events {
		pd, exists := ds.Processes[ev.PID]
		if !exists {
			continue
		}

		pd.Events = append(pd.Events, ev)

		// Track virtual address lifecycle.
		if ev.VirtualAddress != "" {
			vp, ok := pd.Virtual[ev.VirtualAddress]
			if !ok {
				vp = &VirtualPage{Address: ev.VirtualAddress, SeenAt: ev.Timestamp}
				pd.Virtual[ev.VirtualAddress] = vp
			}
			vp.PFN = coalesce(vp.PFN, ev.PFN)
			vp.LastEvent = ev.Event
			vp.UpdatedAt = ev.Timestamp
			vp.Phase = lifecyclePhase(vp.Phase, ev.Event)
		}

		// Track PFN usage independently.
		if ev.PFN != "" {
			pf, ok := pd.PFNs[ev.PFN]
			if !ok {
				pf = &PFNState{PFN: ev.PFN, Virtuals: make(map[string]bool)}
				pd.PFNs[ev.PFN] = pf
			}
			if ev.VirtualAddress != "" {
				pf.Virtuals[ev.VirtualAddress] = true
			}
			pf.Phase = lifecyclePhase(pf.Phase, ev.Event)
			pf.UpdatedAt = ev.Timestamp
		}
	}

	// Keep process order stable by start time (cache lookups)
	type procSort struct {
		pid       int
		startTime time.Time
	}
	sortCache := make([]procSort, len(ds.Order))
	for i, pid := range ds.Order {
		sortCache[i] = procSort{pid: pid, startTime: procMap[pid].StartTime}
	}
	sort.Slice(ds.Order, func(i, j int) bool {
		a, b := sortCache[i], sortCache[j]
		if a.startTime.Equal(b.startTime) {
			return a.pid < b.pid
		}
		return a.startTime.Before(b.startTime)
	})

	return ds, nil
}

func parseTime(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func lifecyclePhase(current, event string) string {
	// Simple mapping of kernel events to lifecycle phases.
	switch event {
	case "page_alloc":
		if current == "" {
			return "created"
		}
		return current
	case "page_fault_user":
		return "initialized"
	case "swap_writepage":
		return "swapped"
	case "do_swap_page":
		return "recovered"
	case "page_free":
		return "freed"
	default:
		return current
	}
}

func convertRanges(rawRanges []rawRange) []AddrRange {
	ranges := make([]AddrRange, 0, len(rawRanges))
	for _, rr := range rawRanges {
		start, err1 := parseAddress(rr.StartAddress)
		end, err2 := parseAddress(rr.EndAddress)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		ranges = append(ranges, AddrRange{
			Start:    rr.StartAddress,
			End:      rr.EndAddress,
			StartVal: start,
			EndVal:   end,
			Size:     end - start,
			Statuses: rr.Statuses,
		})
	}
	return ranges
}

func parseAddress(hexStr string) (uint64, error) {
	clean := strings.TrimPrefix(hexStr, "0x")
	return strconv.ParseUint(clean, 16, 64)
}

// Raw JSON structures (unexported).
type rawProcess struct {
	PID       int         `json:"pid"`
	Name      string      `json:"name"`
	StartTime string      `json:"start_time"`
	Ranges    []rawRange  `json:"virtual_address_ranges"`
	Threads   []rawThread `json:"threads"`
}

type rawThread struct {
	TID    int        `json:"tid"`
	Ranges []rawRange `json:"virtual_address_ranges"`
}

type rawRange struct {
	StartAddress string              `json:"start_address"`
	EndAddress   string              `json:"end_address"`
	Statuses     map[string][]string `json:"statuses"`
}

type rawKernelEvent struct {
	Event          string `json:"event"`
	PID            int    `json:"pid"`
	Timestamp      string `json:"timestamp"`
	VirtualAddress string `json:"virtual_address"`
	PFN            string `json:"pfn"`
}

type rawData struct {
	Processes    []rawProcess     `json:"processes"`
	KernelEvents []rawKernelEvent `json:"kernel_events"`
}
