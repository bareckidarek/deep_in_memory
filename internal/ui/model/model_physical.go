package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"deep_in_memory/internal/data"
	uimsg "deep_in_memory/internal/ui/msg"
	uirender "deep_in_memory/internal/ui/render"
)

// handlePhysicalInput processes key events when the physical memory input is focused.
func (m *Model) handlePhysicalInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.physicalFocused = false
		m.physicalInput.Blur()
		m.showSuggestions = false
		m.selectedSuggestion = -1
		m.dirty = true
		return m, nil

	case "tab":
		// Toggle focus back to process list
		m.physicalFocused = false
		m.physicalInput.Blur()
		m.showSuggestions = false
		m.selectedSuggestion = -1
		m.dirty = true
		return m, nil

	case "enter":
		if m.showSuggestions && m.selectedSuggestion >= 0 && m.selectedSuggestion < len(m.suggestions) {
			// Select the highlighted suggestion
			m.physicalInput.SetValue(m.suggestions[m.selectedSuggestion].Address)
			m.physicalInput.CursorEnd() // Move cursor to end of selected address
			m.showSuggestions = false
			m.selectedSuggestion = -1
			m.dirty = true
		}
		// Perform pagemap lookup
		return m.performPagemapLookup()

	case "down":
		if m.showSuggestions && len(m.suggestions) > 0 {
			m.selectedSuggestion++
			if m.selectedSuggestion >= len(m.suggestions) {
				m.selectedSuggestion = 0
			}
			m.syncLayout()
			m.dirty = true
			return m, nil
		}

	case "up":
		if m.showSuggestions && len(m.suggestions) > 0 {
			m.selectedSuggestion--
			if m.selectedSuggestion < 0 {
				m.selectedSuggestion = len(m.suggestions) - 1
			}
			m.syncLayout()
			m.dirty = true
			return m, nil
		}

	case "ctrl+c":
		return m, func() tea.Msg { return tea.Quit() }
	}

	// Forward to text input
	var cmd tea.Cmd
	m.physicalInput, cmd = m.physicalInput.Update(msg)

	// Update suggestions based on current input
	m.updateSuggestions()

	m.syncLayout()
	m.dirty = true
	return m, cmd
}

// performPagemapLookup executes a pagemap lookup for the current input value.
func (m *Model) performPagemapLookup() (tea.Model, tea.Cmd) {
	val := m.physicalInput.Value()
	if len(val) < 3 { // Need at least "0x" + one char
		return m, nil
	}

	// Parse the virtual address
	addrStr := strings.TrimPrefix(val, "0x")
	addrStr = strings.TrimPrefix(addrStr, "0X")
	vaddr, err := strconv.ParseUint(addrStr, 16, 64)
	if err != nil {
		m.pagemapResult = []string{"Invalid address: " + val}
		m.pagemapHasPFN = false
		m.syncLayout()
		return m, nil
	}

	pid := m.selectedPID
	if pid == 0 {
		m.pagemapResult = []string{"No process selected"}
		m.pagemapHasPFN = false
		m.syncLayout()
		return m, nil
	}

	// Return command that performs async pagemap lookup
	return m, func() tea.Msg {
		entry, err := data.ReadPagemap(pid, vaddr)
		if err != nil {
			return uimsg.PagemapLookupResponse{
				Lines:      []string{fmt.Sprintf("Error: %v", err)},
				HasPFN:     false,
				VAddrInput: val,
			}
		}

		lines := data.FormatPagemapEntry(entry)
		return uimsg.PagemapLookupResponse{
			Lines:      lines,
			PFN:        entry.PFN,
			HasPFN:     entry.Present,
			VAddrInput: val,
		}
	}
}

