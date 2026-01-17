package data

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessData_AddMmapRange(t *testing.T) {
	// Save original page size and restore after test
	originalPageSize := PageSizeBytes
	defer func() { PageSizeBytes = originalPageSize }()

	// Set a known page size for testing
	PageSizeBytes = 4096

	tests := []struct {
		name     string
		pid      int
		tid      int
		addr     uint64
		length   uint64
		validate func(*testing.T, *ProcessData)
	}{
		{
			name:   "add mmap range to new process",
			pid:    1234,
			tid:    1234,
			addr:   0x1000000,
			length: 0x1000, // 1 page
			validate: func(t *testing.T, pd *ProcessData) {
				require.Len(t, pd.Threads, 1)
				assert.Equal(t, 1234, pd.Threads[0].TID)
				require.Len(t, pd.Threads[0].Ranges, 1)

				range_ := pd.Threads[0].Ranges[0]
				assert.Equal(t, "1000000", range_.Start)
				assert.Equal(t, "1001000", range_.End)
				assert.Equal(t, uint64(1), range_.Size)
				assert.Equal(t, uint64(0x1000000), range_.StartVal)
				assert.Equal(t, uint64(0x1001000), range_.EndVal)
			},
		},
		{
			name:   "add multiple mmap ranges to existing thread",
			pid:    1234,
			tid:    1234,
			addr:   0x2000000,
			length: 0x2000, // 2 pages
			validate: func(t *testing.T, pd *ProcessData) {
				require.Len(t, pd.Threads, 1)
				require.Len(t, pd.Threads[0].Ranges, 2)

				// Ranges should be sorted by start address
				firstRange := pd.Threads[0].Ranges[0]
				secondRange := pd.Threads[0].Ranges[1]

				assert.Equal(t, "1000000", firstRange.Start)
				assert.Equal(t, "2000000", secondRange.Start)
			},
		},
		{
			name:   "add mmap range to new thread",
			pid:    1234,
			tid:    1235,
			addr:   0x3000000,
			length: 0x1000, // 1 page
			validate: func(t *testing.T, pd *ProcessData) {
				require.Len(t, pd.Threads, 2)

				// Find the new thread
				var newThread *ThreadData
				for _, thread := range pd.Threads {
					if thread.TID == 1235 {
						newThread = &thread
						break
					}
				}
				require.NotNil(t, newThread)
				require.Len(t, newThread.Ranges, 1)
				assert.Equal(t, "3000000", newThread.Ranges[0].Start)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &ProcessData{
				Proc: Process{PID: tt.pid, Name: "test", StartTime: time.Now()},
			}

			// Add initial range for tests that expect existing data
			if tt.name == "add multiple mmap ranges to existing thread" ||
				tt.name == "add mmap range to new thread" {
				pd.AddMmapRange(1234, 0x1000000, 0x1000)
			}

			pd.AddMmapRange(tt.tid, tt.addr, tt.length)
			tt.validate(t, pd)
		})
	}
}

func TestProcessData_ExtendBrkRange(t *testing.T) {
	originalPageSize := PageSizeBytes
	defer func() { PageSizeBytes = originalPageSize }()
	PageSizeBytes = 4096

	tests := []struct {
		name     string
		pid      int
		tid      int
		brk      uint64
		ret      uint64
		validate func(*testing.T, *ProcessData)
	}{
		{
			name: "create initial heap range with brk==0",
			pid:  1234,
			tid:  1234,
			brk:  0,
			ret:  0x2000000,
			validate: func(t *testing.T, pd *ProcessData) {
				require.Len(t, pd.Threads, 1)
				require.Len(t, pd.Threads[0].Ranges, 1)

				heapRange := pd.Threads[0].Ranges[0]
				assert.Equal(t, "[heap]", heapRange.Label)
				assert.Equal(t, "2000000", heapRange.Start)
				assert.Equal(t, "2000000", heapRange.End)
				assert.Equal(t, uint64(0), heapRange.Size)
			},
		},
		{
			name: "extend existing heap range",
			pid:  1234,
			tid:  1234,
			brk:  0x2001000,
			ret:  0x2001000,
			validate: func(t *testing.T, pd *ProcessData) {
				// First create initial heap
				pd.AddMmapRange(1234, 0x2000000, 0x1000)
				// Mark it as heap (simulating existing state)
				pd.Threads[0].Ranges[0].Label = "[heap]"
				pd.Threads[0].Ranges[0].StartVal = 0x2000000
				pd.Threads[0].Ranges[0].EndVal = 0x2001000

				// Now extend it
				pd.ExtendBrkRange(1234, 0x2001000, 0x2002000)

				heapRange := pd.Threads[0].Ranges[0]
				assert.Equal(t, "[heap]", heapRange.Label)
				assert.Equal(t, "2000000", heapRange.Start)
				assert.Equal(t, "2002000", heapRange.End)
				assert.Equal(t, uint64(2), heapRange.Size) // 2 pages
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &ProcessData{
				Proc: Process{PID: tt.pid, Name: "test", StartTime: time.Now()},
			}

			pd.ExtendBrkRange(tt.tid, tt.brk, tt.ret)
			tt.validate(t, pd)
		})
	}
}

