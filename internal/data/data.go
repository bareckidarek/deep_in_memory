package data

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"deep_in_memory/internal/constants"
)

var PageSizeBytes uint64

func init() {
	PageSizeBytes = uint64(syscall.Getpagesize())
}

// ProcessData aggregates all data for a single process.
type ProcessData struct {
	Proc       Process
	Threads    []ThreadData
	rangeCache map[int]*RangeCache // TID -> RangeCache for fast lookups
}

// Process mirrors a process definition.
type Process struct {
	PID       int
	Name      string
	StartTime time.Time
}

// ThreadData stores thread-level virtual ranges.
type ThreadData struct {
	TID    int
	Ranges []Range
}

// Range represents a virtual address range and statuses.
type Range struct {
	Start    string
	End      string
	StartVal uint64
	EndVal   uint64
	Size     uint64
	Statuses map[string]*StatusCount
	Label    string // Optional label like [heap], [stack], [anon: Go: gc bits], etc.
}

// StatusCount tracks count and list of page offsets in a particular status.
type StatusCount struct {
	Count int `json:"count"`
}

type AddressData struct {
	PID        int
	Threads    []ThreadAddressData
	rangeCache map[int]*AddressRangeCache
}

type ThreadAddressData struct {
	TID    int
	Ranges []AddressRange
}

type AddressRange struct {
	StartVal uint64
	EndVal   uint64
	Statuses map[string]*StatusValue
}

type StatusValue struct {
	Value []uint64 `json:"value,omitempty"` // List of page offsets (in pages, not bytes) from range start
}

// AddressRangeCache provides fast lookup of address ranges for page faults and swap events in AddressData
type AddressRangeCache struct {
	Ranges     []*AddressRange // Pointers to ranges for fast iteration
	StartVals  []uint64        // Corresponding start addresses for binary search
	LastUpdate time.Time       // When cache was last updated
}

// RangeCache provides fast lookup of address ranges for page faults and swap events
type RangeCache struct {
	Ranges     []*Range  // Pointers to ranges for fast iteration
	StartVals  []uint64  // Corresponding start addresses for binary search
	LastUpdate time.Time // When cache was last updated
}

// incrementStatus increments the count for a given status in the range.
func (r *Range) incrementStatus(status string) {
	if r.Statuses == nil {
		r.Statuses = make(map[string]*StatusCount)
	}
	if r.Statuses[status] == nil {
		r.Statuses[status] = &StatusCount{}
	}
	r.Statuses[status].Count++
}

// decrementStatus decrements the count for a given status in the range.
// If count goes below 0, it's clamped to 0.
func (r *Range) decrementStatus(status string) {
	if r.Statuses == nil {
		return
	}
	if r.Statuses[status] != nil {
		r.Statuses[status].Count--
		if r.Statuses[status].Count < 0 {
			r.Statuses[status].Count = 0
		}
	}
}

// SortRanges orders address ranges for consistent display in the UI.
// Priority order: heap ranges first, then anonymous ranges, then labeled ranges,
// and finally file/library-backed ranges. Within each category, ranges are sorted by address.
func SortRanges(ranges []Range) {
	sort.SliceStable(ranges, func(i, j int) bool {
		// Helper to check if label contains file/lib reference
		isFileRef := func(label string) bool {
			return strings.HasPrefix(label, "/") || strings.Contains(label, ".so")
		}

		// Rule 1: [heap] always first
		if ranges[i].Label == "[heap]" {
			return true
		}
		if ranges[j].Label == "[heap]" {
			return false
		}

		// Categorize ranges
		// Category 0: empty label
		// Category 1: non-empty label, no file/lib
		// Category 2: file/lib reference
		getCategory := func(r Range) int {
			if r.Label == "" {
				return 0
			}
			if isFileRef(r.Label) {
				return 2
			}
			return 1
		}

		catI := getCategory(ranges[i])
		catJ := getCategory(ranges[j])

		// Sort by category first
		if catI != catJ {
			return catI < catJ
		}

		// Within same category, sort by StartVal
		return ranges[i].StartVal < ranges[j].StartVal
	})
}

