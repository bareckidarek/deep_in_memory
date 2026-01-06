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

// AddMmapRange adds a new virtual address range for a thread based on mmap event.
func (pd *ProcessData) AddMmapRange(tid int, addr, length uint64) {
	if pd.Threads == nil {
		pd.Threads = []ThreadData{}
	}

	// Find or create thread
	var threadIdx = -1
	for i, th := range pd.Threads {
		if th.TID == tid {
			threadIdx = i
			break
		}
	}

	if threadIdx == -1 {
		// Create new thread
		pd.Threads = append(pd.Threads, ThreadData{
			TID:    tid,
			Ranges: []AddrRange{},
		})
		threadIdx = len(pd.Threads) - 1
	}

	// Create new range
	startAddr := addr
	endAddr := addr + length
	newRange := AddrRange{
		Start:    fmt.Sprintf("%x", startAddr),
		End:      fmt.Sprintf("%x", endAddr),
		StartVal: startAddr,
		EndVal:   endAddr,
		Size:     length,
		Statuses: make(map[string][]string),
	}

	pd.Threads[threadIdx].Ranges = append(pd.Threads[threadIdx].Ranges, newRange)
}

// AddPageFault finds the virtual address range containing the faulted address
// and adds it to Statuses["INIT"] list.
func (pd *ProcessData) AddPageFault(tid int, faultAddr uint64) {
	if pd.Threads == nil {
		return
	}

	// Find thread
	var threadIdx = -1
	for i, th := range pd.Threads {
		if th.TID == tid {
			threadIdx = i
			break
		}
	}

	if threadIdx == -1 {
		// Thread not found
		return
	}

	// Find range containing faultAddr
	for rangeIdx, r := range pd.Threads[threadIdx].Ranges {
		if faultAddr >= r.StartVal && faultAddr < r.EndVal {
			// Found the range
			if pd.Threads[threadIdx].Ranges[rangeIdx].Statuses == nil {
				pd.Threads[threadIdx].Ranges[rangeIdx].Statuses = make(map[string][]string)
			}

			addrHex := fmt.Sprintf("%x", faultAddr)

			// Check if already present
			initList := pd.Threads[threadIdx].Ranges[rangeIdx].Statuses["INIT"]
			for _, existing := range initList {
				if existing == addrHex {
					return // Already tracked
				}
			}

			// Add to INIT status
			pd.Threads[threadIdx].Ranges[rangeIdx].Statuses["INIT"] = append(initList, addrHex)
			return
		}
	}
}

// RemoveMmapRange removes or marks freed a virtual address range for munmap.
func (pd *ProcessData) RemoveMmapRange(tid int, addr, length uint64) {
	if pd.Threads == nil {
		return
	}

	endAddr := addr + length

	// Find thread
	for i, th := range pd.Threads {
		if th.TID != tid {
			continue
		}

		// Find and remove overlapping ranges
		newRanges := []AddrRange{}
		for _, r := range th.Ranges {
			// Check if range overlaps with munmap region
			if r.StartVal < endAddr && r.EndVal > addr {
				// This range overlaps, skip it (remove it)
				continue
			}
			newRanges = append(newRanges, r)
		}
		pd.Threads[i].Ranges = newRanges
		break
	}
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

	// Sort processes by start time (then by PID for ties).
	sortProcessOrder(ds.Order, ds.Processes)

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

	ds := Dataset{Processes: make(map[int]*ProcessData)}

	// Initialize all processes from the input data.
	for pid, proc := range procMap {
		ds.Processes[pid] = &ProcessData{
			Proc:    proc,
			Virtual: make(map[string]*VirtualPage),
			PFNs:    make(map[string]*PFNState),
			Threads: threadMap[pid],
		}
		ds.Order = append(ds.Order, pid)
	}

	// Keep process order stable by start time (then by PID for ties).
	sortProcessOrder(ds.Order, procMap)

	return ds, nil
}

// sortProcessOrder sorts process IDs by their start time (with PID as tiebreaker).
// This function accepts either map[int]Process or map[int]*ProcessData as procSource.
func sortProcessOrder[T Process | *ProcessData](order []int, procSource map[int]T) {
	type procSort struct {
		pid       int
		startTime time.Time
	}

	sortCache := make([]procSort, len(order))
	for i, pid := range order {
		var startTime time.Time
		switch ps := any(procSource[pid]).(type) {
		case Process:
			startTime = ps.StartTime
		case *ProcessData:
			startTime = ps.Proc.StartTime
		}
		sortCache[i] = procSort{pid: pid, startTime: startTime}
	}

	sort.Slice(order, func(i, j int) bool {
		a, b := sortCache[i], sortCache[j]
		if a.startTime.Equal(b.startTime) {
			return a.pid < b.pid
		}
		return a.startTime.Before(b.startTime)
	})
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

type rawData struct {
	Processes []rawProcess `json:"processes"`
}
