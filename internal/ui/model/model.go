package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	uimsg "deep_in_memory/internal/ui/msg"
	uirender "deep_in_memory/internal/ui/render"
)

// Model wraps the Bubble Tea state for the TUI.
type Model struct {
	list             list.Model
	data             data.Dataset
	selectedPID      int // Process selected with Enter (shows details)
	focusedPID       int // Process under cursor (arrow navigation)
	width            int
	height           int
	err              error
	viewportVirtual  viewport.Model
	viewportPhysical viewport.Model
	viewportLogs     viewport.Model
	setTargetPID     func(int) error
	rangesPage       int // Current page for virtual ranges pagination (0-indexed)

	// Modal state
	showModal    bool
	modalType    string // info, warn, error
	modalMessage string

	// Physical memory section state
	physicalInput      textinput.Model       // Text input for virtual address
	physicalFocused    bool                  // Whether physical input is focused
	suggestions        []string              // Current address suggestions
	selectedSuggestion int                   // Currently highlighted suggestion (-1 = none)
	showSuggestions    bool                  // Whether suggestions dropdown is visible
	pagemapResult      []string              // Pagemap lookup result lines
	pagemapPFN         uint64                // Last looked-up PFN
	pagemapHasPFN      bool                  // Whether we have a valid PFN
	iomemRanges        []uimsg.IomemRangeMsg // Parsed /proc/iomem data
	physicalScrollOff  int                   // Scroll offset for right column (iomem)
}

// New constructs a Model from dataset.
func New(d data.Dataset) *Model {
	ti := textinput.New()
	ti.Placeholder = "enter hex address..."
	ti.SetValue("0x")
	ti.CharLimit = 20
	ti.SetWidth(22)
	ti.Prompt = "► "

	// Stylizacja - pobierz domyślne style i zmodyfikuj
	styles := ti.Styles()
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#00afff")).Bold(true)
	styles.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff"))
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Italic(true)
	styles.Blurred.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#0087af"))
	styles.Blurred.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#cccccc"))
	styles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Italic(true)
	ti.SetStyles(styles)

	m := &Model{
		data:               d,
		selectedPID:        0, // No selection until user presses Enter
		focusedPID:         0,
		viewportVirtual:    viewport.New(),
		viewportPhysical:   viewport.New(),
		viewportLogs:       viewport.New(),
		physicalInput:      ti,
		physicalFocused:    false,
		selectedSuggestion: -1,
	}

	// Initialize logs viewport with default content
	m.viewportLogs.SetContent("[logs]\n\nNo events recorded")

	items := make([]list.Item, 0, len(d.Order))
	for _, pid := range d.Order {
		items = append(items, renderListItem(d.Processes[pid].Proc, pid))
	}

	delegate := newProcessDelegate(m)
	l := list.New(items, delegate, constants.DefaultListWidth, constants.DefaultListHeight)
	l.Title = "[processes]"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	// Set title style to match other section headers
	l.Styles.Title = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Bold(true)

	m.list = l

	if len(d.Order) > 0 {
		m.focusedPID = d.Order[0]
	}

	return m
}

// SetDataset replaces the underlying dataset and refreshes the list selection if possible.
func (m *Model) SetDataset(d data.Dataset) {
	m.data = d
	m.refreshList()
	m.syncLayout()
}

// SetTargetPIDSetter provides a callback used to push the selected PID into eBPF maps.
func (m *Model) SetTargetPIDSetter(fn func(int) error) {
	m.setTargetPID = fn
}