// Dataset is the top-level data container.
type Dataset struct {
	Processes map[int]*ProcessData
	Order     []int
}

// AddressDataset is the top-level container for address offset data
type AddressDataset struct {
	Processes map[int]*AddressData
}

// ===== AddressData methods for managing page offsets in separate database =====

// addPageToStatus adds a page offset to a specific status in AddressRange.
// The page is removed from all other statuses first (a page can only be in one status).
// pageOffset is in pages (not bytes) from the range start.
func (r *AddressRange) addPageToStatus(pageOffset uint64, status string) {
	if r.Statuses == nil {
		r.Statuses = make(map[string]*StatusValue)
	}

	// Remove page from all other statuses
	for otherStatus := range r.Statuses {
		if otherStatus != status {
			r.removePageFromStatus(pageOffset, otherStatus)
		}
	}

	// Initialize status if needed
	if r.Statuses[status] == nil {
		r.Statuses[status] = &StatusValue{Value: []uint64{}}
	}

	// Check if page already exists in this status
	for _, offset := range r.Statuses[status].Value {
		if offset == pageOffset {
			return // Already in this status
		}
	}

	// Add page to status
	r.Statuses[status].Value = append(r.Statuses[status].Value, pageOffset)
}

// removePageFromStatus removes a page offset from a specific status in AddressRange.
func (r *AddressRange) removePageFromStatus(pageOffset uint64, status string) {
	if r.Statuses == nil || r.Statuses[status] == nil {
		return
	}

	// Find and remove page
	for i, offset := range r.Statuses[status].Value {
		if offset == pageOffset {
			// Remove by swapping with last element and truncating
			lastIdx := len(r.Statuses[status].Value) - 1
			r.Statuses[status].Value[i] = r.Statuses[status].Value[lastIdx]
			r.Statuses[status].Value = r.Statuses[status].Value[:lastIdx]
			return
		}
	}
}

// invalidateAddressRangeCache marks the address range cache for a thread as stale
func (ad *AddressData) invalidateAddressRangeCache(tid int) {
	if ad.rangeCache != nil {
		delete(ad.rangeCache, tid)
	}
}

// getAddressRangeCache gets or creates an address range cache for a thread
func (ad *AddressData) getAddressRangeCache(tid int) *AddressRangeCache {
	if ad.rangeCache == nil {
		ad.rangeCache = make(map[int]*AddressRangeCache)
	}

	cache, exists := ad.rangeCache[tid]
	if !exists {
		ad.rangeCache[tid] = &AddressRangeCache{
			LastUpdate: time.Now(),
		}
		return ad.rangeCache[tid]
	}

	return cache
}

// rebuildAddressRangeCache reconstructs the internal cache for fast address range lookups in AddressData
func (ad *AddressData) rebuildAddressRangeCache(tid int) {
	cache := ad.getAddressRangeCache(tid)

	// Find thread
	threadIdx := -1
	for i, th := range ad.Threads {
		if th.TID == tid {
			threadIdx = i
			break
		}
	}

	if threadIdx == -1 {
		// Thread not found, clear cache
		cache.Ranges = nil
		cache.StartVals = nil
		return
	}

	thread := ad.Threads[threadIdx]
	cache.Ranges = make([]*AddressRange, len(thread.Ranges))
	cache.StartVals = make([]uint64, len(thread.Ranges))

	for i := range thread.Ranges {
		cache.Ranges[i] = &thread.Ranges[i]
		cache.StartVals[i] = thread.Ranges[i].StartVal
	}

	cache.LastUpdate = time.Now()
}

