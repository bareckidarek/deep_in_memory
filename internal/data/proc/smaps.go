package proc

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"deep_in_memory/internal/data"
)

// LoadSmapsRanges reads /proc/PID/smaps and returns memory ranges as ThreadData.
// All ranges are assigned to TID=PID since smaps doesn't distinguish threads.
func LoadSmapsRanges(pid int) ([]data.ThreadData, error) {
	mainRanges, err := loadRangesFromSmapsPath(fmt.Sprintf("/proc/%d/smaps", pid))
	if err != nil {
		return nil, err
	}

	if len(mainRanges) == 0 {
		return nil, nil
	}

	threads := []data.ThreadData{{
		TID:    pid,
		Ranges: mainRanges,
	}}

	mainRangeSet := buildRangeSet(mainRanges)

	taskIDs, err := readTaskIds(pid)
	if err != nil {
		return threads, nil
	}

	for _, taskID := range taskIDs {
		taskRanges, loadErr := loadRangesFromSmapsPath(fmt.Sprintf("/proc/%d/smaps", taskID))
		if loadErr != nil {
			threads = append(threads, data.ThreadData{TID: taskID, Ranges: nil})
			continue
		}

		taskUniqueRanges := filterRangesNotInSet(taskRanges, mainRangeSet)

		threads = append(threads, data.ThreadData{TID: taskID, Ranges: taskUniqueRanges})
	}

	return threads, nil
}

// LoadSmapsRangesForThread reads /proc/PID/task/TID/smaps and returns memory ranges for a specific thread.
// It filters out ranges that are already present in the main process's smaps to avoid duplicates.
func LoadSmapsRangesForThread(pid int, tid int) ([]data.AddrRange, error) {
	ranges, err := loadRangesFromSmapsPath(fmt.Sprintf("/proc/%d/smaps", tid))
	if err != nil {
		return nil, err
	}

	if len(ranges) == 0 {
		return nil, nil
	}

	mainRanges, err := loadRangesFromSmapsPath(fmt.Sprintf("/proc/%d/smaps", pid))
	if err != nil {
		return ranges, nil
	}

	mainRangeSet := buildRangeSet(mainRanges)
	uniqueRanges := filterRangesNotInSet(ranges, mainRangeSet)

	return uniqueRanges, nil
}

func loadRangesFromSmapsPath(path string) ([]data.AddrRange, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open smaps: %w", err)
	}
	defer file.Close()

	var ranges []data.AddrRange
	scanner := bufio.NewScanner(file)

	var currentRange *data.AddrRange

	for scanner.Scan() {
		line := scanner.Text()

		// Look for lines like: 7f8a4c000000-7f8a4c021000 rw-p 00000000 00:00 0
		if strings.Contains(line, "-") && (strings.Contains(line, " r") || strings.Contains(line, " -")) {
			// If we have a pending range, add it before starting new one
			if currentRange != nil {
				ranges = append(ranges, *currentRange)
				currentRange = nil
			}

			fields := strings.Fields(line)
			if len(fields) < 1 {
				continue
			}

			// Extract label
			labelType, label := extractLabel(fields)

			// Parse address range
			addrRange := fields[0]
			parts := strings.Split(addrRange, "-")
			if len(parts) != 2 {
				continue
			}

			startAddr, err := strconv.ParseUint(parts[0], 16, 64)
			if err != nil {
				continue
			}

			endAddr, err := strconv.ParseUint(parts[1], 16, 64)
			if err != nil {
				continue
			}

			if endAddr <= startAddr {
				continue
			}

			size := endAddr - startAddr
			newRange := data.AddrRange{
				Start:     parts[0],
				End:       parts[1],
				StartVal:  startAddr,
				EndVal:    endAddr,
				Size:      size / data.PageSizeBytes,
				PrevSize:  size / data.PageSizeBytes, // Initialize PrevSize to current size
				Statuses:  make(map[string]*data.StatusCount),
				Label:     label,
				LabelType: labelType,
			}

			// Initialize counters to zero
			newRange.Statuses["INIT"] = &data.StatusCount{Count: 0}
			newRange.Statuses["SWAP"] = &data.StatusCount{Count: 0}
			newRange.Statuses["UNSWAP"] = &data.StatusCount{Count: 0}

			currentRange = &newRange
		} else if currentRange != nil {
			// Parse detail lines for the current range
			// Looking for "Rss:" and "Swap:" lines
			// Format: "Rss:                  12 kB"
			if strings.HasPrefix(strings.TrimSpace(line), "Rss:") {
				if rssPages := parseKBToPages(line); rssPages > 0 {
					currentRange.Statuses["INIT"].Count = rssPages
				}
			} else if strings.HasPrefix(strings.TrimSpace(line), "Swap:") {
				if swapPages := parseKBToPages(line); swapPages > 0 {
					currentRange.Statuses["SWAP"].Count = swapPages
				}
			}
		}
	}

	// Don't forget the last range
	if currentRange != nil {
		ranges = append(ranges, *currentRange)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan smaps: %w", err)
	}

	if len(ranges) == 0 {
		return nil, nil
	}

	// Sort ranges before returning.
	data.SortRanges(ranges)
	return ranges, nil
}

