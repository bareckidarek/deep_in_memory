package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogAggregator(t *testing.T) {
	la := NewLogAggregator()
	require.NotNil(t, la)
	assert.Len(t, la.events, 6)
	assert.Empty(t, la.history)
	assert.Equal(t, 30, la.maxLogs)
}

func TestRecord_SingleEvent(t *testing.T) {
	la := NewLogAggregator()
	la.Record("mmap", 0x1000, 100, 101)

	assert.Len(t, la.events["mmap"], 1)
	info := la.events["mmap"][0x1000]
	assert.Equal(t, 1, info.count)
	assert.Equal(t, 101, info.tid)
}

func TestRecord_SameAddressIncrements(t *testing.T) {
	la := NewLogAggregator()
	la.Record("mmap", 0x1000, 100, 101)
	la.Record("mmap", 0x1000, 100, 102)

	assert.Len(t, la.events["mmap"], 1)
	info := la.events["mmap"][0x1000]
	assert.Equal(t, 2, info.count)
	assert.Equal(t, 102, info.tid) // updated to latest TID
}

func TestRecord_DifferentAddresses(t *testing.T) {
	la := NewLogAggregator()
	la.Record("mmap", 0x1000, 100, 101)
	la.Record("mmap", 0x2000, 100, 101)

	assert.Len(t, la.events["mmap"], 2)
}

func TestRecord_UnknownEventType(t *testing.T) {
	la := NewLogAggregator()
	la.Record("unknown", 0x1000, 100, 101)

	// Unknown event type should be silently ignored
	assert.Empty(t, la.events["mmap"])
}

func TestFlush_CreatesHistoryAndClearsEvents(t *testing.T) {
	la := NewLogAggregator()
	la.Record("mmap", 0x1000, 100, 101)
	la.Record("brk", 0x2000, 100, 101)

	la.Flush(100)

	// Events should be cleared
	assert.Empty(t, la.events["mmap"])
	assert.Empty(t, la.events["brk"])

	// History should have 2 entries
	assert.Len(t, la.history, 2)
}

func TestFlush_NoEventNoHistory(t *testing.T) {
	la := NewLogAggregator()
	la.Flush(100)
	assert.Empty(t, la.history)
}

func TestFlush_HistoryNewestFirst(t *testing.T) {
	la := NewLogAggregator()

	la.Record("mmap", 0x1000, 100, 101)
	la.Flush(100)

	la.Record("brk", 0x2000, 100, 101)
	la.Flush(100)

	require.Len(t, la.history, 2)
	assert.Equal(t, "brk", la.history[0].EventType)  // newest
	assert.Equal(t, "mmap", la.history[1].EventType) // oldest
}

func TestFlush_MaxLogsLimit(t *testing.T) {
	la := NewLogAggregator()
	la.maxLogs = 3

	for i := 0; i < 5; i++ {
		la.Record("mmap", uint64(i*0x1000), 100, 101)
		la.Flush(100)
	}

	assert.Len(t, la.history, 3)
}

func TestGetFormattedLogs_Empty(t *testing.T) {
	la := NewLogAggregator()
	assert.Nil(t, la.GetFormattedLogs())
}

func TestGetFormattedLogs_Format(t *testing.T) {
	la := NewLogAggregator()
	la.Record("mmap", 0x1000, 100, 101)
	la.Flush(100)

	logs := la.GetFormattedLogs()
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0], "PID:100")
	assert.Contains(t, logs[0], "TID:101")
	assert.Contains(t, logs[0], "mmap")
	assert.Contains(t, logs[0], "pages: 1")
	assert.Contains(t, logs[0], "0x1000")
}

func TestFormatAddressRanges_Empty(t *testing.T) {
	assert.Equal(t, "", formatAddressRanges(nil))
	assert.Equal(t, "", formatAddressRanges([]uint64{}))
}

func TestFormatAddressRanges_Single(t *testing.T) {
	result := formatAddressRanges([]uint64{0x1000})
	assert.Equal(t, "0x1000", result)
}

func TestFormatAddressRanges_AdjacentPages(t *testing.T) {
	addrs := []uint64{0x1000, 0x2000, 0x3000}
	result := formatAddressRanges(addrs)
	assert.Equal(t, "0x1000 - 0x3000", result)
}

func TestFormatAddressRanges_NonAdjacent(t *testing.T) {
	addrs := []uint64{0x1000, 0x5000}
	result := formatAddressRanges(addrs)
	assert.Equal(t, "0x1000, 0x5000", result)
}

func TestFormatAddressRanges_MixedRanges(t *testing.T) {
	addrs := []uint64{0x1000, 0x2000, 0x3000, 0x8000, 0x9000}
	result := formatAddressRanges(addrs)
	parts := strings.Split(result, ", ")
	require.Len(t, parts, 2)
	assert.Equal(t, "0x1000 - 0x3000", parts[0])
	assert.Equal(t, "0x8000 - 0x9000", parts[1])
}

func TestFormatAddressRanges_UnsortedInput(t *testing.T) {
	addrs := []uint64{0x3000, 0x1000, 0x2000}
	result := formatAddressRanges(addrs)
	assert.Equal(t, "0x1000 - 0x3000", result)
}

func TestSumCounts(t *testing.T) {
	m := map[uint64]*eventInfo{
		0x1000: {count: 3},
		0x2000: {count: 5},
	}
	assert.Equal(t, 8, sumCounts(m))
}

func TestGetAddresses(t *testing.T) {
	m := map[uint64]*eventInfo{
		0x2000: {count: 1},
		0x1000: {count: 1},
	}
	addrs := getAddresses(m)
	require.Len(t, addrs, 2)
	assert.Equal(t, uint64(0x1000), addrs[0]) // sorted
	assert.Equal(t, uint64(0x2000), addrs[1])
}

func TestGetMostRecentTID(t *testing.T) {
	m := map[uint64]*eventInfo{
		0x1000: {count: 1, tid: 42},
	}
	assert.Equal(t, 42, getMostRecentTID(m))
	assert.Equal(t, 0, getMostRecentTID(map[uint64]*eventInfo{}))
}