// findAddressRangeForAddr efficiently locates the address range containing a specific address in AddressData
func (ad *AddressData) findAddressRangeForAddr(tid int, addr uint64) *AddressRange {
	cache := ad.getAddressRangeCache(tid)

	// Rebuild cache if stale or empty
	if len(cache.Ranges) == 0 || time.Since(cache.LastUpdate) > constants.RangeCacheRefreshInterval {
		ad.rebuildAddressRangeCache(tid)
		cache = ad.rangeCache[tid]
	}

	// Binary search for the correct range
	left, right := 0, len(cache.StartVals)-1
	for left <= right {
		mid := left + (right-left)/2
		if addr >= cache.StartVals[mid] {
			if mid == len(cache.StartVals)-1 || addr < cache.StartVals[mid+1] {
				// Found the range - check if address is within bounds
				r := cache.Ranges[mid]
				if addr >= r.StartVal && addr < r.EndVal {
					return r
				}
				break
			}
			left = mid + 1
		} else {
			right = mid - 1
		}
	}

	// Linear search as fallback for edge cases
	for _, r := range cache.Ranges {
		if addr >= r.StartVal && addr < r.EndVal {
			return r
		}
	}

	return nil
}

// AddMmapAddressRange records a new virtual memory allocation in AddressData
func (ad *AddressData) AddMmapAddressRange(tid int, addr uint64, length uint64) {
	if ad.Threads == nil {
		ad.Threads = []ThreadAddressData{}
	}

	// Find or create thread
	var threadIdx = -1
	for i, th := range ad.Threads {
		if th.TID == tid {
			threadIdx = i
			break
		}
	}

	if threadIdx == -1 {
		// Create new thread
		ad.Threads = append(ad.Threads, ThreadAddressData{
			TID:    tid,
			Ranges: []AddressRange{},
		})
		threadIdx = len(ad.Threads) - 1
	}

	// Create new address range
	startAddr := addr
	endAddr := addr + length
	newRange := AddressRange{
		StartVal: startAddr,
		EndVal:   endAddr,
		Statuses: make(map[string]*StatusValue),
	}

	ad.Threads[threadIdx].Ranges = append(ad.Threads[threadIdx].Ranges, newRange)

	// Invalidate cache for this thread
	ad.invalidateAddressRangeCache(tid)
}

// ExtendBrkAddressRange manages heap growth/shrinkage in AddressData
func (ad *AddressData) ExtendBrkAddressRange(tid int, brk uint64, ret uint64) {
	if ad.Threads == nil {
		ad.Threads = []ThreadAddressData{}
	}

	// Find or create thread
	var threadIdx = -1
	for i, th := range ad.Threads {
		if th.TID == tid {
			threadIdx = i
			break
		}
	}

	if threadIdx == -1 {
		// Create new thread
		ad.Threads = append(ad.Threads, ThreadAddressData{
			TID:    tid,
			Ranges: []AddressRange{},
		})
		threadIdx = len(ad.Threads) - 1
	}

	// If brk==0, this is a query - ret contains the current break
	if brk == 0 {
		// Find existing heap range or create new one
		heapIdx := -1
		for i, r := range ad.Threads[threadIdx].Ranges {
			// We can't check label in AddressRange, so we find by matching ret value
			if r.StartVal <= ret && r.EndVal >= ret {
				heapIdx = i
				break
			}
		}

		if heapIdx == -1 {
			// Create initial heap range (start == end == ret)
			newRange := AddressRange{
				StartVal: ret,
				EndVal:   ret,
				Statuses: make(map[string]*StatusValue),
			}
			ad.Threads[threadIdx].Ranges = append(ad.Threads[threadIdx].Ranges, newRange)
		}
		return
	}

	// brk != 0: extending or shrinking the heap
	// Find existing heap range - assume it's the range containing or near ret
	heapIdx := -1
	for i, r := range ad.Threads[threadIdx].Ranges {
		if r.StartVal <= ret && ret <= r.EndVal+PageSizeBytes {
			heapIdx = i
			break
		}
	}

	if heapIdx != -1 {
		// Extend the existing heap range
		heap := &ad.Threads[threadIdx].Ranges[heapIdx]
		if ret > heap.EndVal {
			heap.EndVal = ret
		} else if ret < heap.EndVal {
			heap.EndVal = ret
		}

		ad.invalidateAddressRangeCache(tid)
	}
}

