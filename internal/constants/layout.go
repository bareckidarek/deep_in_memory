package constants

import "github.com/charmbracelet/lipgloss"

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
