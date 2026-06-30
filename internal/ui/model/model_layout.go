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

// viewportDimensions holds the computed width/height for each viewport pane.
type viewportDimensions struct {
	virtualWidth   int
	physicalWidth  int
	logsWidth      int
	virtualHeight  int
	physicalHeight int
	barWidth       int
}

// calcViewportDimensions derives viewport sizes from the current layout metrics.
func (m *Model) calcViewportDimensions() viewportDimensions {
	available, rowHeight, secondRowHeight, barWidth := m.layoutMetrics()

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

	return viewportDimensions{
		virtualWidth:   virtualWidth,
		physicalWidth:  physicalWidth,
		logsWidth:      logsWidth,
		virtualHeight:  virtualHeight,
		physicalHeight: physicalHeight,
		barWidth:       barWidth,
	}
}

// applyViewportDimensions resizes all viewports and caches their rendered output.
func (m *Model) applyViewportDimensions(d viewportDimensions) {
	m.viewportVirtual.SetWidth(d.virtualWidth)
	m.viewportVirtual.SetHeight(d.virtualHeight)
	m.viewportPhysical.SetWidth(d.physicalWidth)
	m.viewportPhysical.SetHeight(d.physicalHeight)
	m.viewportLogs.SetWidth(d.logsWidth)
	m.viewportLogs.SetHeight(d.physicalHeight) // logs use same height as physical
}

// updateViewports calculates viewport dimensions and renders content for both the virtual
// memory pane (top) and physical memory pane (bottom) based on the selected process data.
// It handles layout constraints and pagination for memory range display.
func (m *Model) updateViewports(pd *data.ProcessData) {
	if pd == nil {
		return
	}

	d := m.calcViewportDimensions()
	m.applyViewportDimensions(d)

	summary := uirender.RenderSummary(pd.Proc, pd)
	m.viewportVirtual.SetContent(uirender.RenderVirtualSectionContent(summary, pd, d.barWidth, m.rangesPage, d.virtualWidth, m.showAllLabelTypes))
	m.viewportPhysical.SetContent(uirender.RenderPhysicalSectionContent(m.buildPhysicalMemoryState(d.physicalWidth, d.physicalHeight)))
	// Logs content is updated via GetLogsMsg timer

	// Cache viewport content for performance
	m.cachedVirtual = m.viewportVirtual.View()
	m.cachedPhysical = m.viewportPhysical.View()
	m.cachedLogs = m.viewportLogs.View()
	m.dirty = true
}

func (m *Model) clearViewports() {
	d := m.calcViewportDimensions()
	m.applyViewportDimensions(d)

	m.viewportVirtual.SetContent(uirender.RenderVirtualEmptyContent())
	m.viewportPhysical.SetContent(uirender.RenderPhysicalSectionContent(m.buildPhysicalMemoryState(d.physicalWidth, d.physicalHeight)))
	// Don't clear logs - they persist independently of selected process

	// Cache viewport content for performance
	m.cachedVirtual = m.viewportVirtual.View()
	m.cachedPhysical = m.viewportPhysical.View()
	m.cachedLogs = m.viewportLogs.View()
	m.dirty = true
}
