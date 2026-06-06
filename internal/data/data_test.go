package data

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ProcessData thread helpers ---

func TestGetOrCreateThread_New(t *testing.T) {
	pd := &ProcessData{Proc: Process{PID: 1}}
	idx := pd.getOrCreateThread(100)

	assert.Equal(t, 0, idx)
	require.Len(t, pd.Threads, 1)
	assert.Equal(t, 100, pd.Threads[0].TID)
}

func TestGetOrCreateThread_Existing(t *testing.T) {
	pd := &ProcessData{
		Proc:    Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{}}},
	}
	idx := pd.getOrCreateThread(100)

	assert.Equal(t, 0, idx)
	assert.Len(t, pd.Threads, 1) // no duplicate created
}

func TestGetOrCreateThread_NilThreads(t *testing.T) {
	pd := &ProcessData{Proc: Process{PID: 1}}
	pd.Threads = nil
	idx := pd.getOrCreateThread(50)

	assert.Equal(t, 0, idx)
	require.Len(t, pd.Threads, 1)
}

// --- AddMmapRange ---

func TestAddMmapRange(t *testing.T) {
	pd := &ProcessData{Proc: Process{PID: 1}}

	pd.AddMmapRange(100, 0x7f0000000000, 0x1000)

	require.Len(t, pd.Threads, 1)
	require.Len(t, pd.Threads[0].Ranges, 1)

	r := pd.Threads[0].Ranges[0]
	assert.Equal(t, uint64(0x7f0000000000), r.StartVal)
	assert.Equal(t, uint64(0x7f0000001000), r.EndVal)
	assert.Equal(t, uint64(1), r.Size) // one 4kB page
}

func TestAddMmapRange_MultipleRanges(t *testing.T) {
	pd := &ProcessData{Proc: Process{PID: 1}}

	pd.AddMmapRange(100, 0x2000, 0x1000)
	pd.AddMmapRange(100, 0x1000, 0x1000)

	require.Len(t, pd.Threads[0].Ranges, 2)
	// Ranges should be sorted by address
	assert.Equal(t, uint64(0x1000), pd.Threads[0].Ranges[0].StartVal)
	assert.Equal(t, uint64(0x2000), pd.Threads[0].Ranges[1].StartVal)
}

// --- ExtendBrkRange ---

func TestExtendBrkRange_Query(t *testing.T) {
	pd := &ProcessData{Proc: Process{PID: 1}}

	// brk==0 is a query — should create initial heap range
	pd.ExtendBrkRange(100, 0, 0x5000)

	require.Len(t, pd.Threads, 1)
	require.Len(t, pd.Threads[0].Ranges, 1)
	assert.Equal(t, "[heap]", pd.Threads[0].Ranges[0].Label)
	assert.Equal(t, uint64(0x5000), pd.Threads[0].Ranges[0].StartVal)
}

func TestExtendBrkRange_Grow(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{{
			Start: "5000", End: "5000",
			StartVal: 0x5000, EndVal: 0x5000,
			Size: 0, Statuses: make(map[string]*StatusCount),
			Label: "[heap]",
		}}}},
	}

	pd.ExtendBrkRange(100, 0x5000, 0x8000)

	r := pd.Threads[0].Ranges[0]
	assert.Equal(t, uint64(0x8000), r.EndVal)
	expectedPages := (uint64(0x8000) - uint64(0x5000)) / PageSizeBytes
	assert.Equal(t, expectedPages, r.Size)
}

func TestExtendBrkRange_Shrink(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{{
			Start: "5000", End: "8000",
			StartVal: 0x5000, EndVal: 0x8000,
			Size: 3, Statuses: make(map[string]*StatusCount),
			Label: "[heap]",
		}}}},
	}

	pd.ExtendBrkRange(100, 0x8000, 0x6000)

	r := pd.Threads[0].Ranges[0]
	assert.Equal(t, uint64(0x6000), r.EndVal)
}

// --- RemoveMmapRange ---

func TestRemoveMmapRange(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{
			{StartVal: 0x1000, EndVal: 0x2000, Size: 1},
			{StartVal: 0x3000, EndVal: 0x4000, Size: 1},
		}}},
	}

	pd.RemoveMmapRange(100, 0x1000, 0x1000)

	require.Len(t, pd.Threads[0].Ranges, 1)
	assert.Equal(t, uint64(0x3000), pd.Threads[0].Ranges[0].StartVal)
}

