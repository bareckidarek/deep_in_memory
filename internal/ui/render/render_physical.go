package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"deep_in_memory/internal/constants"
	"github.com/dustin/go-humanize"
)

// SuggestionItem represents a single address suggestion with its memory status.
type SuggestionItem struct {
	Address string // Virtual address (e.g., "0x12345000")
	Status  string // Memory status: "" (reserved only), "INIT", "SWAP", "UNSWAP"
}

// PhysicalMemoryState holds all data needed to render the physical memory section.
type PhysicalMemoryState struct {
	InputView          string              // Rendered text input
	Focused            bool                // Whether the input is focused
	Suggestions        []SuggestionItem    // Address suggestions with status
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

type physicalColumnsLayout struct {
	leftWidth   int
	rightWidth  int
	availHeight int
}

func calculatePhysicalColumnsLayout(totalWidth, totalHeight int) physicalColumnsLayout {
	if totalWidth < 20 {
		totalWidth = 60
	}

	leftWidth := totalWidth / 2
	rightWidth := totalWidth - leftWidth - 1 // -1 for separator
	if leftWidth < 20 {
		leftWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	availHeight := totalHeight - 2 // account for title
	if availHeight < 5 {
		availHeight = 5
	}

	return physicalColumnsLayout{
		leftWidth:   leftWidth,
		rightWidth:  rightWidth,
		availHeight: availHeight,
	}
}

func mergePhysicalColumns(leftContent, rightContent string, leftWidth, maxLines int) string {
	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")

	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	separator := lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Render("│")
	combined := make([]string, 0, maxLines)
	for i := 0; i < maxLines; i++ {
		var left, right string
		if i < len(leftLines) {
			left = leftLines[i]
		}
		if i < len(rightLines) {
			right = rightLines[i]
		}

		leftVisual := ansi.StringWidth(left)
		if leftVisual < leftWidth {
			left += strings.Repeat(" ", leftWidth-leftVisual)
		} else if leftVisual > leftWidth {
			left = ansi.Truncate(left, leftWidth, "…")
		}

		combined = append(combined, left+separator+right)
	}

	return strings.Join(combined, "\n")
}

// RenderPhysicalSectionContent builds the physical memory section text with two columns.
func RenderPhysicalSectionContent(state PhysicalMemoryState) string {
	layout := calculatePhysicalColumnsLayout(state.TotalWidth, state.TotalHeight)

	leftContent := renderLeftColumn(state, layout.leftWidth)
	rightContent := renderRightColumn(state, layout.rightWidth)
	maxLines := layout.availHeight
	combined := mergePhysicalColumns(leftContent, rightContent, layout.leftWidth, maxLines)

	title := StyleTitle("[physical memory]")
	hint := ""
	if !state.Focused {
		hint = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(constants.PhysicalHintTabToFocus)
	}
	return title + hint + "\n\n" + combined
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

// renderSuggestions renders the suggestion dropdown with colored backgrounds based on memory status.
func renderSuggestions(suggestions []SuggestionItem, selected int, width int) string {
	var lines []string

	for i, item := range suggestions {
		normalColor, _ := StatusColors(item.Status)

		// Base style with status-based background
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color(normalColor))

		// Add marker and bold for selected item
		prefix := constants.MarkerDefaultPrefix
		if i == selected {
			prefix = constants.MarkerSelectedPrefix
			style = style.Bold(true).Foreground(lipgloss.Color("#ffffff"))
		}

		// Build display text
		displayText := prefix + item.Address

		// Pad to width
		textLen := len(prefix) + len(item.Address)
		if textLen < width-2 {
			displayText += strings.Repeat(" ", width-2-textLen)
		}

		lines = append(lines, style.Render(" "+displayText+" "))
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
	// Calculate how many lines we can display (total height - header - separator)
	maxDisplayLines := state.TotalHeight - 5
	if maxDisplayLines < 5 {
		maxDisplayLines = 5
	}

	startIdx, endIdx := CalculateScrollWindow(len(state.IomemRanges), state.ScrollOffset, maxDisplayLines)

	for i := startIdx; i < endIdx; i++ {
		r := state.IomemRanges[i]
		indent := strings.Repeat("  ", r.Level)

		// Calculate range size and format with appropriate units
		size := r.End - r.Start + 1
		sizeStr := humanize.IBytes(size)

		text := fmt.Sprintf("%s%x-%x %s [%s]", indent, r.Start, r.End, r.Name, sizeStr)

		// Truncate if too long
		if ansi.StringWidth(text) > width-1 {
			text = ansi.Truncate(text, width-1, "…")
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
