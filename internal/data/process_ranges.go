package data

import (
	"fmt"
	"time"
)

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

func (pd *ProcessData) AppendThread(thread ThreadData) {
	if pd.Threads == nil {
		pd.Threads = []ThreadData{}
	}
	pd.Threads = append(pd.Threads, thread)

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
		Start:     fmt.Sprintf("%x", startAddr),
		End:       fmt.Sprintf("%x", endAddr),
		StartVal:  startAddr,
		EndVal:    endAddr,
		Size:      length / PageSizeBytes,
		PrevSize:  length / PageSizeBytes, // Equal to Size: no arrow on initial creation
		Statuses:  make(map[string]*StatusCount),
		ChangedAt: time.Now(), // Highlight new ranges
		LabelType: Empty,
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
				Start:     fmt.Sprintf("%x", ret),
				End:       fmt.Sprintf("%x", ret),
				StartVal:  ret,
				EndVal:    ret,
				Size:      0,
				Statuses:  make(map[string]*StatusCount),
				Label:     "[heap]",
				LabelType: Heap,
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
		oldSize := heap.Size
		heap.End = fmt.Sprintf("%x", ret)
		heap.EndVal = ret
		heap.Size = (ret - heap.StartVal) / PageSizeBytes
		if heap.Size != oldSize {
			if oldSize == 0 && heap.PrevSize == 0 {
				// First resize from zero: record current size so no arrow is shown
				heap.PrevSize = heap.Size
			} else {
				heap.PrevSize = oldSize
			}
			heap.ChangedAt = time.Now()
		}
	} else if ret < heap.EndVal {
		// Heap is shrinking
		oldSize := heap.Size
		heap.End = fmt.Sprintf("%x", ret)
		heap.EndVal = ret
		if ret > heap.StartVal {
			heap.Size = (ret - heap.StartVal) / PageSizeBytes
		} else {
			heap.Size = 0
		}
		if heap.Size != oldSize {
			if oldSize == 0 && heap.PrevSize == 0 {
				// First resize from zero: record current size so no arrow is shown
				heap.PrevSize = heap.Size
			} else {
				heap.PrevSize = oldSize
			}
			heap.ChangedAt = time.Now()
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

		// Trim or split overlapping ranges based on the unmapped subrange.
		newRanges := make([]AddrRange, 0, len(th.Ranges))
		for _, r := range th.Ranges {
			// No overlap: keep as-is.
			if r.StartVal >= endAddr || r.EndVal <= addr {
				newRanges = append(newRanges, r)
				continue
			}

			// Overlap exists. Keep left segment if any: [r.StartVal, addr).
			if r.StartVal < addr {
				left := r
				left.EndVal = addr
				left.End = fmt.Sprintf("%x", left.EndVal)
				left.Size = (left.EndVal - left.StartVal) / PageSizeBytes
				left.PrevSize = left.Size
				left.ChangedAt = time.Now()
				// Page-status counts are no longer exact after partial unmap.
				left.Statuses = make(map[string]*StatusCount)
				newRanges = append(newRanges, left)
			}

			// Keep right segment if any: [endAddr, r.EndVal).
			if r.EndVal > endAddr {
				right := r
				right.StartVal = endAddr
				right.Start = fmt.Sprintf("%x", right.StartVal)
				right.Size = (right.EndVal - right.StartVal) / PageSizeBytes
				right.PrevSize = right.Size
				right.ChangedAt = time.Now()
				// Page-status counts are no longer exact after partial unmap.
				right.Statuses = make(map[string]*StatusCount)
				newRanges = append(newRanges, right)
			}
		}
		SortRanges(newRanges)
		pd.Threads[i].Ranges = newRanges
		// Invalidate cache for this thread
		pd.invalidateRangeCache(tid)
		break
	}
}