func (m *Model) Init() tea.Cmd {
	return nil // App handles log request timing
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case uimsg.LogsResponseMsg:
		// Update logs viewport with received logs
		if len(msg.Logs) == 0 {
			styledTitle := uirender.StyleTitle("[logs]")
			m.viewportLogs.SetContent(styledTitle + "\n\nNo events recorded")
		} else {
			var lines []string
			lines = append(lines, uirender.StyleTitle("[logs]"))
			lines = append(lines, "") // Empty line after header

			// Word wrap each log line to viewport width
			for _, log := range msg.Logs {
				// Wrap line if it exceeds viewport width
				if m.viewportLogs.Width() > 0 {
					wrapped := ansi.Wordwrap(log, m.viewportLogs.Width(), "")
					lines = append(lines, wrapped)
				} else {
					lines = append(lines, log)
				}
			}

			m.viewportLogs.SetContent(strings.Join(lines, "\n"))
		}
		return m, nil

	case uimsg.IomemLoadResponse:
		m.iomemRanges = msg.Ranges
		m.syncLayout()
		return m, nil

	case uimsg.PagemapLookupResponse:
		m.pagemapResult = msg.Lines
		m.pagemapPFN = msg.PFN
		m.pagemapHasPFN = msg.HasPFN
		// Reset scroll to show highlighted item at top
		if msg.HasPFN {
			m.physicalScrollOff = 0
		}
		m.syncLayout()
		return m, nil

	case uimsg.ClearPhysicalMemoryMsg:
		// Clear physical memory state only if it matches the exiting process
		if m.selectedPID == msg.PID {
			m.pagemapResult = nil
			m.pagemapHasPFN = false
			m.pagemapPFN = 0
			m.physicalInput.SetValue("0x")
			m.suggestions = nil
			m.showSuggestions = false
			m.selectedSuggestion = -1
			m.physicalScrollOff = 0
			// Return focus to process list
			m.physicalFocused = false
			m.physicalInput.Blur()
			m.syncLayout()
		}
		return m, nil

	case uimsg.ApplicationError:
		// Show modal based on severity
		m.showModal = true
		switch msg.Severity {
		case "error":
			m.modalType = constants.ModalTypeError
		case "warning", "warn":
			m.modalType = constants.ModalTypeWarn
		default:
			m.modalType = constants.ModalTypeInfo
		}
		m.modalMessage = msg.Message

		// Hide modal after configured duration
		return m, tea.Tick(constants.ModalDisplayDuration, func(time.Time) tea.Msg {
			return uimsg.HideModalMsg{}
		})
	case uimsg.HideModalMsg:
		m.showModal = false
		return m, nil
	case tea.PasteMsg:
		// Handle paste (Ctrl+V / Cmd+V) when physical input is focused
		if m.physicalFocused {
			// PasteMsg contains the pasted text
			currentVal := m.physicalInput.Value()
			m.physicalInput.SetValue(currentVal + msg.String())
			m.physicalInput.CursorEnd()
			m.updateSuggestions()
			m.syncLayout()
		}
		return m, nil
	case tea.KeyPressMsg:
		// When physical input is focused, route keys there
		if m.physicalFocused {
			return m.handlePhysicalInput(msg)
		}
		if handler, ok := m.keyHandlers()[msg.String()]; ok {
			return handler()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(constants.ListWidthFallback, max(m.height-2, constants.DefaultListHeight))
		m.syncLayout()
	}

	var cmdList tea.Cmd
	m.list, cmdList = m.list.Update(msg)

	// Update focused PID based on cursor position (not selection)
	if sel, ok := m.list.SelectedItem().(listItem); ok {
		m.focusedPID = sel.pid
	}

	return m, cmdList
}

func (m *Model) View() tea.View {
	var content string
	if m.err != nil {
		content = fmt.Sprintf("error: %v", m.err)
	} else {
		left := m.list.View()
		leftBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(m.leftPaneWidth()).Height(m.height - 2).Render(left)
		right := m.renderSplitDetails()
		content = lipgloss.JoinHorizontal(lipgloss.Top, leftBox, right)
	}

	// If modal is active, overlay it on top of the content
	if m.showModal {
		content = m.renderWithModal(content)
	}

	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

// renderWithModal overlays a modal/notification dialog on top of the main content.
func (m *Model) renderWithModal(baseContent string) string {
	// Create modal box styled based on notification type
	var bgColor string
	switch m.modalType {
	case constants.ModalTypeInfo:
		bgColor = "12" // Blue
	case constants.ModalTypeWarn:
		bgColor = "214" // Orange
	case constants.ModalTypeError:
		bgColor = "9" // Red
	default:
		bgColor = "12" // Default to blue
	}

	// Create 3-line modal with text in the middle line
	horizontalPadding := 5
	textWidth := lipgloss.Width(m.modalMessage)
	totalWidth := textWidth + (horizontalPadding * 2)

	emptyLine := strings.Repeat(" ", totalWidth)
	centeredText := strings.Repeat(" ", horizontalPadding) + m.modalMessage + strings.Repeat(" ", horizontalPadding)

	modalLines := []string{
		emptyLine,
		centeredText,
		emptyLine,
	}

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
	if len(baseLines) > m.height {
		baseLines = baseLines[:m.height]
	}

	modalY := constants.ModalTopOffset
	if modalY < 0 {
		modalY = 0
	}

	modalX := (m.width - modalWidth) / 2
	if modalX < 0 {
		modalX = 0
	}

	var outputLines []string

	for i := 0; i < m.height; i++ {
		var baseLine string
		if i < len(baseLines) {
			baseLine = baseLines[i]
		} else {
			baseLine = strings.Repeat(" ", m.width)
		}

		visualWidth := ansi.StringWidth(baseLine)
		if visualWidth < m.width {
			baseLine += strings.Repeat(" ", m.width-visualWidth)
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
