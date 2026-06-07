package proc

import (
	"encoding/binary"
	"fmt"
	"os"

	"deep_in_memory/internal/data"
)

// PagemapEntry represents parsed information from /proc/PID/pagemap for a single page.
type PagemapEntry struct {
	Present   bool
	Swapped   bool
	FilePage  bool
	Exclusive bool
	SoftDirty bool
	PFN       uint64 // Page Frame Number (valid only if Present)
	SwapType  uint8  // (valid only if Swapped)
	SwapOff   uint64 // (valid only if Swapped)
	RawValue  uint64
}

// ReadPagemap reads the pagemap entry for a given virtual address in a process.
func ReadPagemap(pid int, vaddr uint64) (*PagemapEntry, error) {
	pageSize := data.PageSizeBytes
	entrySize := uint64(8)
	pageIndex := vaddr / pageSize

	path := fmt.Sprintf("/proc/%d/pagemap", pid)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open pagemap: %w", err)
	}
	defer f.Close()

	offset := int64(pageIndex * entrySize)
	_, err = f.Seek(offset, 0)
	if err != nil {
		return nil, fmt.Errorf("seek pagemap: %w", err)
	}

	var value uint64
	err = binary.Read(f, binary.LittleEndian, &value)
	if err != nil {
		return nil, fmt.Errorf("read pagemap: %w", err)
	}

	entry := &PagemapEntry{
		Present:   (value>>63)&1 == 1,
		Swapped:   (value>>62)&1 == 1,
		FilePage:  (value>>61)&1 == 1,
		Exclusive: (value>>56)&1 == 1,
		SoftDirty: (value>>55)&1 == 1,
		RawValue:  value,
	}

	if entry.Present {
		entry.PFN = value & ((1 << 55) - 1)
	}

	if entry.Swapped {
		entry.SwapType = uint8(value & 0x1f)
		entry.SwapOff = (value >> 5) & 0x7ffffffff
	}

	return entry, nil
}

// FormatPagemapEntry returns a human-readable representation of a pagemap entry.
func FormatPagemapEntry(e *PagemapEntry) []string {
	if e == nil {
		return []string{"no pagemap data"}
	}

	lines := []string{
		fmt.Sprintf("present:    %v", e.Present),
		fmt.Sprintf("swapped:    %v", e.Swapped),
		fmt.Sprintf("file_page:  %v", e.FilePage),
		fmt.Sprintf("exclusive:  %v", e.Exclusive),
		fmt.Sprintf("soft_dirty: %v", e.SoftDirty),
	}

	if e.Present {
		lines = append(lines, fmt.Sprintf("pfn:        0x%x (%d)", e.PFN, e.PFN))
		physAddr := e.PFN * data.PageSizeBytes
		lines = append(lines, fmt.Sprintf("phys_addr:  0x%x", physAddr))
		lines = append(lines, "            (looking in iomem)")
	} else {
		lines = append(lines, "pfn:        n/a")
		lines = append(lines, "phys_addr:  n/a (page not present)")
	}

	if e.Swapped {
		lines = append(lines, fmt.Sprintf("swap_type:  %d", e.SwapType))
		lines = append(lines, fmt.Sprintf("swap_off:   0x%x", e.SwapOff))
	}

	return lines
}

// GeneratePageAlignedAddresses generates page-aligned virtual addresses within a range.
// Returns up to maxCount addresses that start with the given prefix.
func GeneratePageAlignedAddresses(startVal, endVal uint64, prefix string, maxCount int) []string {
	var result []string

	// Align start to page boundary
	alignedStart := (startVal / data.PageSizeBytes) * data.PageSizeBytes
	if alignedStart < startVal {
		alignedStart += data.PageSizeBytes
	}

	for addr := alignedStart; addr < endVal && len(result) < maxCount; addr += data.PageSizeBytes {
		addrStr := fmt.Sprintf("0x%x", addr)
		if prefix == "" || len(prefix) <= 2 || hasHexPrefix(addrStr, prefix) {
			result = append(result, addrStr)
		}
	}

	return result
}

// hasHexPrefix checks if addrStr starts with the given prefix (case-insensitive).
func hasHexPrefix(addrStr, prefix string) bool {
	if len(addrStr) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		a := addrStr[i]
		b := prefix[i]
		// Case-insensitive hex comparison
		if a >= 'A' && a <= 'F' {
			a += 32
		}
		if b >= 'A' && b <= 'F' {
			b += 32
		}
		if a != b {
			return false
		}
	}
	return true
}
