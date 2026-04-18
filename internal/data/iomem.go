package data

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// IomemRange represents a single entry from /proc/iomem.
type IomemRange struct {
	Start uint64
	End   uint64
	Name  string
	Level int // indentation level (0 = top-level)
}

// LoadIomem reads /proc/iomem and returns all system memory ranges.
// Requires root or CAP_SYS_RAWIO for actual addresses (otherwise all zeros).
func LoadIomem() ([]IomemRange, error) {
	file, err := os.Open("/proc/iomem")
	if err != nil {
		return nil, fmt.Errorf("open /proc/iomem: %w", err)
	}
	defer file.Close()

	var ranges []IomemRange
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		r, ok := parseIomemLine(line)
		if ok {
			ranges = append(ranges, r)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan /proc/iomem: %w", err)
	}

	return ranges, nil
}

// parseIomemLine parses a single line from /proc/iomem.
// Format: "  00001000-0009ffff : System RAM"
func parseIomemLine(line string) (IomemRange, bool) {
	// Count leading spaces for indentation level
	level := 0
	for _, ch := range line {
		if ch == ' ' {
			level++
		} else {
			break
		}
	}
	level /= 2 // Each level is typically 2 spaces

	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return IomemRange{}, false
	}

	// Split on " : "
	colonIdx := strings.Index(trimmed, " : ")
	if colonIdx < 0 {
		return IomemRange{}, false
	}

	addrPart := trimmed[:colonIdx]
	name := trimmed[colonIdx+3:]

	// Parse address range "start-end"
	parts := strings.Split(addrPart, "-")
	if len(parts) != 2 {
		return IomemRange{}, false
	}

	start, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 16, 64)
	if err != nil {
		return IomemRange{}, false
	}

	end, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 16, 64)
	if err != nil {
		return IomemRange{}, false
	}

	return IomemRange{
		Start: start,
		End:   end,
		Name:  name,
		Level: level,
	}, true
}


