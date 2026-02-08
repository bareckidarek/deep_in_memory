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
		return m, nil

	case "tab":
		// Toggle focus back to process list
		m.physicalFocused = false
		m.physicalInput.Blur()
		m.showSuggestions = false
		m.selectedSuggestion = -1
		return m, nil

	case "enter":
		if m.showSuggestions && m.selectedSuggestion >= 0 && m.selectedSuggestion < len(m.suggestions) {
			// Select the highlighted suggestion
			m.physicalInput.SetValue(m.suggestions[m.selectedSuggestion])
			m.physicalInput.CursorEnd() // Move cursor to end of selected address
			m.showSuggestions = false
			m.selectedSuggestion = -1
		}
		// Perform pagemap lookup
		return m.performPagemapLookup()

	case "down":
		if m.showSuggestions && len(m.suggestions) > 0 {
			m.selectedSuggestion++
			if m.selectedSuggestion >= len(m.suggestions) {
				m.selectedSuggestion = 0
			}
			return m, nil
		}

	case "up":
		if m.showSuggestions && len(m.suggestions) > 0 {
			m.selectedSuggestion--
			if m.selectedSuggestion < 0 {
				m.selectedSuggestion = len(m.suggestions) - 1
			}
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
	var allSuggestions []string
	for _, thread := range pd.Threads {
		for _, r := range thread.Ranges {
			addrs := data.GeneratePageAlignedAddresses(r.StartVal, r.EndVal, prefix, maxSuggestions)
			allSuggestions = append(allSuggestions, addrs...)
			if len(allSuggestions) >= maxSuggestions*2 { // Over-collect then trim
				break
			}
		}
		if len(allSuggestions) >= maxSuggestions*2 {
			break
		}
	}

	// Sort and deduplicate
	sort.Strings(allSuggestions)
	unique := make([]string, 0, len(allSuggestions))
	seen := make(map[string]bool)
	for _, s := range allSuggestions {
		if !seen[s] {
			seen[s] = true
			unique = append(unique, s)
		}
	}

	// Limit to maxSuggestions
	if len(unique) > maxSuggestions {
		unique = unique[:maxSuggestions]
	}

	m.suggestions = unique
	m.showSuggestions = len(unique) > 0
	m.selectedSuggestion = -1
}
