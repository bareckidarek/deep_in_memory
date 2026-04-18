package render

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// RenderLogsSectionContent builds the logs section text.
// If logs is empty, returns a placeholder message.
func RenderLogsSectionContent(logs []string, viewportWidth int) string {
	styledTitle := StyleTitle("[logs]")
	if len(logs) == 0 {
		return styledTitle + "\n\nNo events recorded"
	}

	lines := []string{styledTitle, ""} // title + empty line after header

	for _, log := range logs {
		if viewportWidth > 0 {
			lines = append(lines, ansi.Wordwrap(log, viewportWidth, ""))
		} else {
			lines = append(lines, log)
		}
	}

	return strings.Join(lines, "\n")
}
