package model

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	physicalInput      textinput.Model           // Text input for virtual address
	physicalFocused    bool                      // Whether physical input is focused
	suggestions        []uirender.SuggestionItem // Current address suggestions with status
	selectedSuggestion int                       // Currently highlighted suggestion (-1 = none)
	showSuggestions    bool                      // Whether suggestions dropdown is visible
	pagemapResult      []string                  // Pagemap lookup result lines
	pagemapPFN         uint64                    // Last looked-up PFN
	pagemapHasPFN      bool                      // Whether we have a valid PFN
	iomemRanges        []uimsg.IomemRangeMsg     // Parsed /proc/iomem data
	physicalScrollOff  int                       // Scroll offset for right column (iomem)

	// Logs display
	logsWrap bool // Whether log lines are word-wrapped

	// Performance optimizations - cache styles and viewport content
	virtualStyle        lipgloss.Style // Cached style for virtual memory box
	physicalStyle       lipgloss.Style // Cached style for physical memory box
	logsStyle           lipgloss.Style // Cached style for logs box
	cachedVirtual       string         // Cached viewport.View() for virtual memory
	cachedPhysical      string         // Cached viewport.View() for physical memory
	cachedLogs          string         // Cached viewport.View() for logs
	cachedView          string         // Cached entire View() output
	dirty               bool           // Whether view needs re-rendering
	lastAvailable       int            // Last available width for style cache
	lastRowHeight       int            // Last row height for style cache
	lastSecondRowHeight int            // Last second row height for style cache
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
		dirty:              true, // Initial render needed
	}

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
	m.dirty = true
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
		m.viewportLogs.SetContent(uirender.RenderLogsSectionContent(msg.Logs, m.viewportLogs.Width()))
		// Cache viewport content and mark dirty
		m.cachedLogs = m.viewportLogs.View()
		m.dirty = true
		return m, nil

	case uimsg.IomemLoadResponse:
		m.iomemRanges = msg.Ranges
		m.syncLayout()
		m.dirty = true
		return m, nil

	case uimsg.PagemapLookupResponse:
		m.pagemapResult = msg.Lines
		m.pagemapPFN = msg.PFN
		m.pagemapHasPFN = msg.HasPFN
		// Reset scroll to show highlighted item at top
		if msg.HasPFN {
			m.physicalScrollOff = 0
		}
		m.dirty = true
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
			m.dirty = true
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
		m.dirty = true

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
			m.dirty = true
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
		m.dirty = true
		m.syncLayout()
	}

	var cmdList tea.Cmd
	oldList := m.list
	m.list, cmdList = m.list.Update(msg)

	// Update focused PID based on cursor position (not selection)
	if sel, ok := m.list.SelectedItem().(listItem); ok {
		if m.focusedPID != sel.pid {
			m.focusedPID = sel.pid
			m.dirty = true // List cursor moved - re-render needed
		}
	}

	// Mark dirty if list state changed (scrolling, filtering, etc.)
	if oldList.Index() != m.list.Index() {
		m.dirty = true
	}

	return m, cmdList
}

func (m *Model) View() tea.View {
	// Use cached view if nothing changed
	if !m.dirty && m.cachedView != "" {
		view := tea.NewView(m.cachedView)
		view.AltScreen = true
		return view
	}

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
		content = uirender.RenderWithModal(content, m.modalType, m.modalMessage, m.width, m.height)
	}

	// Cache the rendered view
	m.cachedView = content
	m.dirty = false

	view := tea.NewView(content)
	view.AltScreen = true
	return view
}
