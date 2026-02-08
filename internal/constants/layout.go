package constants

import (
	"time"

	"charm.land/lipgloss/v2"
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

	// Modal notification timeout
	ModalDisplayDuration = 3 * time.Second

	// Log aggregation flush interval
	LogFlushInterval = 2 * time.Second

	// Event batching flush interval
	EventBatchFlushInterval = 50 * time.Millisecond
)

// Event batching sizes for different collectors
const (
	// Batch sizes optimized for event frequency
	EventBatchSize = 200 // General batch size for all collectors to maintain order
)

// Modal notification types
const (
	ModalTypeInfo  = "info"
	ModalTypeWarn  = "warn"
	ModalTypeError = "error"
)

// Modal position constants
const (
	ModalTopOffset = 2 // Lines from top of screen
)

// Data processing constants
const (
	// File system units
	BytesPerKilobyte = 1024
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