func TestRemoveMmapRange_NoMatch(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{
			{StartVal: 0x1000, EndVal: 0x2000, Size: 1},
		}}},
	}

	pd.RemoveMmapRange(100, 0x5000, 0x1000) // non-overlapping
	assert.Len(t, pd.Threads[0].Ranges, 1)  // unchanged
}

func TestRemoveMmapRange_NilThreads(t *testing.T) {
	pd := &ProcessData{Proc: Process{PID: 1}}
	pd.RemoveMmapRange(100, 0x1000, 0x1000) // should not panic
}

// --- FindRangeForAddr ---

func TestFindRangeForAddr_Found(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{
			{StartVal: 0x1000, EndVal: 0x2000},
			{StartVal: 0x3000, EndVal: 0x5000},
		}}},
	}

	r := pd.FindRangeForAddr(100, 0x3500)
	require.NotNil(t, r)
	assert.Equal(t, uint64(0x3000), r.StartVal)
}

func TestFindRangeForAddr_NotFound(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{
			{StartVal: 0x1000, EndVal: 0x2000},
		}}},
	}

	assert.Nil(t, pd.FindRangeForAddr(100, 0x5000))
}

func TestFindRangeForAddr_ExactStart(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{
			{StartVal: 0x1000, EndVal: 0x2000},
		}}},
	}

	r := pd.FindRangeForAddr(100, 0x1000)
	require.NotNil(t, r)
	assert.Equal(t, uint64(0x1000), r.StartVal)
}

func TestFindRangeForAddr_WrongThread(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{
			{StartVal: 0x1000, EndVal: 0x2000},
		}}},
	}

	assert.Nil(t, pd.FindRangeForAddr(999, 0x1000))
}

// --- AddPageFault ---

func TestAddPageFault(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{
			{
				StartVal: 0x1000, EndVal: 0x5000,
				Statuses: make(map[string]*StatusCount),
			},
		}}},
	}

	pd.AddPageFault(100, 0x2000)

	r := pd.Threads[0].Ranges[0]
	require.NotNil(t, r.Statuses["INIT"])
	assert.Equal(t, 1, r.Statuses["INIT"].Count)
}

func TestAddPageFault_NilThreads(t *testing.T) {
	pd := &ProcessData{Proc: Process{PID: 1}}
	pd.AddPageFault(100, 0x2000) // should not panic
}

// --- AddSwapStatus ---

func TestAddSwapStatus_Swap(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{
			{
				StartVal: 0x1000, EndVal: 0x5000,
				Statuses: make(map[string]*StatusCount),
			},
		}}},
	}

	pd.AddSwapStatus(100, 0x2000, true)

	r := pd.Threads[0].Ranges[0]
	require.NotNil(t, r.Statuses["SWAP"])
	assert.Equal(t, 1, r.Statuses["SWAP"].Count)
}

func TestAddSwapStatus_Unswap(t *testing.T) {
	pd := &ProcessData{
		Proc: Process{PID: 1},
		Threads: []ThreadData{{TID: 100, Ranges: []AddrRange{
			{
				StartVal: 0x1000, EndVal: 0x5000,
				Statuses: make(map[string]*StatusCount),
			},
		}}},
	}

	pd.AddSwapStatus(100, 0x2000, false)

	r := pd.Threads[0].Ranges[0]
	require.NotNil(t, r.Statuses["UNSWAP"])
	assert.Equal(t, 1, r.Statuses["UNSWAP"].Count)
}

// --- SortRanges ---

func TestSortRanges_HeapFirst(t *testing.T) {
	ranges := []AddrRange{
		{StartVal: 0x5000, Label: ""},
		{StartVal: 0x1000, Label: "[heap]"},
	}

	SortRanges(ranges)

	assert.Equal(t, "[heap]", ranges[0].Label)
}

// func TestSortRanges_FilePathsLast(t *testing.T) {
// 	ranges := []AddrRange{
// 		{StartVal: 0x1000, Label: "/usr/lib/libc.so"},
// 		{StartVal: 0x2000, Label: ""},
// 		{StartVal: 0x3000, Label: "[stack]"},
// 	}

