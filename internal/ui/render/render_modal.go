package render

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"deep_in_memory/internal/constants"
)

// RenderWithModal overlays a modal/notification dialog on top of the main content.
// modalType must be one of constants.ModalTypeInfo, ModalTypeWarn, or ModalTypeError.
func RenderWithModal(baseContent, modalType, modalMessage string, width, height int) string {
	var bgColor string
	switch modalType {
	case constants.ModalTypeInfo:
		bgColor = "12" // Blue
	case constants.ModalTypeWarn:
		bgColor = "214" // Orange
	case constants.ModalTypeError:
		bgColor = "9" // Red
	default:
		bgColor = "12"
	}

	// Create 3-line modal with text in the middle line
	horizontalPadding := 5
	textWidth := lipgloss.Width(modalMessage)
	totalWidth := textWidth + (horizontalPadding * 2)

	emptyLine := strings.Repeat(" ", totalWidth)
	centeredText := strings.Repeat(" ", horizontalPadding) + modalMessage + strings.Repeat(" ", horizontalPadding)

	modalLines := []string{emptyLine, centeredText, emptyLine}

	modalStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color("#000000")).
		Bold(true)

	var styledLines []string
	for _, line := range modalLines {
		styledLines = append(styledLines, modalStyle.Render(line))
	}

	modalHeight := len(styledLines)
	modalWidth := lipgloss.Width(styledLines[0])

	baseLines := strings.Split(baseContent, "\n")
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}

	modalY := constants.ModalTopOffset
	if modalY < 0 {
		modalY = 0
	}

	modalX := (width - modalWidth) / 2
	if modalX < 0 {
		modalX = 0
	}

	var outputLines []string
	for i := 0; i < height; i++ {
		var baseLine string
		if i < len(baseLines) {
			baseLine = baseLines[i]
		} else {
			baseLine = strings.Repeat(" ", width)
		}

		visualWidth := ansi.StringWidth(baseLine)
		if visualWidth < width {
			baseLine += strings.Repeat(" ", width-visualWidth)
		}

		if i >= modalY && i < modalY+modalHeight {
			modalLineIdx := i - modalY
			modalLine := styledLines[modalLineIdx]
			modalLineWidth := lipgloss.Width(modalLine)

			leftPart := ""
			if modalX > 0 {
				leftPart = ansi.Truncate(baseLine, modalX, "")
			}

			rightPart := ""
			skipChars := modalX + modalLineWidth
			if skipChars < visualWidth {
				fullWidth := ansi.StringWidth(baseLine)
				charsToKeep := fullWidth - skipChars
				if charsToKeep > 0 {
					rightPart = ansi.TruncateLeft(baseLine, skipChars, "")
				}
			}

			outputLines = append(outputLines, leftPart+modalLine+rightPart)
		} else {
			outputLines = append(outputLines, baseLine)
		}
	}

	return strings.Join(outputLines, "\n")
}
