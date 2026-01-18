package constants

import (
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Layout constants for the terminal UI
const (
	// Default dimensions
	DefaultListWidth  = 30
	DefaultListHeight = 20

	// Layout spacing and padding
	LeftPanePadding   = 4 // border (2) + padding (2)
	ListWidthFallback = DefaultListWidth + 2

	// Minimum dimensions
	MinAvailableWidth  = 20
	MinBarWidth        = 10
	MinContentHeight   = 6
	MinViewportHeight  = 3
	StatusBarReduction = 4
	BarWidthBuffer     = 50

	// Layout percentages
	VirtualMemoryPercent  = 55
	PhysicalMemoryPercent = 45

	// Pagination
	RangesPerPage = 25

	// Display constants
	StatusLegendWidth = 59 // "██ reservation  ██ allocation  ██ page-out  ██ page-in" = 59 chars visible

	// Matrix display
	MatrixColumns = 8
	MatrixMaxRows = 6

	// Phase sampling
	PhaseSampleLimit = 5
)

// Event constants for eBPF tracepoints
const (
	// Page fault error codes
	PageFaultErrorCode = 0x6 // User write page fault

	// Swap event error codes
	SwapEventSuccessCode = 0 // Success code for swap events
)

// Timing constants
const (
	// Cache refresh intervals
	RangeCacheRefreshInterval = 100 * time.Millisecond

	// eBPF collector retry delays
	EbpfRetryDelay = 10 * time.Millisecond

	// Application refresh intervals
	DefaultRefreshInterval = 1 * time.Second

	// Toast notification duration
	ToastDuration = 3 * time.Second
)

// Data processing constants
const (
	// File system units
	BytesPerKilobyte = 1024
)

// UI interaction constants
const (
	// Mouse event offsets
	BorderOffset  = 1
	PaddingOffset = 1
)

// Color constants for memory status visualization
var (
	// Memory status colors
	GrayColor  = "#666666" // reservation/unmapped
	GreenColor = "#00af5f" // allocation (INIT)
	RedColor   = "#ff5f5f" // page-out (SWAP)
	BlueColor  = "#5f87ff" // page-in (UNSWAP)

	// UI colors
	SelectedColor    = lipgloss.Color("10")  // Green for selected process
	FocusColor       = lipgloss.Color("12")  // Blue for cursor focus
	BackgroundColor  = lipgloss.Color("236") // Dark background for focused items
	DescriptionColor = lipgloss.Color("241") // Gray for descriptions

	// Toast notification colors
	ToastInfoColor    = lipgloss.Color("12")  // Blue for info notifications
	ToastSuccessColor = lipgloss.Color("10")  // Green for success notifications
	ToastWarningColor = lipgloss.Color("11")  // Yellow for warning notifications
	ToastErrorColor   = lipgloss.Color("9")   // Red for error notifications
	ToastBgColor      = lipgloss.Color("235") // Dark background for toast
	ToastBorderColor  = lipgloss.Color("240") // Border color for toast
)

// Status order for rendering memory bars
var StatusOrder = []string{"SWAP", "UNSWAP", "INIT"}

// Memory status labels for display
const (
	StatusReservation = "reservation"
	StatusAllocation  = "allocation"
	StatusPageOut     = "page-out"
	StatusPageIn      = "page-in"
)