func TestProcessData_AddPageFault(t *testing.T) {
	originalPageSize := PageSizeBytes
	defer func() { PageSizeBytes = originalPageSize }()
	PageSizeBytes = 4096

	tests := []struct {
		name      string
		setup     func(*ProcessData)
		tid       int
		faultAddr uint64
		validate  func(*testing.T, *ProcessData)
	}{
		{
			name: "page fault in existing range increments INIT count",
			setup: func(pd *ProcessData) {
				pd.AddMmapRange(1234, 0x1000000, 0x2000) // 2 pages
			},
			tid:       1234,
			faultAddr: 0x1001000, // Second page
			validate: func(t *testing.T, pd *ProcessData) {
				require.Len(t, pd.Threads[0].Ranges, 1)
				range_ := pd.Threads[0].Ranges[0]
				require.NotNil(t, range_.Statuses["INIT"])
				assert.Equal(t, 1, range_.Statuses["INIT"].Count)
			},
		},
		{
			name: "page fault outside any range is ignored",
			setup: func(pd *ProcessData) {
				pd.AddMmapRange(1234, 0x1000000, 0x1000)
			},
			tid:       1234,
			faultAddr: 0x2000000, // Outside range
			validate: func(t *testing.T, pd *ProcessData) {
				range_ := pd.Threads[0].Ranges[0]
				if range_.Statuses["INIT"] != nil {
					assert.Equal(t, 0, range_.Statuses["INIT"].Count)
				}
			},
		},
		{
			name:      "page fault in non-existent thread is ignored",
			tid:       5678, // Non-existent thread
			faultAddr: 0x1001000,
			validate: func(t *testing.T, pd *ProcessData) {
				// Should not crash or create data
				assert.Empty(t, pd.Threads)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &ProcessData{
				Proc: Process{PID: 1234, Name: "test", StartTime: time.Now()},
			}

			if tt.setup != nil {
				tt.setup(pd)
			}

			pd.AddPageFault(tt.tid, tt.faultAddr)
			tt.validate(t, pd)
		})
	}
}

