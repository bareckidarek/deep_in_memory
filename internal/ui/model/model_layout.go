package model

import (
	"charm.land/lipgloss/v2"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	uirender "deep_in_memory/internal/ui/render"
)

// syncLayout ensures viewport sizes and content stay in sync with current selection and window size.
func (m *Model) syncLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	// Only show details if a process is selected (via Enter)
	if m.selectedPID != 0 {
		if pd, ok := m.currentProcessData(); ok {
			m.updateViewports(pd)
		} else {
			// Selected process no longer exists, clear selection
			m.selectedPID = 0
			m.clearViewports()
		}
	} else {
		m.clearViewports()
	}
}

// leftPaneWidth approximates rendered width of the bordered list box.
func (m *Model) leftPaneWidth() int {
	listWidth := m.list.Width()
	if listWidth == 0 {
		listWidth = constants.ListWidthFallback // default width plus default border padding used by list
	}
	return listWidth + constants.LeftPanePadding
}

// layoutMetrics returns key layout values used for routing events and rendering.
func (m *Model) layoutMetrics() (available, rowHeight, secondRowHeight, barWidth int) {
	available = m.width - m.leftPaneWidth()
	if available < constants.MinAvailableWidth {
		available = constants.MinAvailableWidth
	}
	barWidth = available - constants.BarWidthBuffer
	if barWidth < constants.MinBarWidth {
		barWidth = constants.MinBarWidth
	}

	contentHeight := m.height
	if contentHeight < constants.MinContentHeight {
		contentHeight = constants.MinContentHeight
	}
	// Virtual memory gets VirtualMemoryPercent%, physical memory gets PhysicalMemoryPercent%
	rowHeight = (contentHeight * constants.VirtualMemoryPercent) / 100
	if rowHeight < constants.MinContentHeight {
		rowHeight = constants.MinContentHeight
	}
	secondRowHeight = contentHeight - rowHeight
	if secondRowHeight < constants.MinContentHeight {
		secondRowHeight = constants.MinContentHeight
	}

	return
}

// updateStyles recreates lipgloss styles only when layout dimensions change.
// This prevents recreating styles on every View() call, significantly reducing CPU usage.
func (m *Model) updateStyles(available, rowHeight, secondRowHeight int) {
	if m.lastAvailable == available &&
		m.lastRowHeight == rowHeight &&
		m.lastSecondRowHeight == secondRowHeight {
		return
	}

	halfWidth := available / 2

	m.virtualStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(available).
		Height(rowHeight - 2)

	m.physicalStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(halfWidth).
		Height(secondRowHeight - 2)

	m.logsStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(available - halfWidth).
		Height(secondRowHeight - 2)

	m.lastAvailable = available
	m.lastRowHeight = rowHeight
	m.lastSecondRowHeight = secondRowHeight
	m.dirty = true
}

func (m *Model) renderSplitDetails() string {
	available, rowHeight, secondRowHeight, _ := m.layoutMetrics()

	// Update cached styles only if dimensions changed
	m.updateStyles(available, rowHeight, secondRowHeight)

	// Use cached viewport content
	virtualBox := m.virtualStyle.Render(m.cachedVirtual)
	physicalBox := m.physicalStyle.Render(m.cachedPhysical)
	logsBox := m.logsStyle.Render(m.cachedLogs)

	secondRow := lipgloss.JoinHorizontal(lipgloss.Top, physicalBox, logsBox)
	stack := lipgloss.JoinVertical(lipgloss.Left, virtualBox, secondRow)
	return stack
}

