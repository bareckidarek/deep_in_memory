package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"deep_in_memory/internal/constants"
)

// PhysicalMemoryState holds all data needed to render the physical memory section.
type PhysicalMemoryState struct {
	InputView          string              // Rendered text input
	Focused            bool                // Whether the input is focused
	Suggestions        []string            // Address suggestions
	SelectedSuggestion int                 // Currently highlighted suggestion (-1 = none)
	ShowSuggestions    bool                // Whether to show suggestions
	PagemapResult      []string            // Pagemap lookup result lines
	PagemapPFN         uint64              // PFN from last lookup
	HasPFN             bool                // Whether PFN is valid
	IomemRanges        []IomemRangeDisplay // Parsed /proc/iomem data
	TotalWidth         int                 // Total available width
	TotalHeight        int                 // Total available height
	ScrollOffset       int                 // Scroll offset for iomem column
}

// IomemRangeDisplay holds display data for an iomem range.
type IomemRangeDisplay struct {
	Start     uint64
	End       uint64
	Name      string
	Level     int
	Highlight bool // Whether this range is highlighted (PFN match)
}

// RenderPhysicalSectionContent builds the physical memory section text with two columns.
func RenderPhysicalSectionContent(state PhysicalMemoryState) string {
	if state.TotalWidth < 20 {
		state.TotalWidth = 60
	}

	// Split into two columns: left (pagemap input/results), right (iomem)
	leftWidth := state.TotalWidth / 2
	rightWidth := state.TotalWidth - leftWidth - 1 // -1 for separator
	if leftWidth < 20 {
		leftWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	leftContent := renderLeftColumn(state, leftWidth)
	rightContent := renderRightColumn(state, rightWidth)

	// Join columns with a vertical separator
	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	// Limit to available height
	availHeight := state.TotalHeight - 2 // account for title
	if availHeight < 5 {
		availHeight = 5
	}
	if maxLines > availHeight {
		maxLines = availHeight
	}

	separator := lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Render("│")

	var combined []string
	for i := 0; i < maxLines; i++ {
		var left, right string
		if i < len(leftLines) {
			left = leftLines[i]
		}
		if i < len(rightLines) {
			right = rightLines[i]
		}

		// Pad left column to fixed width
		leftVisual := ansi.StringWidth(left)
		if leftVisual < leftWidth {
			left += strings.Repeat(" ", leftWidth-leftVisual)
		} else if leftVisual > leftWidth {
			left = ansi.Truncate(left, leftWidth, "…")
		}

		combined = append(combined, left+separator+right)
	}

	title := StyleTitle("[physical memory]")
	hint := ""
	if !state.Focused {
		hint = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("  [Tab to focus input]")
	}
	return title + hint + "\n\n" + strings.Join(combined, "\n")
}

// renderLeftColumn renders the pagemap input and results column.
func renderLeftColumn(state PhysicalMemoryState, width int) string {
	var lines []string

	// Label
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#aaaaaa"))
	lines = append(lines, labelStyle.Render("Virtual Address:"))

	// Text input with border
	inputBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00afff")).
		Padding(0, 1)
	if state.Focused {
		inputBorder = inputBorder.BorderForeground(lipgloss.Color("#00ff00"))
	}
	lines = append(lines, inputBorder.Render(state.InputView))

	// Suggestions dropdown
	if state.ShowSuggestions && len(state.Suggestions) > 0 {
		lines = append(lines, renderSuggestions(state.Suggestions, state.SelectedSuggestion, width))
	}

	// Separator
	lines = append(lines, "")

	// Pagemap results
	if len(state.PagemapResult) > 0 {
		resultLabel := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#aaaaaa"))
		lines = append(lines, resultLabel.Render("Pagemap Result:"))
		for _, l := range state.PagemapResult {
			lines = append(lines, l)
		}
	}

	return strings.Join(lines, "\n")
}

// renderSuggestions renders the suggestion dropdown.
func renderSuggestions(suggestions []string, selected int, width int) string {
	var lines []string
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#cccccc")).
		Background(lipgloss.Color("#333333"))
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#00af5f")).
		Bold(true)

	for i, s := range suggestions {
		// Pad suggestion to width
		padded := s
		if len(padded) < width-2 {
			padded += strings.Repeat(" ", width-2-len(padded))
		}

		if i == selected {
			lines = append(lines, selectedStyle.Render(" "+padded+" "))
		} else {
			lines = append(lines, normalStyle.Render(" "+padded+" "))
		}
	}
	return strings.Join(lines, "\n")
}

// renderRightColumn renders the /proc/iomem display column.
func renderRightColumn(state PhysicalMemoryState, width int) string {
	var lines []string

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#aaaaaa"))
	lines = append(lines, headerStyle.Render("/proc/iomem [use j/k to scroll]:"))

	if len(state.IomemRanges) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("No data (requires root)"))
		return strings.Join(lines, "\n")
	}

	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	highlightStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(constants.GreenColor)).
		Bold(true)

	// Apply scroll offset
	startIdx := state.ScrollOffset
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= len(state.IomemRanges) {
		startIdx = len(state.IomemRanges) - 1
		if startIdx < 0 {
			startIdx = 0
		}
	}

	// Calculate how many lines we can display (total height - header - separator)
	maxDisplayLines := state.TotalHeight - 5
	if maxDisplayLines < 5 {
		maxDisplayLines = 5
	}

	endIdx := startIdx + maxDisplayLines
	if endIdx > len(state.IomemRanges) {
		endIdx = len(state.IomemRanges)
	}

	for i := startIdx; i < endIdx; i++ {
		r := state.IomemRanges[i]
		indent := strings.Repeat("  ", r.Level)

		// Calculate range size and format with appropriate units
		size := int64(r.End - r.Start + 1)
		sizeStr := HumanBytes(size)

		text := fmt.Sprintf("%s%x-%x %s [%s]", indent, r.Start, r.End, r.Name, sizeStr)

		// Truncate if too long
		if len(text) > width-1 {
			text = text[:width-4] + "…"
		}

		if r.Highlight {
			lines = append(lines, highlightStyle.Render(text))
		} else {
			lines = append(lines, normalStyle.Render(text))
		}
	}

	// Show scroll indicator if there are more items
	if endIdx < len(state.IomemRanges) {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(fmt.Sprintf("  ... (%d more)", len(state.IomemRanges)-endIdx)))
	}

	return strings.Join(lines, "\n")
}