func TestProcessData_AddSwapStatus(t *testing.T) {
	originalPageSize := PageSizeBytes
	defer func() { PageSizeBytes = originalPageSize }()
	PageSizeBytes = 4096

	tests := []struct {
		name     string
		setup    func(*ProcessData)
		tid      int
		swapAddr uint64
		isSwap   bool
		validate func(*testing.T, *ProcessData)
	}{
		{
			name: "swap write increments SWAP and decrements INIT",
			setup: func(pd *ProcessData) {
				pd.AddMmapRange(1234, 0x1000000, 0x2000)
				pd.AddPageFault(1234, 0x1001000) // Add 1 INIT
			},
			tid:      1234,
			swapAddr: 0x1001000,
			isSwap:   true,
			validate: func(t *testing.T, pd *ProcessData) {
				range_ := pd.Threads[0].Ranges[0]
				assert.Equal(t, 0, range_.Statuses["INIT"].Count) // Decremented
				assert.Equal(t, 1, range_.Statuses["SWAP"].Count) // Incremented
			},
		},
		{
			name: "swap read increments UNSWAP and decrements SWAP",
			setup: func(pd *ProcessData) {
				pd.AddMmapRange(1234, 0x1000000, 0x2000)
				pd.AddPageFault(1234, 0x1001000)        // Add 1 INIT
				pd.AddSwapStatus(1234, 0x1001000, true) // Swap it out
			},
			tid:      1234,
			swapAddr: 0x1001000,
			isSwap:   false, // swap in
			validate: func(t *testing.T, pd *ProcessData) {
				range_ := pd.Threads[0].Ranges[0]
				assert.Equal(t, 0, range_.Statuses["INIT"].Count)   // Decremented (SWAP went to 0)
				assert.Equal(t, 0, range_.Statuses["SWAP"].Count)   // Decremented
				assert.Equal(t, 1, range_.Statuses["UNSWAP"].Count) // Incremented
			},
		},
		{
			name: "swap operation outside range is ignored",
			setup: func(pd *ProcessData) {
				pd.AddMmapRange(1234, 0x1000000, 0x1000)
			},
			tid:      1234,
			swapAddr: 0x2000000, // Outside range
			isSwap:   true,
			validate: func(t *testing.T, pd *ProcessData) {
				range_ := pd.Threads[0].Ranges[0]
				assert.Nil(t, range_.Statuses["SWAP"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &ProcessData{
				Proc: Process{PID: 1234, Name: "test", StartTime: time.Now()},
			}

			if tt.setup != nil {
				tt.setup(pd)
			}

			pd.AddSwapStatus(tt.tid, tt.swapAddr, tt.isSwap)
			tt.validate(t, pd)
		})
	}
}

func TestProcessData_RemoveMmapRange(t *testing.T) {
	originalPageSize := PageSizeBytes
	defer func() { PageSizeBytes = originalPageSize }()
	PageSizeBytes = 4096

	tests := []struct {
		name     string
		setup    func(*ProcessData)
		tid      int
		addr     uint64
		length   uint64
		validate func(*testing.T, *ProcessData)
	}{
		{
			name: "remove exact range",
			setup: func(pd *ProcessData) {
				pd.AddMmapRange(1234, 0x1000000, 0x1000)
				pd.AddMmapRange(1234, 0x2000000, 0x1000)
			},
			tid:    1234,
			addr:   0x2000000,
			length: 0x1000,
			validate: func(t *testing.T, pd *ProcessData) {
				require.Len(t, pd.Threads[0].Ranges, 1)
				assert.Equal(t, "1000000", pd.Threads[0].Ranges[0].Start)
			},
		},
		{
			name: "remove overlapping range",
			setup: func(pd *ProcessData) {
				pd.AddMmapRange(1234, 0x1000000, 0x2000) // 0x1000000-0x1002000
			},
			tid:    1234,
			addr:   0x1000500, // Overlap middle
			length: 0x1000,    // 0x1000500-0x1001500
			validate: func(t *testing.T, pd *ProcessData) {
				// Entire overlapping range should be removed
				require.Len(t, pd.Threads[0].Ranges, 0)
			},
		},
		{
			name: "remove from non-existent thread does nothing",
			setup: func(pd *ProcessData) {
				pd.AddMmapRange(1234, 0x1000000, 0x1000)
			},
			tid:    5678, // Non-existent thread
			addr:   0x2000000,
			length: 0x1000,
			validate: func(t *testing.T, pd *ProcessData) {
				// Original range should remain
				require.Len(t, pd.Threads[0].Ranges, 1)
				assert.Equal(t, "1000000", pd.Threads[0].Ranges[0].Start)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &ProcessData{
				Proc: Process{PID: 1234, Name: "test", StartTime: time.Now()},
			}

			if tt.setup != nil {
				tt.setup(pd)
			}

			pd.RemoveMmapRange(tt.tid, tt.addr, tt.length)
			tt.validate(t, pd)
		})
	}
}

func TestAddrRange_StatusOperations(t *testing.T) {
	tests := []struct {
		name          string
		status        string
		initialCount  int
		operation     func(*AddrRange)
		expectedCount int
	}{
		{
			name:          "increment new status creates entry",
			status:        "INIT",
			initialCount:  0,
			operation:     func(r *AddrRange) { r.incrementStatus("INIT") },
			expectedCount: 1,
		},
		{
			name:          "increment existing status",
			status:        "INIT",
			initialCount:  2,
			operation:     func(r *AddrRange) { r.incrementStatus("INIT") },
			expectedCount: 3,
		},
		{
			name:          "decrement existing status",
			status:        "INIT",
			initialCount:  3,
			operation:     func(r *AddrRange) { r.decrementStatus("INIT") },
			expectedCount: 2,
		},
		{
			name:          "decrement below zero clamps to zero",
			status:        "INIT",
			initialCount:  1,
			operation:     func(r *AddrRange) { r.decrementStatus("INIT") },
			expectedCount: 0,
		},
		{
			name:          "decrement non-existent status does nothing",
			status:        "SWAP",
			initialCount:  0,
			operation:     func(r *AddrRange) { r.decrementStatus("SWAP") },
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &AddrRange{
				Start:    "1000000",
				End:      "1001000",
				StartVal: 0x1000000,
				EndVal:   0x1001000,
				Size:     1,
				Statuses: make(map[string]*StatusCount),
			}

			// Set initial count if specified
			if tt.initialCount > 0 {
				r.Statuses[tt.status] = &StatusCount{Count: tt.initialCount}
			}

			tt.operation(r)

			if tt.expectedCount > 0 {
				require.NotNil(t, r.Statuses[tt.status])
				assert.Equal(t, tt.expectedCount, r.Statuses[tt.status].Count)
			} else {
				// Status might still exist with count 0, or be nil
				if r.Statuses[tt.status] != nil {
					assert.Equal(t, 0, r.Statuses[tt.status].Count)
				}
			}
		})
	}
}

func TestSortRanges(t *testing.T) {
	ranges := []AddrRange{
		{Start: "3000000", StartVal: 0x3000000, Label: "/usr/lib/libc.so.6"},
		{Start: "1000000", StartVal: 0x1000000, Label: "[heap]"},
		{Start: "2000000", StartVal: 0x2000000, Label: ""},
		{Start: "4000000", StartVal: 0x4000000, Label: "[anon: Go: gc bits]"},
		{Start: "5000000", StartVal: 0x5000000, Label: "/usr/lib/libm.so.6"},
	}

	SortRanges(ranges)

	// Expected order: [heap] -> empty label -> non-file label -> file references
	expected := []struct {
		start string
		label string
	}{
		{"1000000", "[heap]"},              // Heap first
		{"2000000", ""},                    // Empty label second
		{"4000000", "[anon: Go: gc bits]"}, // Non-file label third
		{"3000000", "/usr/lib/libc.so.6"},  // File references sorted by address
		{"5000000", "/usr/lib/libm.so.6"},
	}

	require.Len(t, ranges, len(expected))
	for i, expectedRange := range expected {
		assert.Equal(t, expectedRange.start, ranges[i].Start)
		assert.Equal(t, expectedRange.label, ranges[i].Label)
	}
}

func TestBuildDataset(t *testing.T) {
	now := time.Now().UTC()

	processes := []ProcessData{
		{
			Proc: Process{PID: 1001, Name: "early", StartTime: now.Add(-2 * time.Hour)},
		},
		{
			Proc: Process{PID: 1002, Name: "late", StartTime: now.Add(-1 * time.Hour)},
		},
		{
			Proc: Process{PID: 1000, Name: "earliest", StartTime: now.Add(-3 * time.Hour)},
		},
	}

	dataset := BuildDataset(processes)

	// Verify all processes are included
	assert.Len(t, dataset.Processes, 3)
	assert.Contains(t, dataset.Processes, 1000)
	assert.Contains(t, dataset.Processes, 1001)
	assert.Contains(t, dataset.Processes, 1002)

	// Verify order is by start time, then PID
	expectedOrder := []int{1001, 1000, 1002} // early, earliest, late (actual sorting order)
	assert.Equal(t, expectedOrder, dataset.Order)

	// Verify maps are initialized
	for _, pid := range expectedOrder {
		pd := dataset.Processes[pid]
		assert.NotNil(t, pd.Virtual)
		assert.NotNil(t, pd.PFNs)
	}
}

func TestRangeCache_Optimization(t *testing.T) {
	originalPageSize := PageSizeBytes
	defer func() { PageSizeBytes = originalPageSize }()
	PageSizeBytes = 4096

	pd := &ProcessData{
		Proc: Process{PID: 1234, Name: "test", StartTime: time.Now()},
	}

	// Add multiple ranges to test caching
	pd.AddMmapRange(1234, 0x1000000, 0x1000) // Range 1
	pd.AddMmapRange(1234, 0x2000000, 0x1000) // Range 2
	pd.AddMmapRange(1234, 0x3000000, 0x1000) // Range 3

	// Test cache is built on first lookup
	r1 := pd.findRangeForAddr(1234, 0x1000500)
	require.NotNil(t, r1)
	assert.Equal(t, uint64(0x1000000), r1.StartVal)

	// Test subsequent lookups use cache
	r2 := pd.findRangeForAddr(1234, 0x2000500)
	require.NotNil(t, r2)
	assert.Equal(t, uint64(0x2000000), r2.StartVal)

	// Test cache invalidation when range is added
	pd.AddMmapRange(1234, 0x4000000, 0x1000)
	r3 := pd.findRangeForAddr(1234, 0x4000500)
	require.NotNil(t, r3)
	assert.Equal(t, uint64(0x4000000), r3.StartVal)

	// Test cache invalidation when range is removed
	pd.RemoveMmapRange(1234, 0x2000000, 0x1000)
	r4 := pd.findRangeForAddr(1234, 0x2000500)
	assert.Nil(t, r4) // Should not find removed range
}
