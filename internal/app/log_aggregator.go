package app

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"deep_in_memory/internal/data"

	"github.com/dustin/go-humanize"
)

// LogEntry represents a single aggregated log entry with timestamp
type LogEntry struct {
	Timestamp time.Time
	EventType string
	Count     int
	Addresses []uint64
	Sizes     map[uint64]uint64
	PID       int
	TID       int
}

// LogAggregator aggregates eBPF events for the selected process.
// Thread-safe: all public methods use mutex protection for concurrent access
// from eBPF event handlers and UI update goroutines.
type LogAggregator struct {
	mu sync.Mutex // Protects all fields below
	// Map of event type -> (pid/tid -> (address -> event info with count))
	events  map[string]map[eventSourceKey]map[uint64]*eventInfo
	history []LogEntry // Last N log entries (newest first)
	maxLogs int
}

type eventSourceKey struct {
	pid int
	tid int
}

type eventInfo struct {
	count  int
	length uint64
}

// NewLogAggregator creates a new log aggregator
func NewLogAggregator() *LogAggregator {
	return &LogAggregator{
		events: map[string]map[eventSourceKey]map[uint64]*eventInfo{
			"brk":        make(map[eventSourceKey]map[uint64]*eventInfo),
			"mmap":       make(map[eventSourceKey]map[uint64]*eventInfo),
			"munmap":     make(map[eventSourceKey]map[uint64]*eventInfo),
			"clone":      make(map[eventSourceKey]map[uint64]*eventInfo),
			"init":       make(map[eventSourceKey]map[uint64]*eventInfo),
			"swap_write": make(map[eventSourceKey]map[uint64]*eventInfo),
			"swap_read":  make(map[eventSourceKey]map[uint64]*eventInfo),
		},
		history: make([]LogEntry, 0, 30),
		maxLogs: 30,
	}
}

// Record adds an event to the aggregator for the given PID and TID.
// Thread-safe: can be called concurrently from multiple eBPF event handlers.
func (la *LogAggregator) Record(eventType string, addr, length uint64, pid, tid int) {
	la.mu.Lock()
	defer la.mu.Unlock()

	if eventMap, ok := la.events[eventType]; ok {
		sourceKey := eventSourceKey{pid: pid, tid: tid}
		addrMap, exists := eventMap[sourceKey]
		if !exists {
			addrMap = make(map[uint64]*eventInfo)
			eventMap[sourceKey] = addrMap
		}

		if info, exists := addrMap[addr]; exists {
			info.count++
			if length > 0 {
				info.length = length
			}
		} else {
			addrMap[addr] = &eventInfo{count: 1, length: length}
		}
	}
}

// Flush creates log entries and adds them to history, then clears aggregators.
// Thread-safe: acquires exclusive lock to ensure atomic flush operation.
func (la *LogAggregator) Flush(pids []int) {
	la.mu.Lock()
	defer la.mu.Unlock()
	_ = pids

	now := time.Now()

	// Process all event types
	for eventType, bySource := range la.events {
		if len(bySource) > 0 {
			for source, addrMap := range bySource {
				la.addToHistoryLocked(LogEntry{
					Timestamp: now,
					EventType: eventType,
					Count:     sumCounts(addrMap),
					Addresses: getAddresses(addrMap),
					Sizes:     getAddressSizes(addrMap),
					PID:       source.pid,
					TID:       source.tid,
				})
			}
			// Clear the map for this event type
			la.events[eventType] = make(map[eventSourceKey]map[uint64]*eventInfo)
		}
	}
}

// addToHistoryLocked adds a log entry to history, maintaining max size (newest first).
// IMPORTANT: Caller MUST hold la.mu lock before calling this method.
// This is an internal helper method called only from Flush() which already holds the lock.
func (la *LogAggregator) addToHistoryLocked(entry LogEntry) {
	// Prepend to keep newest at top
	la.history = append([]LogEntry{entry}, la.history...)

	// Trim to max size
	if len(la.history) > la.maxLogs {
		la.history = la.history[:la.maxLogs]
	}
}

// formatEventType converts internal event type names to display names
func formatEventType(eventType string) string {
	switch eventType {
	case "brk":
		return "virtual memory mapping [brk]"
	case "mmap":
		return "virtual memory mapping [mmap]"
	case "munmap":
		return "virtual memory unmapping [munmap]"
	case "init":
		return "lazy physical allocation"
	case "clone":
		return "process/thread cloning"
	case "swap_write":
		return "page_out"
	case "swap_read":
		return "page_in"
	default:
		return eventType
	}
}

// GetFormattedLogs returns formatted log lines (newest first).
// Thread-safe: creates a safe copy of the history under lock.
func (la *LogAggregator) GetFormattedLogs() []string {
	la.mu.Lock()
	defer la.mu.Unlock()

	if len(la.history) == 0 {
		return nil
	}

	lines := make([]string, len(la.history))
	for i, entry := range la.history {
		timestamp := entry.Timestamp.Format("2006-01-02 15:04:05")
		// addresses are not available for clone events, so we display "n/a" in that case
		addrStr := "n/a"
		if entry.EventType != "clone" {
			if entry.EventType == "mmap" || entry.EventType == "munmap" {
				addrStr = formatAddressesWithSizes(entry.Addresses, entry.Sizes)
			} else {
				addrStr = formatAddressRanges(entry.Addresses)
			}
		}
		countStr := fmt.Sprintf("%d", entry.Count)
		if entry.EventType == "init" || entry.EventType == "swap_write" || entry.EventType == "swap_read" {
			countStr = fmt.Sprintf("%d [%s]", entry.Count, humanize.IBytes(uint64(entry.Count)*data.PageSizeBytes))
		}
		displayType := formatEventType(entry.EventType)
		lines[i] = fmt.Sprintf("%s | PID:%d, TID:%d | %s -> count: %s, addresses: %s",
			timestamp, entry.PID, entry.TID, displayType, countStr, addrStr)
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

func getAddressSizes(m map[uint64]*eventInfo) map[uint64]uint64 {
	sizes := make(map[uint64]uint64, len(m))
	for addr, info := range m {
		sizes[addr] = info.length
	}
	return sizes
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

func formatAddressesWithSizes(addrs []uint64, sizes map[uint64]uint64) string {
	if len(addrs) == 0 {
		return ""
	}

	sorted := make([]uint64, len(addrs))
	copy(sorted, addrs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	parts := make([]string, 0, len(sorted))
	for _, addr := range sorted {
		length := sizes[addr]
		if length == 0 {
			parts = append(parts, fmt.Sprintf("0x%x", addr))
			continue
		}
		parts = append(parts, fmt.Sprintf("0x%x [%s]", addr, humanize.IBytes(length)))
	}

	return strings.Join(parts, ", ")
}
