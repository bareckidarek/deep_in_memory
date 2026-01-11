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

			// Filter: accept all ranges WITHOUT file paths
			// Extract label - everything after the last column (inode or 0)
			// Labels can contain spaces like "[anon: Go: gc bits]"
			var label string

			// Find the bracket or path after the fixed columns
			// Format: addr permissions offset dev inode [label or /path]
			// Need to get everything after field[4] (inode/0)
			if len(fields) >= 6 {
				// Reconstruct what comes after the 5th field
				// Find position after "00:00 X" pattern
				parts := strings.SplitN(line, " ", 6)
				if len(parts) == 6 {
					// parts[5] contains everything after the inode/device, with leading spaces
					remainder := strings.TrimSpace(parts[5])

					if remainder == "" {
						// No label - anonymous memory
						label = ""
					} else if strings.HasPrefix(remainder, "[") {
						// Bracketed label like [heap], [stack], [anon: Go: gc bits]
						label = remainder
					} else if strings.Contains(remainder, "anon") {
						// anon_inode:bpf-map
						label = remainder
					} else if strings.HasPrefix(remainder, "/") {
						// File path - skip this range
						continue
					} else {
						// Check if it's just a number (inode)
						if _, err := strconv.ParseUint(remainder, 10, 64); err != nil {
							// Not a number - likely a file path without /, skip
							continue
						}
						label = ""
					}
				}
			}

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

	// Return all ranges under a single ThreadData with TID=PID
	return []ThreadData{
		{
			TID:    pid,
			Ranges: ranges,
		},
	}, nil
}

// parseKBToPages extracts the numeric value from a line like "Rss:   12 kB"
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

	// Convert kB to pages (4 kB per page)
	pages := valueKB / 4
	return int(pages)
}
