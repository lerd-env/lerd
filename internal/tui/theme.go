package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/geodro/lerd/internal/feedback"
)

// lipgloss v2 dropped AdaptiveColor; default to the dark variant (unchanged look
// on a dark terminal) and pick the light one only when the background is light.
var darkBackground = true

func adaptive(light, dark string) color.Color {
	if darkBackground {
		return lipgloss.Color(dark)
	}
	return lipgloss.Color(light)
}

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
	// onAccent is the foreground for a label sitting on an accent-filled
	// background (active tab pill, key chip, log search match). It follows the
	// terminal's light/dark mode so the text stays legible on the themed accent
	// instead of assuming a bright one, which black text would need.
	onAccent = adaptive("#f5f5f5", "#0b0b0b")
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
			Foreground(onAccent).
			Bold(true).
			Padding(0, 1)
	keyChipLabelStyle = lipgloss.NewStyle().Foreground(colDim)
)

// spinnerFrames cycles through Braille spinner glyphs (the same set
// charm/bubbletea uses in its spinner package). Animated by the existing
// tickCmd — every snapshotMsg also bumps the spinner phase indirectly via
// time.Now sampling in renderSpinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// applyBackground re-derives the adaptive palette (the two-tone greys and the
// on-accent foreground) and every style built from it, for the given terminal
// background. lipgloss v2 has no render-time adaptive colour, so the TUI calls
// this once bubbletea reports the background; the package defaults to the dark
// palette above, so a dark terminal never needs it. It must mirror the adaptive
// style definitions here and in modal.go / logs_render.go.
func applyBackground(dark bool) {
	darkBackground = dark
	feedback.SetDarkBackground(dark)
	colDim = feedback.ColDim
	colDivider = feedback.ColDivider
	colStopped = feedback.ColStopped
	onAccent = adaptive("#f5f5f5", "#0b0b0b")

	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(colDim)
	dimStyle = lipgloss.NewStyle().Foreground(colDim)
	unfocusedPane = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colDivider).Padding(0, 1)
	stoppedStyle = lipgloss.NewStyle().Foreground(colStopped)
	helpStyle = lipgloss.NewStyle().Foreground(colDim)
	keyChipStyle = lipgloss.NewStyle().Background(colAccent).Foreground(onAccent).Bold(true).Padding(0, 1)
	keyChipLabelStyle = lipgloss.NewStyle().Foreground(colDim)

	// Adaptive styles that live next to their features rather than in the palette.
	modalFooterStyle = lipgloss.NewStyle().Foreground(colDim)
	logMatchStyle = lipgloss.NewStyle().Background(colAccent).Foreground(onAccent)
	logDimNonMatch = lipgloss.NewStyle().Foreground(colDim)
}
