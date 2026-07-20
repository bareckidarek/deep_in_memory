package model

import (
	"fmt"
	"os"
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
	// Process list and dataset state
	list        list.Model
	data        data.Dataset
	selectedPID int // Process selected with Enter (shows details)
	focusedPID  int // Process under cursor (arrow navigation)

	// Window and error state
	width  int
	height int
	err    error

	// Viewports
	viewportVirtual  viewport.Model
	viewportPhysical viewport.Model
	viewportLogs     viewport.Model

	// App callbacks and virtual pagination
	setTargetPID func(int) error
	rangesPage   int // Current page for virtual ranges pagination (0-indexed)

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

	// Label type filtering
	showAllLabelTypes bool // When false (default), only Heap and Empty ranges are shown

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

	os.Setenv("COLORTERM", "truecolor")

	ti := textinput.New()
	ti.Placeholder = "enter hex address..."
	ti.SetValue(constants.DefaultAddressInputValue)
	ti.CharLimit = 20
	ti.SetWidth(22)
	ti.Prompt = constants.MarkerSelectedPrefix

	// Apply styles to focused and blurred states
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

	// Initialize logs viewport with default content
	m.viewportLogs.SetContent(uirender.RenderLogsSectionContent(nil, m.viewportLogs.Width(), m.logsWrap))

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
	l.Styles.Title = lipgloss.NewStyle().
		Background(lipgloss.Color(constants.SectionTitleBgColor)).
		Foreground(lipgloss.Color(constants.SectionTitleFgColor)).
		Bold(true)

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

func (m *Model) handleLogsResponse(msg uimsg.LogsResponseMsg) (tea.Model, tea.Cmd) {
	m.viewportLogs.SetContent(uirender.RenderLogsSectionContent(msg.Logs, m.viewportLogs.Width(), m.logsWrap))
	m.cachedLogs = m.viewportLogs.View()
	m.dirty = true
	return m, nil
}

func (m *Model) handleIomemLoad(msg uimsg.IomemLoadResponse) (tea.Model, tea.Cmd) {
	m.iomemRanges = msg.Ranges
	m.syncLayout()
	m.dirty = true
	return m, nil
}

func (m *Model) handlePagemapLookup(msg uimsg.PagemapLookupResponse) (tea.Model, tea.Cmd) {
	m.pagemapResult = msg.Lines
	m.pagemapPFN = msg.PFN
	m.pagemapHasPFN = msg.HasPFN
	if msg.HasPFN {
		m.physicalScrollOff = 0
	}
	m.dirty = true
	m.syncLayout()
	return m, nil
}

func (m *Model) handleClearPhysicalMemory(msg uimsg.ClearPhysicalMemoryMsg) (tea.Model, tea.Cmd) {
	if m.selectedPID == msg.PID {
		m.resetPhysicalMemory()
		m.dirty = true
		m.syncLayout()
	}
	return m, nil
}

func (m *Model) handleApplicationError(msg uimsg.ApplicationError) (tea.Model, tea.Cmd) {
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

	return m, tea.Tick(constants.ModalDisplayDuration, func(time.Time) tea.Msg {
		return uimsg.HideModalMsg{}
	})
}

func (m *Model) handleHideModal() (tea.Model, tea.Cmd) {
	m.showModal = false
	return m, nil
}

func (m *Model) handlePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	if !m.physicalFocused {
		return m, nil
	}

	currentVal := m.physicalInput.Value()
	m.physicalInput.SetValue(currentVal + msg.String())
	m.physicalInput.CursorEnd()
	m.updateSuggestions()
	m.syncLayout()
	m.dirty = true
	return m, nil
}

func (m *Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.physicalFocused {
		model, cmd := m.handlePhysicalInput(msg)
		return model, cmd, true
	}
	if handler, ok := m.keyHandlers()[msg.String()]; ok {
		model, cmd := handler()
		return model, cmd, true
	}
	return m, nil, false
}

func (m *Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.list.SetSize(constants.ListWidthFallback, max(m.height-2, constants.DefaultListHeight))
	m.dirty = true
	m.syncLayout()
	return m, nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case uimsg.LogsResponseMsg:
		return m.handleLogsResponse(msg)

	case uimsg.IomemLoadResponse:
		return m.handleIomemLoad(msg)

	case uimsg.PagemapLookupResponse:
		return m.handlePagemapLookup(msg)

	case uimsg.ClearPhysicalMemoryMsg:
		return m.handleClearPhysicalMemory(msg)

	case uimsg.ApplicationError:
		return m.handleApplicationError(msg)
	case uimsg.HideModalMsg:
		return m.handleHideModal()
	case tea.PasteMsg:
		return m.handlePaste(msg)
	case tea.KeyPressMsg:
		model, cmd, handled := m.handleKeyPress(msg)
		if handled {
			return model, cmd
		}
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
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
