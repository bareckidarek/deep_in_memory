package render

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"deep_in_memory/internal/constants"
)

func modalBackgroundColor(modalType string) string {
	switch modalType {
	case constants.ModalTypeInfo:
		return constants.ModalInfoColor
	case constants.ModalTypeWarn:
		return constants.ModalWarnColor
	case constants.ModalTypeError:
		return constants.ModalErrorColor
	default:
		return constants.ModalInfoColor
	}
}

func buildModalLines(modalMessage string, bgColor string) []string {
	horizontalPadding := constants.ModalHorizontalPadding
	textWidth := lipgloss.Width(modalMessage)
	totalWidth := textWidth + (horizontalPadding * 2)

	emptyLine := strings.Repeat(" ", totalWidth)
	centeredText := strings.Repeat(" ", horizontalPadding) + modalMessage + strings.Repeat(" ", horizontalPadding)

	modalStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color("#000000")).
		Bold(true)

	return []string{
		modalStyle.Render(emptyLine),
		modalStyle.Render(centeredText),
		modalStyle.Render(emptyLine),
	}
}

func normalizeBaseLines(baseContent string, width, height int) []string {
	baseLines := strings.Split(baseContent, "\n")
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}

	normalized := make([]string, 0, height)
	for i := 0; i < height; i++ {
		line := strings.Repeat(" ", width)
		if i < len(baseLines) {
			line = baseLines[i]
		}

		visualWidth := ansi.StringWidth(line)
		if visualWidth < width {
			line += strings.Repeat(" ", width-visualWidth)
		}
		normalized = append(normalized, line)
	}

	return normalized
}

func overlayModalLine(baseLine, modalLine string, modalX int) string {
	modalWidth := lipgloss.Width(modalLine)
	leftPart := ""
	if modalX > 0 {
		leftPart = ansi.Truncate(baseLine, modalX, "")
	}

	baseWidth := ansi.StringWidth(baseLine)
	rightPart := ""
	skipChars := modalX + modalWidth
	if skipChars < baseWidth {
		rightPart = ansi.TruncateLeft(baseLine, skipChars, "")
	}

	return leftPart + modalLine + rightPart
}

// RenderWithModal overlays a modal/notification dialog on top of the main content.
// modalType must be one of constants.ModalTypeInfo, ModalTypeWarn, or ModalTypeError.
func RenderWithModal(baseContent, modalType, modalMessage string, width, height int) string {
	styledLines := buildModalLines(modalMessage, modalBackgroundColor(modalType))
	modalHeight := len(styledLines)
	modalWidth := lipgloss.Width(styledLines[0])
	baseLines := normalizeBaseLines(baseContent, width, height)

	modalY := constants.ModalTopOffset
	if modalY < 0 {
		modalY = 0
	}

	modalX := (width - modalWidth) / 2
	if modalX < 0 {
		modalX = 0
	}

	outputLines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		baseLine := baseLines[i]

		if i >= modalY && i < modalY+modalHeight {
			modalLineIdx := i - modalY
			modalLine := styledLines[modalLineIdx]
			outputLines = append(outputLines, overlayModalLine(baseLine, modalLine, modalX))
		} else {
			outputLines = append(outputLines, baseLine)
		}
	}

	return strings.Join(outputLines, "\n")
}
