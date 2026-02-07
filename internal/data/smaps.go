package data

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LoadSmapsRanges reads /proc/PID/smaps and returns memory ranges as ThreadData.
// All ranges are assigned to TID=PID since smaps doesn't distinguish threads.
func LoadSmapsRanges(pid int) ([]ThreadData, error) {
	smapsPath := fmt.Sprintf("/proc/%d/smaps", pid)
	file, err := os.Open(smapsPath)
	if err != nil {
		return nil, fmt.Errorf("open smaps: %w", err)
	}
	defer file.Close()

	var ranges []AddrRange
	scanner := bufio.NewScanner(file)

	var currentRange *AddrRange

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
			label := extractLabel(fields, line)

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
			newRange := AddrRange{
				Start:    parts[0],
				End:      parts[1],
				StartVal: startAddr,
				EndVal:   endAddr,
				Size:     size / PageSizeBytes,
				Statuses: make(map[string]*StatusCount),
				Label:    label,
			}

			// Initialize counters to zero
			newRange.Statuses["INIT"] = &StatusCount{Count: 0}
			newRange.Statuses["SWAP"] = &StatusCount{Count: 0}
			newRange.Statuses["UNSWAP"] = &StatusCount{Count: 0}

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

	// Sort ranges before returning
	SortRanges(ranges)

	// Return all ranges under a single ThreadData with TID=PID
	return []ThreadData{
		{
			TID:    pid,
			Ranges: ranges,
		},
	}, nil
}

// extractLabel parses the label portion of a smaps header line.
// Returns the label string (e.g. "[heap]", "/usr/lib/...", or "" for anonymous memory).
func extractLabel(fields []string, line string) string {
	if len(fields) < 6 {
		return ""
	}

	// Reconstruct everything after the 5th field (inode/device).
	parts := strings.SplitN(line, " ", 6)
	if len(parts) < 6 {
		return ""
	}
	remainder := strings.TrimSpace(parts[5])

	switch {
	case remainder == "":
		return ""
	case strings.HasPrefix(remainder, "["):
		return remainder // e.g. [heap], [stack], [anon: Go: gc bits]
	case strings.Contains(remainder, "anon"):
		return remainder // e.g. anon_inode:bpf-map
	case strings.HasPrefix(remainder, "/"):
		return remainder // file-backed mapping
	default:
		// Check if it's just a number (inode) — treat as anonymous
		if _, err := strconv.ParseUint(remainder, 10, 64); err == nil {
			return ""
		}
		return remainder
	}
}

// and converts it to page count (dividing by 4 kB per page).
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
	pageSizeKB := int64(PageSizeBytes / 1024)
	if pageSizeKB == 0 {
		pageSizeKB = 4
	}
	pages := valueKB / pageSizeKB
	return int(pages)
}
