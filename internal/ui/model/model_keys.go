package model

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"deep_in_memory/internal/constants"
	uimsg "deep_in_memory/internal/ui/msg"
)

// keyAction is a small dispatch unit for keyboard handlers.
type keyAction func() (tea.Model, tea.Cmd)

func (m *Model) modalHideTick() tea.Cmd {
	return tea.Tick(constants.ModalDisplayDuration, func(time.Time) tea.Msg {
		return uimsg.HideModalMsg{}
	})
}

func (m *Model) resetPhysicalMemory() {
	m.pagemapResult = nil
	m.pagemapHasPFN = false
	m.pagemapPFN = 0
	m.physicalInput.SetValue(constants.DefaultAddressInputValue)
	m.suggestions = nil
	m.showSuggestions = false
	m.selectedSuggestion = -1
	m.physicalScrollOff = 0
	m.physicalFocused = false
	m.physicalInput.Blur()
}

func (m *Model) blurPhysicalInput() {
	m.physicalFocused = false
	m.physicalInput.Blur()
	m.showSuggestions = false
	m.selectedSuggestion = -1
}

func (m *Model) cycleSuggestion(delta int) {
	if !m.showSuggestions || len(m.suggestions) == 0 {
		return
	}
	m.selectedSuggestion += delta
	if m.selectedSuggestion >= len(m.suggestions) {
		m.selectedSuggestion = 0
	}
	if m.selectedSuggestion < 0 {
		m.selectedSuggestion = len(m.suggestions) - 1
	}
}

func (m *Model) scrollIomem(delta int) {
	if m.physicalFocused || len(m.iomemRanges) == 0 {
		return
	}
	m.physicalScrollOff += delta
	if m.physicalScrollOff < 0 {
		m.physicalScrollOff = 0
	}
	if m.physicalScrollOff >= len(m.iomemRanges) {
		m.physicalScrollOff = len(m.iomemRanges) - 1
	}
}

func (m *Model) handleQuit() (tea.Model, tea.Cmd) {
	return m, func() tea.Msg { return tea.Quit() }
}

func (m *Model) handleEnterSelection() (tea.Model, tea.Cmd) {
	sel, ok := m.list.SelectedItem().(listItem)
	if !ok {
		return m, nil
	}

	m.selectedPID = sel.pid
	m.rangesPage = 0
	m.resetPhysicalMemory()

	if m.setTargetPID != nil {
		if err := m.setTargetPID(sel.pid); err != nil {
			m.showModal = true
			m.modalType = constants.ModalTypeError
			m.modalMessage = fmt.Sprintf("Failed to set target PID %d in eBPF: %v", sel.pid, err)
			m.dirty = true
			return m, m.modalHideTick()
		}
	}

	m.syncLayout()
	m.showModal = true
	m.modalType = constants.ModalTypeInfo
	m.modalMessage = fmt.Sprintf("Selected process %s [%s]", sel.title, sel.description)
	m.dirty = true

	return m, tea.Batch(
		func() tea.Msg { return uimsg.LoadSmapsRequest{PID: sel.pid} },
		m.modalHideTick(),
	)
}

func (m *Model) handleNextRangesPage() (tea.Model, tea.Cmd) {
	if m.selectedPID == 0 {
		return m, nil
	}
	m.rangesPage++
	if m.rangesPage < 0 {
		m.rangesPage = 0
	}
	m.syncLayout()
	m.dirty = true
	return m, nil
}

func (m *Model) handlePreviousRangesPage() (tea.Model, tea.Cmd) {
	if m.selectedPID == 0 || m.rangesPage <= 0 {
		return m, nil
	}
	m.rangesPage--
	if m.rangesPage < 0 {
		m.rangesPage = 0
	}
	m.syncLayout()
	m.dirty = true
	return m, nil
}

func (m *Model) handleTabFocusToggle() (tea.Model, tea.Cmd) {
	if m.selectedPID == 0 {
		return m, nil
	}
	m.physicalFocused = true
	m.physicalInput.Focus()
	m.updateSuggestions()
	m.dirty = true
	return m, nil
}

func (m *Model) handleIomemScrollDown() (tea.Model, tea.Cmd) {
	m.scrollIomem(1)
	m.syncLayout()
	m.dirty = true
	return m, nil
}

func (m *Model) handleIomemScrollUp() (tea.Model, tea.Cmd) {
	m.scrollIomem(-1)
	m.syncLayout()
	m.dirty = true
	return m, nil
}

func (m *Model) handleToggleLogsWrap() (tea.Model, tea.Cmd) {
	m.logsWrap = !m.logsWrap
	m.dirty = true
	return m, func() tea.Msg { return uimsg.GetLogsMsg{} }
}

func (m *Model) handleToggleLabelFilter() (tea.Model, tea.Cmd) {
	m.showAllLabelTypes = !m.showAllLabelTypes
	m.syncLayout()
	m.dirty = true
	return m, nil
}

// keyHandlers returns a map of keyboard shortcuts to their corresponding actions.
func (m *Model) keyHandlers() map[string]keyAction {
	return map[string]keyAction{
		"q":      m.handleQuit,
		"ctrl+c": m.handleQuit,
		"enter":  m.handleEnterSelection,
		"n":      m.handleNextRangesPage,
		"p":      m.handlePreviousRangesPage,
		"tab":    m.handleTabFocusToggle,
		"j":      m.handleIomemScrollDown,
		"k":      m.handleIomemScrollUp,
		"w":      m.handleToggleLogsWrap,
		"f":      m.handleToggleLabelFilter,
	}
}