// RemoveMmapAddressRange removes a virtual address range from AddressData
func (ad *AddressData) RemoveMmapAddressRange(tid int, addr uint64, length uint64) {
	if ad.Threads == nil {
		return
	}

	endAddr := addr + length

	// Find thread
	for i, th := range ad.Threads {
		if th.TID != tid {
			continue
		}

		// Find and remove overlapping ranges
		newRanges := []AddressRange{}
		for _, r := range th.Ranges {
			// Check if range overlaps with munmap region
			if r.StartVal < endAddr && r.EndVal > addr {
				// This range overlaps, skip it (remove it)
				continue
			}
			newRanges = append(newRanges, r)
		}
		ad.Threads[i].Ranges = newRanges
		// Invalidate cache for this thread
		ad.invalidateAddressRangeCache(tid)
		break
	}
}

// AddPageFaultOffset adds a page fault offset to AddressData
func (ad *AddressData) AddPageFaultOffset(tid int, faultAddr uint64) {
	if ad.Threads == nil {
		return
	}

	// Calculate page address (align to page boundary)
	pageAddr := (faultAddr / PageSizeBytes) * PageSizeBytes

	// Use optimized range lookup
	r := ad.findAddressRangeForAddr(tid, pageAddr)
	if r != nil {
		// Calculate offset from range start
		pageOffset := (pageAddr - r.StartVal) / PageSizeBytes
		// Add to INIT status
		r.addPageToStatus(pageOffset, "INIT")
	}
}

// AddSwapOffset adds a swap offset to AddressData
func (ad *AddressData) AddSwapOffset(tid int, swapAddr uint64, isSwap bool) {
	if ad.Threads == nil {
		return
	}

	// Use optimized range lookup
	r := ad.findAddressRangeForAddr(tid, swapAddr)
	if r != nil {
		pageOffset := (swapAddr - r.StartVal) / PageSizeBytes

		if isSwap {
			// swap_writepage: move from INIT to SWAP
			r.removePageFromStatus(pageOffset, "INIT")
			r.addPageToStatus(pageOffset, "SWAP")
		} else {
			// do_swap_page: move from SWAP to UNSWAP
			r.removePageFromStatus(pageOffset, "SWAP")
			r.addPageToStatus(pageOffset, "UNSWAP")
		}
	}
}

// ===== End of AddressData methods =====

// invalidateRangeCache marks the range cache for a thread as stale
func (pd *ProcessData) invalidateRangeCache(tid int) {
	if pd.rangeCache != nil {
		delete(pd.rangeCache, tid)
	}
}

// getRangeCache gets or creates a range cache for a thread
func (pd *ProcessData) getRangeCache(tid int) *RangeCache {
	if pd.rangeCache == nil {
		pd.rangeCache = make(map[int]*RangeCache)
	}

	cache, exists := pd.rangeCache[tid]
	if !exists {
		pd.rangeCache[tid] = &RangeCache{
			LastUpdate: time.Now(),
		}
		return pd.rangeCache[tid]
	}

	return cache
}

// rebuildRangeCache reconstructs the internal cache for fast address range lookups.
// Creates sorted arrays of range pointers and start addresses for binary search.
// Called automatically when cache is stale or missing.
func (pd *ProcessData) rebuildRangeCache(tid int) {
	cache := pd.getRangeCache(tid)

	// Find thread
	threadIdx := -1
	for i, th := range pd.Threads {
		if th.TID == tid {
			threadIdx = i
			break
		}
	}

	if threadIdx == -1 {
		// Thread not found, clear cache
		cache.Ranges = nil
		cache.StartVals = nil
		return
	}

	thread := pd.Threads[threadIdx]
	cache.Ranges = make([]*Range, len(thread.Ranges))
	cache.StartVals = make([]uint64, len(thread.Ranges))

	for i := range thread.Ranges {
		cache.Ranges[i] = &thread.Ranges[i]
		cache.StartVals[i] = thread.Ranges[i].StartVal
	}

	cache.LastUpdate = time.Now()
}

