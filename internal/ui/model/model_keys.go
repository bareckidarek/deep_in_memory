package model

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"deep_in_memory/internal/constants"
	uimsg "deep_in_memory/internal/ui/msg"
)

// keyHandlers returns a map of keyboard shortcuts to their corresponding actions.
// Supports navigation (arrows), selection (Enter), pagination (n/p), and quitting (q/Ctrl+C).
func (m *Model) keyHandlers() map[string]func() (tea.Model, tea.Cmd) {
	return map[string]func() (tea.Model, tea.Cmd){
		"q":      func() (tea.Model, tea.Cmd) { return m, func() tea.Msg { return tea.Quit() } },
		"ctrl+c": func() (tea.Model, tea.Cmd) { return m, func() tea.Msg { return tea.Quit() } },
		"enter": func() (tea.Model, tea.Cmd) {
			if sel, ok := m.list.SelectedItem().(listItem); ok {
				// Enter selects the focused process
				m.selectedPID = sel.pid
				m.rangesPage = 0 // Reset to first page when selecting new process

				// Reset physical memory section state for new process
				m.pagemapResult = nil
				m.pagemapHasPFN = false
				m.pagemapPFN = 0
				m.physicalInput.SetValue("0x")
				m.suggestions = nil
				m.showSuggestions = false
				m.selectedSuggestion = -1
				m.physicalFocused = false
				m.physicalInput.Blur()

				if m.setTargetPID != nil {
					if err := m.setTargetPID(sel.pid); err != nil {
						m.showModal = true
						m.modalType = constants.ModalTypeError
						m.modalMessage = fmt.Sprintf("Failed to set target PID %d in eBPF: %v", sel.pid, err)
						m.dirty = true
						// Don't proceed with success modal
						return m, tea.Tick(constants.ModalDisplayDuration, func(time.Time) tea.Msg {
							return uimsg.HideModalMsg{}
						})
					}
				}
				m.syncLayout()

				// Show modal with info notification
				m.showModal = true
				m.modalType = constants.ModalTypeInfo
				m.modalMessage = fmt.Sprintf("Selected process %d", sel.pid)
				m.dirty = true

				// Send message to load smaps for this process and hide modal after configured duration
				return m, tea.Batch(
					func() tea.Msg {
						return uimsg.LoadSmapsRequest{PID: sel.pid}
					},
					tea.Tick(constants.ModalDisplayDuration, func(time.Time) tea.Msg {
						return uimsg.HideModalMsg{}
					}),
				)
			}
			return m, nil
		},
		"n": func() (tea.Model, tea.Cmd) {
			// Next page of virtual ranges
			if m.selectedPID != 0 {
				m.rangesPage++
				m.syncLayout()
				m.dirty = true
			}
			return m, nil
		},
		"p": func() (tea.Model, tea.Cmd) {
			// Previous page of virtual ranges
			if m.selectedPID != 0 && m.rangesPage > 0 {
				m.rangesPage--
				m.syncLayout()
				m.dirty = true
			}
			return m, nil
		},
		"tab": func() (tea.Model, tea.Cmd) {
			// Toggle focus to physical memory input
			if m.selectedPID != 0 {
				m.physicalFocused = true
				m.physicalInput.Focus()
				m.updateSuggestions()
				m.dirty = true
			}
			return m, nil
		},
		"j": func() (tea.Model, tea.Cmd) {
			// Scroll iomem down (when not focused on input)
			if !m.physicalFocused && len(m.iomemRanges) > 0 {
				m.physicalScrollOff++
				if m.physicalScrollOff >= len(m.iomemRanges) {
					m.physicalScrollOff = len(m.iomemRanges) - 1
				}
				m.syncLayout()
				m.dirty = true
			}
			return m, nil
		},
		"k": func() (tea.Model, tea.Cmd) {
			// Scroll iomem up (when not focused on input)
			if !m.physicalFocused && len(m.iomemRanges) > 0 {
				m.physicalScrollOff--
				if m.physicalScrollOff < 0 {
					m.physicalScrollOff = 0
				}
				m.syncLayout()
				m.dirty = true
			}
			return m, nil
		},
		"w": func() (tea.Model, tea.Cmd) {
			// Toggle word-wrap in logs viewport
			m.logsWrap = !m.logsWrap
			// Re-request logs so the viewport rebuilds with new wrap setting
			m.dirty = true
			return m, func() tea.Msg { return uimsg.GetLogsMsg{} }
		},
		"f": func() (tea.Model, tea.Cmd) {
			// Toggle label type filter (Heap+Empty only vs. all)
			m.showAllLabelTypes = !m.showAllLabelTypes
			m.syncLayout()
			m.dirty = true
			return m, nil
		},
	}
}
