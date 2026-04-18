package render

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// RenderLogsSectionContent builds the logs section text.
// If logs is empty, returns a placeholder message.
func RenderLogsSectionContent(logs []string, viewportWidth int, logsWrap bool) string {
	title := StyleTitle("[logs]") + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("  [w: toggle wrap]")
	if len(logs) == 0 {
		return title + "\n\nNo events recorded"
	}

	lines := []string{title, ""} // title + empty line after header

	for _, log := range logs {
		if logsWrap && viewportWidth > 0 {
			wrapped := ansi.Wordwrap(log, viewportWidth, "")
			lines = append(lines, wrapped)
		} else {
			lines = append(lines, log)
		}
	}

	return strings.Join(lines, "\n")
}
