package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/geodro/lerd/internal/feedback"
)

// Palette is shared with the CLI feedback package so the TUI and the
// command-line progress output stay in lockstep.
var (
	colTitle    = feedback.ColTitle
	colDim      = feedback.ColDim
	colDivider  = feedback.ColDivider
	colRunning  = feedback.ColRunning
	colStopped  = feedback.ColStopped
	colFailing  = feedback.ColFailing
	colPaused   = feedback.ColPaused
	colAccent   = feedback.ColAccent
	colSelected = feedback.ColTitle
)

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(colTitle)
	sectionStyle   = lipgloss.NewStyle().Bold(true).Foreground(colDim)
	dimStyle       = lipgloss.NewStyle().Foreground(colDim)
	selectedStyle  = lipgloss.NewStyle().Bold(true).Foreground(colSelected)
	focusedPane    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 1)
	unfocusedPane  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colDivider).Padding(0, 1)
	runningStyle   = lipgloss.NewStyle().Foreground(colRunning)
	stoppedStyle   = lipgloss.NewStyle().Foreground(colStopped)
	failingStyle   = lipgloss.NewStyle().Foreground(colFailing).Bold(true)
	pausedStyle    = lipgloss.NewStyle().Foreground(colPaused)
	suspendedStyle = lipgloss.NewStyle().Foreground(colPaused)
	accentStyle    = lipgloss.NewStyle().Foreground(colAccent)
	helpStyle      = lipgloss.NewStyle().Foreground(colDim)
)

const (
	glyphRunning   = "●"
	glyphStopped   = "○"
	glyphFailing   = "✖"
	glyphPaused    = "◐"
	glyphSuspended = "◔"
)

// keyChipStyle wraps a single keybinding name (e.g. " y ", " esc ") in a
// pill: accent background, dark foreground, padded by one space on each
// side. Used in modal footers and toast actions so the user sees the
// shortcut as a button instead of as inline prose.
var (
	keyChipStyle = lipgloss.NewStyle().
			Background(colAccent).
			Foreground(lipgloss.Color("#0b0b0b")).
			Bold(true).
			Padding(0, 1)
	keyChipLabelStyle = lipgloss.NewStyle().Foreground(colDim)
)

// spinnerFrames cycles through Braille spinner glyphs (the same set
// charm/bubbletea uses in its spinner package). Animated by the existing
// tickCmd — every snapshotMsg also bumps the spinner phase indirectly via
// time.Now sampling in renderSpinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