// 	SortRanges(ranges)

// 	// anonymous (empty) first, then labeled, then file path
// 	assert.Equal(t, "", ranges[0].Label)
// 	assert.Equal(t, "[stack]", ranges[1].Label)
// 	assert.Equal(t, "/usr/lib/libc.so", ranges[2].Label)
// }

func TestSortRanges_SameCategoryByAddress(t *testing.T) {
	ranges := []AddrRange{
		{StartVal: 0x5000, Label: ""},
		{StartVal: 0x1000, Label: ""},
		{StartVal: 0x3000, Label: ""},
	}

	SortRanges(ranges)

	assert.Equal(t, uint64(0x1000), ranges[0].StartVal)
	assert.Equal(t, uint64(0x3000), ranges[1].StartVal)
	assert.Equal(t, uint64(0x5000), ranges[2].StartVal)
}

// --- BuildDataset ---

func TestBuildDataset_Empty(t *testing.T) {
	ds := BuildDataset(nil)
	assert.Empty(t, ds.Processes)
	assert.Empty(t, ds.Order)
}

func TestBuildDataset_Basic(t *testing.T) {
	procs := []ProcessData{
		{Proc: Process{PID: 1, Name: "a", StartTime: time.Unix(200, 0)}},
		{Proc: Process{PID: 2, Name: "b", StartTime: time.Unix(100, 0)}},
	}

	ds := BuildDataset(procs)

	assert.Len(t, ds.Processes, 2)
	require.Len(t, ds.Order, 2)
	// Ordered by start time
	assert.Equal(t, 2, ds.Order[0]) // PID 2 started first
	assert.Equal(t, 1, ds.Order[1])
}

func TestBuildDataset_SameStartTime_OrderByPID(t *testing.T) {
	ts := time.Unix(100, 0)
	procs := []ProcessData{
		{Proc: Process{PID: 5, Name: "e", StartTime: ts}},
		{Proc: Process{PID: 2, Name: "b", StartTime: ts}},
	}

	ds := BuildDataset(procs)

	assert.Equal(t, 2, ds.Order[0]) // lower PID first
	assert.Equal(t, 5, ds.Order[1])
}

// --- AddressData ---

func TestAddressData_AddPageOffset(t *testing.T) {
	ad := &AddressData{PID: 100}

	ad.AddPageOffset(100, 0x1000, 5, "INIT")

	require.Len(t, ad.Threads, 1)
	require.Len(t, ad.Threads[0].Ranges, 1)
	require.NotNil(t, ad.Threads[0].Ranges[0].Statuses["INIT"])
	assert.Contains(t, ad.Threads[0].Ranges[0].Statuses["INIT"].Value, uint64(5))
}

func TestAddressData_AddPageOffset_NoDuplicates(t *testing.T) {
	ad := &AddressData{PID: 100}

	ad.AddPageOffset(100, 0x1000, 5, "INIT")
	ad.AddPageOffset(100, 0x1000, 5, "INIT") // duplicate

	assert.Len(t, ad.Threads[0].Ranges[0].Statuses["INIT"].Value, 1)
}

func TestAddressData_AddPageOffset_StatusTransition(t *testing.T) {
	ad := &AddressData{PID: 100}

	// Page is first INIT, then swapped
	ad.AddPageOffset(100, 0x1000, 5, "INIT")
	ad.AddPageOffset(100, 0x1000, 5, "SWAP")

	// Should be in SWAP only, removed from INIT
	assert.Empty(t, ad.Threads[0].Ranges[0].Statuses["INIT"].Value)
	assert.Contains(t, ad.Threads[0].Ranges[0].Statuses["SWAP"].Value, uint64(5))
}

func TestAddressData_GetStatusCounts(t *testing.T) {
	ad := &AddressData{PID: 100}
	ad.AddPageOffset(100, 0x1000, 1, "INIT")
	ad.AddPageOffset(100, 0x1000, 2, "INIT")
	ad.AddPageOffset(100, 0x1000, 3, "SWAP")

	counts := ad.GetStatusCounts(100, 0x1000)
	assert.Equal(t, 2, counts["INIT"])
	assert.Equal(t, 1, counts["SWAP"])
}

