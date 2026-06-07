package data

import "fmt"


// getOrCreateThread returns the index of the thread with the given TID,
// creating it if it doesn't exist. Returns the thread index.
func (pd *ProcessData) getOrCreateThread(tid int) int {
	if pd.Threads == nil {
		pd.Threads = []ThreadData{}
	}
	for i, th := range pd.Threads {
		if th.TID == tid {
			return i
		}
	}
	pd.Threads = append(pd.Threads, ThreadData{
		TID:    tid,
		Ranges: []AddrRange{},
	})
	return len(pd.Threads) - 1
}

// AddMmapRange records a new virtual memory allocation from an mmap syscall.
// Creates or finds the thread's range list and adds the new address range.
// Automatically sorts ranges after insertion for consistent ordering.
func (pd *ProcessData) AddMmapRange(tid int, addr uint64, length uint64) {
	threadIdx := pd.getOrCreateThread(tid)

	// Create new range
	startAddr := addr
	endAddr := addr + length
	newRange := AddrRange{
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
	threadIdx := pd.getOrCreateThread(tid)

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
			newRange := AddrRange{
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
		// Invalidate cache for this thread
		pd.invalidateRangeCache(tid)
		break
	}
}