// findRangeForAddr efficiently locates the virtual address range containing a specific address.
// Uses a cached binary search for O(log n) lookup performance, rebuilding the cache
// if it's stale or doesn't exist. Returns nil if no range contains the address.
func (pd *ProcessData) findRangeForAddr(tid int, addr uint64) *Range {
	cache := pd.getRangeCache(tid)

	// Rebuild cache if stale or empty
	if len(cache.Ranges) == 0 || time.Since(cache.LastUpdate) > constants.RangeCacheRefreshInterval {
		pd.rebuildRangeCache(tid)
		cache = pd.rangeCache[tid]
	}

	// Binary search for the correct range
	left, right := 0, len(cache.StartVals)-1
	for left <= right {
		mid := left + (right-left)/2
		if addr >= cache.StartVals[mid] {
			if mid == len(cache.StartVals)-1 || addr < cache.StartVals[mid+1] {
				// Found the range - check if address is within bounds
				r := cache.Ranges[mid]
				if addr >= r.StartVal && addr < r.EndVal {
					return r
				}
				break
			}
			left = mid + 1
		} else {
			right = mid - 1
		}
	}

	// Linear search as fallback for edge cases
	for _, r := range cache.Ranges {
		if addr >= r.StartVal && addr < r.EndVal {
			return r
		}
	}

	return nil
}

// getThread gets a thread by TID
func (pd *ProcessData) getThread(tid int) *ThreadData {
	for i := range pd.Threads {
		if pd.Threads[i].TID == tid {
			return &pd.Threads[i]
		}
	}
	return nil
}

// AddMmapRange records a new virtual memory allocation from an mmap syscall.
// Creates or finds the thread's range list and adds the new address range.
// Automatically sorts ranges after insertion for consistent ordering.
func (pd *ProcessData) AddMmapRange(tid int, addr uint64, length uint64) {
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
			Ranges: []Range{},
		})
		threadIdx = len(pd.Threads) - 1
	}

	// Create new range
	startAddr := addr
	endAddr := addr + length
	newRange := Range{
		Start:    fmt.Sprintf("%x", startAddr),
		End:      fmt.Sprintf("%x", endAddr),
		StartVal: startAddr,
		EndVal:   endAddr,
		Size:     length / PageSizeBytes,
		Statuses: make(map[string]*StatusCount),
	}

	pd.Threads[threadIdx].Ranges = append(pd.Threads[threadIdx].Ranges, newRange)
	// Sort ranges after adding new one
	SortRanges(pd.Threads[threadIdx].Ranges)

	// Invalidate cache for this thread
	pd.invalidateRangeCache(tid)
}

// ExtendBrkRange manages heap growth/shrinkage from brk syscalls.
// When brk==0, ret contains current break address (query operation).
// When brk!=0, ret is the actual allocated break address after attempted change.
// Maintains a special [heap] range that tracks heap memory usage.
func (pd *ProcessData) ExtendBrkRange(tid int, brk uint64, ret uint64) {
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
			Ranges: []Range{},
		})
		threadIdx = len(pd.Threads) - 1
	}

	// If brk==0, this is a query - ret contains the current break
	if brk == 0 {
		// Find existing heap range or create new one
		heapIdx := -1
		for i, r := range pd.Threads[threadIdx].Ranges {
			if r.Label == "[heap]" {
				heapIdx = i
				break
			}
		}

		if heapIdx == -1 {
			// Create initial heap range (start == end == ret)
			newRange := Range{
				Start:    fmt.Sprintf("%x", ret),
				End:      fmt.Sprintf("%x", ret),
				StartVal: ret,
				EndVal:   ret,
				Size:     0,
				Statuses: make(map[string]*StatusCount),
				Label:    "[heap]",
			}
			pd.Threads[threadIdx].Ranges = append(pd.Threads[threadIdx].Ranges, newRange)
			// Sort ranges after adding heap
			SortRanges(pd.Threads[threadIdx].Ranges)
		}
		return
	}

	// brk != 0: extending or shrinking the heap
	// Find existing heap range
	heapIdx := -1
	for i, r := range pd.Threads[threadIdx].Ranges {
		if r.Label == "[heap]" {
			heapIdx = i
			break
		}
	}

	if heapIdx == -1 {
		// No heap range exists yet, create one with ret as both start and end
		// This shouldn't normally happen, but handle it
		return
	}

	// Extend the existing heap range
	heap := &pd.Threads[threadIdx].Ranges[heapIdx]
	if ret > heap.EndVal {
		// Heap is growing
		heap.End = fmt.Sprintf("%x", ret)
		heap.EndVal = ret
		heap.Size = (ret - heap.StartVal) / PageSizeBytes
	} else if ret < heap.EndVal {
		// Heap is shrinking
		heap.End = fmt.Sprintf("%x", ret)
		heap.EndVal = ret
		if ret > heap.StartVal {
			heap.Size = (ret - heap.StartVal) / PageSizeBytes
		} else {
			heap.Size = 0
		}
	}
	// If ret == heap.EndVal, no change needed

	// Invalidate cache for this thread if heap was modified
	if heapIdx != -1 {
		pd.invalidateRangeCache(tid)
	}
}

