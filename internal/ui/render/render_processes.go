package render

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
)

// FormatListItem formats a process into display title and description strings.
func FormatListItem(proc data.Process, pid int) (title, description string) {
	description = fmt.Sprintf("Started: %s UTC", proc.StartTime.UTC().Format("15:04:05"))
	title = fmt.Sprintf("%s (%d)", proc.Name, proc.PID)
	return
}

// RenderProcessListItem renders a process list item with appropriate styling
// based on cursor position and selection state.
// Returns rendered (title, description) strings ready for display.
func RenderProcessListItem(title, description string, isCursor, isSelected bool) (string, string) {
	prefixMarker := constants.MarkerDefaultPrefix
	titleStyle := lipgloss.NewStyle()
	descStyle := lipgloss.NewStyle().Foreground(constants.DescriptionColor)

	if isSelected {
		prefixMarker = constants.MarkerSelectedPrefix
		titleStyle = titleStyle.Foreground(constants.SelectedColor).Bold(true)
	}

	if isCursor {
		if !isSelected {
			prefixMarker = constants.MarkerFocusedPrefix
			titleStyle = titleStyle.Foreground(constants.FocusColor)
		}
		titleStyle = titleStyle.Background(constants.BackgroundColor)
		descStyle = descStyle.Background(constants.BackgroundColor)
	}

	return titleStyle.Render(prefixMarker + title), descStyle.Render(constants.MarkerDefaultPrefix + description)
}
