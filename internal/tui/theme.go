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
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colTitle)
	sectionStyle  = lipgloss.NewStyle().Bold(true).Foreground(colDim)
	dimStyle      = lipgloss.NewStyle().Foreground(colDim)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(colSelected)
	focusedPane   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 1)
	unfocusedPane = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colDivider).Padding(0, 1)
	runningStyle  = lipgloss.NewStyle().Foreground(colRunning)
	stoppedStyle  = lipgloss.NewStyle().Foreground(colStopped)
	failingStyle  = lipgloss.NewStyle().Foreground(colFailing).Bold(true)
	// Active-but-unreachable: same problem colour as failing, but not bold and a
	// distinct glyph, so it reads as its own state rather than a systemd failure.
	unreachableStyle = lipgloss.NewStyle().Foreground(colFailing)
	pausedStyle      = lipgloss.NewStyle().Foreground(colPaused)
	suspendedStyle   = lipgloss.NewStyle().Foreground(colPaused)
	accentStyle      = lipgloss.NewStyle().Foreground(colAccent)
	helpStyle        = lipgloss.NewStyle().Foreground(colDim)
)

// Footer key-hint styles. Navigation/view keys stay in the main accent colour
// so movement reads as one family; keys that act on something (start, stop,
// toggle, heal, …) are amber so destructive/mutating shortcuts stand apart at
// a glance. Labels stay dim so the coloured key is what the eye catches.
var (
	footNavKeyStyle    = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	footActionKeyStyle = lipgloss.NewStyle().Foreground(colPaused).Bold(true)
	footLabelStyle     = lipgloss.NewStyle().Foreground(colDim)
)

// Top tab bar styles. The active tab reads as a filled accent pill so it
// stands out as the current screen; inactive tabs sit dim until hovered or
// clicked. Both keep the same padding so the bar's hit regions line up.
var (
	tabActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0b0b0b")).Background(colAccent).Padding(0, 2)
	tabInactiveStyle = lipgloss.NewStyle().Foreground(colDim).Padding(0, 2)
	// A blank row above the tabs keeps the active pill off the terminal's own
	// top chrome (tmux/term tab line) instead of butting right against it.
	tabBarStyle = lipgloss.NewStyle().Padding(1, 1, 0, 1)
)

// cardStyle is the bordered box every dashboard grid card draws inside. It
// mirrors unfocusedPane (rounded divider border, single-cell padding) so the
// grid reads as a set of panels in the same visual language as the lists.
var cardStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colDivider).Padding(0, 1)

const (
	glyphRunning     = "●"
	glyphStopped     = "○"
	glyphFailing     = "✖"
	glyphUnreachable = "⊘"
	glyphPaused      = "◐"
	glyphSuspended   = "◔"
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