// AddPageFault finds the virtual address range containing the faulted address
// and adds it to Statuses["INIT"] list.
func (pd *ProcessData) AddPageFault(tid int, faultAddr uint64) {
	if pd.Threads == nil {
		return
	}

	// Calculate page address (align to page boundary)
	pageAddr := (faultAddr / PageSizeBytes) * PageSizeBytes

	// Use optimized range lookup instead of O(n) linear search
	r := pd.findRangeForAddr(tid, pageAddr)
	if r != nil {
		// Found the range - increment INIT count
		r.incrementStatus("INIT")
	}
}

// AddSwapStatus finds the virtual address range containing the swap address
// and adds it to Statuses["SWAP"] or Statuses["UNSWAP"] based on isSwap flag.
func (pd *ProcessData) AddSwapStatus(tid int, swapAddr uint64, isSwap bool) {
	if pd.Threads == nil {
		return
	}

	// Use optimized range lookup instead of O(n) linear search
	r := pd.findRangeForAddr(tid, swapAddr)
	if r != nil {
		// Found the range
		if isSwap {
			// swap_writepage: SWAP++, INIT--
			r.incrementStatus("SWAP")
			r.decrementStatus("INIT")
		} else {
			// do_swap_page: UNSWAP++, SWAP--
			r.incrementStatus("UNSWAP")
			r.decrementStatus("SWAP")
			// If SWAP went to 0, also decrement INIT
			if r.Statuses != nil && r.Statuses["SWAP"] != nil && r.Statuses["SWAP"].Count <= 0 {
				r.decrementStatus("INIT")
			}
		}
	}
}

// RemoveMmapRange removes or marks freed a virtual address range for munmap.
func (pd *ProcessData) RemoveMmapRange(tid int, addr uint64, length uint64) {
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
		newRanges := []Range{}
		for _, r := range th.Ranges {
			// Check if range overlaps with munmap region
			if r.StartVal < endAddr && r.EndVal > addr {
				// This range overlaps, skip it (remove it)
				continue
			}
			newRanges = append(newRanges, r)
		}
		pd.Threads[i].Ranges = newRanges
		// Invalidate cache for this thread
		pd.invalidateRangeCache(tid)
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
			Proc: proc,

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

func convertRanges(rawRanges []rawRange) []Range {
	ranges := make([]Range, 0, len(rawRanges))
	for _, rr := range rawRanges {
		start, err1 := parseAddress(rr.StartAddress)
		end, err2 := parseAddress(rr.EndAddress)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		// Convert raw statuses to StatusCount
		statuses := make(map[string]*StatusCount)
		for key, val := range rr.Statuses {
			if val != nil {
				statuses[key] = val
			}
		}
		ranges = append(ranges, Range{
			Start:    rr.StartAddress,
			End:      rr.EndAddress,
			StartVal: start,
			EndVal:   end,
			Size:     end - start,
			Statuses: statuses,
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
	StartAddress string                  `json:"start_address"`
	EndAddress   string                  `json:"end_address"`
	Statuses     map[string]*StatusCount `json:"statuses"`
}

type rawData struct {
	Processes []rawProcess `json:"processes"`
}