func readTaskIds(pid int) ([]int, error) {
	taskPath := fmt.Sprintf("/proc/%d/task/", pid)
	entries, err := os.ReadDir(taskPath)
	if err != nil {
		return nil, fmt.Errorf("read task IDs: %w", err)
	}

	fields := make([]int, 0, len(entries))
	for _, entry := range entries {
		tid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		if tid == pid { // Exclude the main thread (PID)
			continue
		}
		fields = append(fields, tid)
	}

	return fields, nil
}

func buildRangeSet(ranges []data.AddrRange) map[string]struct{} {
	set := make(map[string]struct{}, len(ranges))
	for _, r := range ranges {
		set[rangeKey(r)] = struct{}{}
	}
	return set
}

func filterRangesNotInSet(ranges []data.AddrRange, rangeSet map[string]struct{}) []data.AddrRange {
	filtered := make([]data.AddrRange, 0, len(ranges))
	for _, r := range ranges {
		if _, exists := rangeSet[rangeKey(r)]; exists {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func rangeKey(r data.AddrRange) string {
	return fmt.Sprintf("%x-%x", r.StartVal, r.EndVal)
}

// extractLabel parses the label portion of a smaps header line.
// Returns the label string (e.g. "[heap]", "/usr/lib/...", or "" for anonymous memory).
func extractLabel(fields []string) (data.LabelType, string) {
	if len(fields) < 6 {
		return data.Empty, ""
	}
	extractedLabel := strings.TrimSpace(strings.Join(fields[5:], " "))

	switch {
	case extractedLabel == "":
		return data.Empty, ""
	case strings.Contains(extractedLabel, "[heap]"):
		return data.Heap, extractedLabel
	case strings.Contains(extractedLabel, "[stack]"):
		return data.Stack, extractedLabel
	case strings.Contains(extractedLabel, "anon"):
		return data.Anonymous, extractedLabel
	case strings.HasPrefix(extractedLabel, "["):
		return data.Other, extractedLabel
	case strings.HasPrefix(extractedLabel, "/"):
		return data.FileBacked, extractedLabel
	default:
		// Check if it's just a number (inode) — treat as anonymous
		if _, err := strconv.ParseUint(extractedLabel, 10, 64); err == nil {
			return data.Anonymous, extractedLabel
		}
		return data.Other, extractedLabel
	}
}

// parseKBToPages parses a "Rss:" or "Swap:" line from /proc/PID/smaps
// and converts it to page count (dividing by the system page size).
func parseKBToPages(line string) int {
	// Expected format: "Rss:                  12 kB" or "Swap:     0 kB"
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}

	// Second field should be the numeric value
	valueStr := fields[1]
	valueKB, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return 0
	}

	// Convert kB to pages using the system page size
	pageSizeKB := int64(data.PageSizeBytes / 1024)
	if pageSizeKB == 0 {
		pageSizeKB = 4
	}
	pages := valueKB / pageSizeKB
	return int(pages)
}