func TestAddressData_GetStatusCounts_NoMatch(t *testing.T) {
	ad := &AddressData{PID: 100}
	counts := ad.GetStatusCounts(100, 0x9999)
	assert.Empty(t, counts)
}

func TestAddressData_MergeInto(t *testing.T) {
	src := &AddressData{PID: 100, Threads: []ThreadAddressData{
		{TID: 100, Ranges: []RangeAddressData{
			{StartVal: 0x1000, Statuses: map[string]*StatusOffsets{
				"INIT": {Value: []uint64{1, 2, 3}},
			}},
		}},
	}}

	target := &AddressData{PID: 100}
	src.MergeInto(target)

	require.Len(t, target.Threads, 1)
	assert.Len(t, target.Threads[0].Ranges[0].Statuses["INIT"].Value, 3)
}

func TestAddressData_MergeInto_NilSafety(t *testing.T) {
	var src *AddressData
	target := &AddressData{PID: 100}

	// Should not panic
	src.MergeInto(target)
}

// --- parseAddress ---

func TestParseAddress(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
		hasError bool
	}{
		{"1000", 0x1000, false},
		{"0x1000", 0x1000, false},
		{"7fffffff", 0x7fffffff, false},
		{"", 0, true},
		{"xyz", 0, true},
	}

	for _, tc := range tests {
		result, err := parseAddress(tc.input)
		if tc.hasError {
			assert.Error(t, err, "input: %s", tc.input)
		} else {
			assert.NoError(t, err, "input: %s", tc.input)
			assert.Equal(t, tc.expected, result, "input: %s", tc.input)
		}
	}
}

// --- parseTime ---

func TestParseTime(t *testing.T) {
	ts := parseTime("2024-01-15T12:00:00Z")
	assert.False(t, ts.IsZero())
	assert.Equal(t, 2024, ts.Year())

	assert.True(t, parseTime("").IsZero())
	assert.True(t, parseTime("invalid").IsZero())
}

// --- addPageToStatus / removePageFromStatus ---

func TestAddPageToStatus(t *testing.T) {
	r := &AddrRange{Statuses: make(map[string]*StatusCount)}

	r.addPageToStatus(0, "INIT")
	assert.Equal(t, 1, r.Statuses["INIT"].Count)

	// Adding to different status should decrement previous
	r.addPageToStatus(0, "SWAP")
	assert.Equal(t, 0, r.Statuses["INIT"].Count)
	assert.Equal(t, 1, r.Statuses["SWAP"].Count)
}

func TestAddPageToStatus_NilStatuses(t *testing.T) {
	r := &AddrRange{}
	r.addPageToStatus(0, "INIT") // should not panic
	assert.Equal(t, 1, r.Statuses["INIT"].Count)
}

func TestRemovePageFromStatus(t *testing.T) {
	r := &AddrRange{
		Statuses: map[string]*StatusCount{
			"INIT": {Count: 3},
		},
	}

	r.removePageFromStatus("INIT")
	assert.Equal(t, 2, r.Statuses["INIT"].Count)
}

func TestRemovePageFromStatus_FloorAtZero(t *testing.T) {
	r := &AddrRange{
		Statuses: map[string]*StatusCount{
			"INIT": {Count: 0},
		},
	}

	r.removePageFromStatus("INIT")
	assert.Equal(t, 0, r.Statuses["INIT"].Count)
}

func TestRemovePageFromStatus_NilStatuses(t *testing.T) {
	r := &AddrRange{}
	r.removePageFromStatus("INIT") // should not panic
}

// --- getThread ---

func TestGetThread_Found(t *testing.T) {
	pd := &ProcessData{
		Threads: []ThreadData{
			{TID: 100},
			{TID: 200},
		},
	}

	th := pd.getThread(200)
	require.NotNil(t, th)
	assert.Equal(t, 200, th.TID)
}

func TestGetThread_NotFound(t *testing.T) {
	pd := &ProcessData{
		Threads: []ThreadData{{TID: 100}},
	}

	assert.Nil(t, pd.getThread(999))
}