// buildPhysicalMemoryState constructs the render state for the physical memory section.
func (m *Model) buildPhysicalMemoryState(width, height int) uirender.PhysicalMemoryState {
	// Build iomem display ranges with highlight
	var displayRanges []uirender.IomemRangeDisplay
	var highlightedRanges []uirender.IomemRangeDisplay
	var normalRanges []uirender.IomemRangeDisplay

	for _, r := range m.iomemRanges {
		highlight := false
		if m.pagemapHasPFN && m.pagemapPFN > 0 {
			physAddr := m.pagemapPFN * data.PageSizeBytes
			if physAddr >= r.Start && physAddr <= r.End {
				highlight = true
			}
		}
		display := uirender.IomemRangeDisplay{
			Start:     r.Start,
			End:       r.End,
			Name:      r.Name,
			Level:     r.Level,
			Highlight: highlight,
		}
		if highlight {
			highlightedRanges = append(highlightedRanges, display)
		} else {
			normalRanges = append(normalRanges, display)
		}
	}

	// Put highlighted ranges first, then normal ranges
	displayRanges = append(displayRanges, highlightedRanges...)
	displayRanges = append(displayRanges, normalRanges...)

	return uirender.PhysicalMemoryState{
		InputView:          m.physicalInput.View(),
		Focused:            m.physicalFocused,
		Suggestions:        m.suggestions,
		SelectedSuggestion: m.selectedSuggestion,
		ShowSuggestions:    m.showSuggestions,
		PagemapResult:      m.pagemapResult,
		PagemapPFN:         m.pagemapPFN,
		HasPFN:             m.pagemapHasPFN,
		IomemRanges:        displayRanges,
		TotalWidth:         width,
		TotalHeight:        height,
		ScrollOffset:       m.physicalScrollOff,
	}
}

// updateSuggestions rebuilds the suggestion list based on current input text.
func (m *Model) updateSuggestions() {
	prefix := m.physicalInput.Value()
	if len(prefix) < 2 {
		m.suggestions = nil
		m.showSuggestions = false
		m.selectedSuggestion = -1
		return
	}

	if m.selectedPID == 0 {
		m.suggestions = nil
		m.showSuggestions = false
		return
	}

	pd, ok := m.data.Processes[m.selectedPID]
	if !ok || pd == nil {
		m.suggestions = nil
		m.showSuggestions = false
		return
	}

	const maxSuggestions = 20

	// Collect all page-aligned addresses from all ranges that match the prefix
	var allSuggestions []uirender.SuggestionItem
	seen := make(map[string]bool)

	for _, thread := range pd.Threads {
		for _, r := range thread.Ranges {
			addrs := data.GeneratePageAlignedAddresses(r.StartVal, r.EndVal, prefix, maxSuggestions)
			for _, addr := range addrs {
				if !seen[addr] {
					seen[addr] = true
					// Determine the status of this specific address within the range
					status := getAddressStatus(addr, r)
					allSuggestions = append(allSuggestions, uirender.SuggestionItem{
						Address: addr,
						Status:  status,
					})
				}
				if len(allSuggestions) >= maxSuggestions*2 { // Over-collect then trim
					break
				}
			}
			if len(allSuggestions) >= maxSuggestions*2 {
				break
			}
		}
		if len(allSuggestions) >= maxSuggestions*2 {
			break
		}
	}

	// Sort by address
	sort.Slice(allSuggestions, func(i, j int) bool {
		return allSuggestions[i].Address < allSuggestions[j].Address
	})

	// Limit to maxSuggestions
	if len(allSuggestions) > maxSuggestions {
		allSuggestions = allSuggestions[:maxSuggestions]
	}

	m.suggestions = allSuggestions
	m.showSuggestions = len(allSuggestions) > 0
	m.selectedSuggestion = -1
}

// getAddressStatus determines the memory status of a specific virtual address within a range.
// Returns: "" (reserved only), "INIT", "SWAP", or "UNSWAP".
func getAddressStatus(addrStr string, r data.AddrRange) string {
	// Parse the address to get the page number offset within the range
	addrStr = strings.TrimPrefix(addrStr, "0x")
	addrStr = strings.TrimPrefix(addrStr, "0X")
	addr, err := strconv.ParseUint(addrStr, 16, 64)
	if err != nil {
		return "" // Reserved only if we can't parse
	}

	// Check if address is within this range
	if addr < r.StartVal || addr > r.EndVal {
		return "" // Outside range
	}

	// Check statuses in priority order: SWAP > UNSWAP > INIT
	// (SWAP is most important to highlight)
	if statusCount := r.Statuses["SWAP"]; statusCount != nil && statusCount.Count > 0 {
		// For simplicity, assume uniform distribution
		// In real implementation, would check actual page offsets from address database
		return "SWAP"
	}
	if statusCount := r.Statuses["UNSWAP"]; statusCount != nil && statusCount.Count > 0 {
		return "UNSWAP"
	}
	if statusCount := r.Statuses["INIT"]; statusCount != nil && statusCount.Count > 0 {
		return "INIT"
	}

	// No specific status - just reserved
	return ""
}
