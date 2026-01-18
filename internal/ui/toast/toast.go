package toast

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"deep_in_memory/internal/constants"
)

// ToastType represents the type of toast notification
type ToastType string

const (
	ToastTypeInfo    ToastType = "info"
	ToastTypeSuccess ToastType = "success"
	ToastTypeWarning ToastType = "warning"
	ToastTypeError   ToastType = "error"
)

// Toast represents a toast notification
type Toast struct {
	Message   string
	Type      ToastType
	ExpiresAt time.Time
	Visible   bool
}

// NewToast creates a new toast notification
func NewToast(message string, toastType ToastType) *Toast {
	return &Toast{
		Message:   message,
		Type:      toastType,
		ExpiresAt: time.Now().Add(constants.ToastDuration),
		Visible:   true,
	}
}

// IsExpired checks if the toast has expired
func (t *Toast) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// Hide marks the toast as hidden
func (t *Toast) Hide() {
	t.Visible = false
}

// Render renders the toast notification as a styled string
func (t *Toast) Render(width int) string {
	if !t.Visible {
		return ""
	}

	// Get color based on toast type
	var color lipgloss.Color
	switch t.Type {
	case ToastTypeInfo:
		color = constants.ToastInfoColor
	case ToastTypeSuccess:
		color = constants.ToastSuccessColor
	case ToastTypeWarning:
		color = constants.ToastWarningColor
	case ToastTypeError:
		color = constants.ToastErrorColor
	default:
		color = constants.ToastInfoColor
	}

	// Create toast style
	toastStyle := lipgloss.NewStyle().
		Background(constants.ToastBgColor).
		Foreground(color).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(constants.ToastBorderColor).
		Padding(0, 1).
		Width(width - 4) // Account for border padding

	// Truncate message if too long
	maxMsgWidth := width - 8 // Account for border and padding
	if maxMsgWidth < 10 {
		maxMsgWidth = 10
	}
	message := t.Message
	if len(message) > maxMsgWidth {
		message = message[:maxMsgWidth-3] + "..."
	}

	// Center the message within available width
	lines := strings.Split(message, "\n")
	renderedLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) > maxMsgWidth {
			// Word wrap long lines
			wrapped := lipgloss.NewStyle().Width(maxMsgWidth).Render(line)
			wrappedLines := strings.Split(wrapped, "\n")
			renderedLines = append(renderedLines, wrappedLines...)
		} else {
			renderedLines = append(renderedLines, line)
		}
	}

	rendered := toastStyle.Render(strings.Join(renderedLines, "\n"))
	return rendered
}

// ToastExpiredMsg is a message sent when a toast expires
type ToastExpiredMsg struct{}

// StartToastTimer starts a timer that will send ToastExpiredMsg when the toast expires
func StartToastTimer(toast *Toast) tea.Cmd {
	if toast == nil {
		return nil
	}

	duration := time.Until(toast.ExpiresAt)
	if duration <= 0 {
		return nil
	}

	return tea.Tick(duration, func(t time.Time) tea.Msg {
		return ToastExpiredMsg{}
	})
}