// updateViewports calculates viewport dimensions and renders content for both the virtual
// memory pane (top) and physical memory pane (bottom) based on the selected process data.
// It handles layout constraints and pagination for memory range display.
func (m *Model) updateViewports(pd *data.ProcessData) {
	if pd == nil {
		return
	}

	available, rowHeight, secondRowHeight, barWidth := m.layoutMetrics()
	proc := pd.Proc
	summary := uirender.RenderSummary(proc, pd)

	virtualWidth := available - constants.LeftPanePadding
	if virtualWidth < constants.MinBarWidth {
		virtualWidth = constants.MinBarWidth
	}

	// Split physical/logs width (second row is split horizontally)
	halfWidth := available / 2
	physicalWidth := halfWidth - constants.LeftPanePadding
	logsWidth := available - halfWidth - constants.LeftPanePadding

	if physicalWidth < constants.MinBarWidth {
		physicalWidth = constants.MinBarWidth
	}
	if logsWidth < constants.MinBarWidth {
		logsWidth = constants.MinBarWidth
	}

	virtualHeight := rowHeight - 2
	if virtualHeight < constants.MinViewportHeight {
		virtualHeight = constants.MinViewportHeight
	}
	physicalHeight := secondRowHeight - 2
	if physicalHeight < constants.MinViewportHeight {
		physicalHeight = constants.MinViewportHeight
	}
	logsHeight := physicalHeight // Same height as physical

	m.viewportVirtual.SetWidth(virtualWidth)
	m.viewportVirtual.SetHeight(virtualHeight)
	m.viewportPhysical.SetWidth(physicalWidth)
	m.viewportPhysical.SetHeight(physicalHeight)
	m.viewportLogs.SetWidth(logsWidth)
	m.viewportLogs.SetHeight(logsHeight)

	m.viewportVirtual.SetContent(uirender.RenderVirtualSectionContent(summary, pd, barWidth, m.rangesPage, virtualWidth))
	m.viewportPhysical.SetContent(uirender.RenderPhysicalSectionContent(m.buildPhysicalMemoryState(physicalWidth, physicalHeight)))
	// Logs content is updated via FlushLogsMsg timer

	// Cache viewport content for performance
	m.cachedVirtual = m.viewportVirtual.View()
	m.cachedPhysical = m.viewportPhysical.View()
	m.cachedLogs = m.viewportLogs.View()
	m.dirty = true
}

func (m *Model) clearViewports() {
	// Set dimensions same as updateViewports to maintain consistent layout
	available, rowHeight, secondRowHeight, _ := m.layoutMetrics()

	virtualWidth := available - constants.LeftPanePadding
	if virtualWidth < constants.MinBarWidth {
		virtualWidth = constants.MinBarWidth
	}

	halfWidth := available / 2
	physicalWidth := halfWidth - constants.LeftPanePadding
	logsWidth := available - halfWidth - constants.LeftPanePadding

	if physicalWidth < constants.MinBarWidth {
		physicalWidth = constants.MinBarWidth
	}
	if logsWidth < constants.MinBarWidth {
		logsWidth = constants.MinBarWidth
	}

	virtualHeight := rowHeight - 2
	if virtualHeight < constants.MinViewportHeight {
		virtualHeight = constants.MinViewportHeight
	}
	physicalHeight := secondRowHeight - 2
	if physicalHeight < constants.MinViewportHeight {
		physicalHeight = constants.MinViewportHeight
	}
	logsHeight := physicalHeight

	m.viewportVirtual.SetWidth(virtualWidth)
	m.viewportVirtual.SetHeight(virtualHeight)
	m.viewportPhysical.SetWidth(physicalWidth)
	m.viewportPhysical.SetHeight(physicalHeight)
	m.viewportLogs.SetWidth(logsWidth)
	m.viewportLogs.SetHeight(logsHeight)

	// Set content with styled titles
	m.viewportVirtual.SetContent(uirender.RenderVirtualEmptyContent())
	m.viewportPhysical.SetContent(uirender.RenderPhysicalSectionContent(m.buildPhysicalMemoryState(physicalWidth, physicalHeight)))
	// Don't clear logs - they persist independently of selected process

	// Cache viewport content for performance
	m.cachedVirtual = m.viewportVirtual.View()
	m.cachedPhysical = m.viewportPhysical.View()
	m.cachedLogs = m.viewportLogs.View()
	m.dirty = true
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
