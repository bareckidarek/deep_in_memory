package data

import (
	"time"

	"deep_in_memory/internal/constants"
)

// RangeCache provides fast lookup of address ranges for page faults and swap events
type RangeCache struct {
	Ranges     []*AddrRange // Pointers to ranges for fast iteration
	StartVals  []uint64     // Corresponding start addresses for binary search
	LastUpdate time.Time    // When cache was last updated
}

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
	cache.Ranges = make([]*AddrRange, len(thread.Ranges))
	cache.StartVals = make([]uint64, len(thread.Ranges))

	for i := range thread.Ranges {
		cache.Ranges[i] = &thread.Ranges[i]
		cache.StartVals[i] = thread.Ranges[i].StartVal
	}

	cache.LastUpdate = time.Now()
}

// FindRangeForAddr efficiently locates the virtual address range containing a specific address.
// Uses a cached binary search for O(log n) lookup performance, rebuilding the cache
// if it's stale or doesn't exist. Returns nil if no range contains the address.
func (pd *ProcessData) FindRangeForAddr(tid int, addr uint64) *AddrRange {
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
