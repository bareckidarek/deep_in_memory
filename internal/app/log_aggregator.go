package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// LogEntry represents a single aggregated log entry with timestamp
type LogEntry struct {
	Timestamp time.Time
	EventType string
	Count     int
	Addresses []uint64
	PID       int
	TID       int
}

// LogAggregator aggregates eBPF events for the selected process
type LogAggregator struct {
	// Map of event type -> (address -> event info with count and TID)
	events  map[string]map[uint64]*eventInfo
	history []LogEntry // Last N log entries (newest first)
	maxLogs int
}

type eventInfo struct {
	count int
	tid   int // Last TID that triggered this event
}

// NewLogAggregator creates a new log aggregator
func NewLogAggregator() *LogAggregator {
	return &LogAggregator{
		events: map[string]map[uint64]*eventInfo{
			"brk":        make(map[uint64]*eventInfo),
			"mmap":       make(map[uint64]*eventInfo),
			"munmap":     make(map[uint64]*eventInfo),
			"page_fault": make(map[uint64]*eventInfo),
			"swap_write": make(map[uint64]*eventInfo),
			"swap_read":  make(map[uint64]*eventInfo),
		},
		history: make([]LogEntry, 0, 30),
		maxLogs: 30,
	}
}

// Record adds an event to the aggregator for the given PID and TID
func (la *LogAggregator) Record(eventType string, addr uint64, pid, tid int) {
	if eventMap, ok := la.events[eventType]; ok {
		if info, exists := eventMap[addr]; exists {
			info.count++
			info.tid = tid // Update to latest TID
		} else {
			eventMap[addr] = &eventInfo{count: 1, tid: tid}
		}
	}
}

// Flush creates log entries and adds them to history, then clears aggregators
func (la *LogAggregator) Flush(pid int) {
	now := time.Now()

	// Process all event types
	for eventType, addrMap := range la.events {
		if len(addrMap) > 0 {
			la.addToHistory(LogEntry{
				Timestamp: now,
				EventType: eventType,
				Count:     sumCounts(addrMap),
				Addresses: getAddresses(addrMap),
				PID:       pid,
				TID:       getMostRecentTID(addrMap),
			})
			// Clear the map for this event type
			la.events[eventType] = make(map[uint64]*eventInfo)
		}
	}
}

// addToHistory adds a log entry to history, maintaining max size (newest first)
func (la *LogAggregator) addToHistory(entry LogEntry) {
	// Prepend to keep newest at top
	la.history = append([]LogEntry{entry}, la.history...)

	// Trim to max size
	if len(la.history) > la.maxLogs {
		la.history = la.history[:la.maxLogs]
	}
}

// GetFormattedLogs returns formatted log lines (newest first)
func (la *LogAggregator) GetFormattedLogs() []string {
	if len(la.history) == 0 {
		return nil
	}

	lines := make([]string, len(la.history))
	for i, entry := range la.history {
		timestamp := entry.Timestamp.Format("2006-01-02 15:04:05")
		addrStr := formatAddressRanges(entry.Addresses)
		lines[i] = fmt.Sprintf("%s | PID:%d, TID:%d | %s -> count: %d, addresses: %s",
			timestamp, entry.PID, entry.TID, entry.EventType, entry.Count, addrStr)
	}
	return lines
}

func sumCounts(m map[uint64]*eventInfo) int {
	total := 0
	for _, info := range m {
		total += info.count
	}
	return total
}

func getAddresses(m map[uint64]*eventInfo) []uint64 {
	addrs := make([]uint64, 0, len(m))
	for addr := range m {
		addrs = append(addrs, addr)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })
	return addrs
}

func getMostRecentTID(m map[uint64]*eventInfo) int {
	// Return the TID from any event (they should all be from the same process)
	// If events come from multiple threads, return the last one we saw
	for _, info := range m {
		return info.tid
	}
	return 0
}

// formatAddressRanges converts addresses into ranges when they are adjacent
func formatAddressRanges(addrs []uint64) string {
	if len(addrs) == 0 {
		return ""
	}

	// Sort addresses
	sorted := make([]uint64, len(addrs))
	copy(sorted, addrs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	var result []string
	rangeStart := sorted[0]
	rangeEnd := sorted[0]
	// Page size - addresses are grouped into ranges only if they differ by exactly 4kB (one page)
	const pageSize = uint64(0x1000) // 4kB

	for i := 1; i < len(sorted); i++ {
		// Check if addresses are exactly one page apart
		if sorted[i]-rangeEnd == pageSize {
			rangeEnd = sorted[i]
		} else {
			// Add range to result
			if rangeStart == rangeEnd {
				result = append(result, fmt.Sprintf("0x%x", rangeStart))
			} else {
				result = append(result, fmt.Sprintf("0x%x - 0x%x", rangeStart, rangeEnd))
			}
			rangeStart = sorted[i]
			rangeEnd = sorted[i]
		}
	}

	// Add final range
	if rangeStart == rangeEnd {
		result = append(result, fmt.Sprintf("0x%x", rangeStart))
	} else {
		result = append(result, fmt.Sprintf("0x%x - 0x%x", rangeStart, rangeEnd))
	}

	return strings.Join(result, ", ")
}
