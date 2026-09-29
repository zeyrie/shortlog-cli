package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Shared styles. setTheme rebuilds them when the terminal reports its
// background colour; until then they assume a dark background, which most
// terminals have. They are package-level so the small render helpers every
// screen shares (panel frames, list rows, load states) can use them without a
// styles value threaded through each call. Only the root model's Update calls
// setTheme, on Bubble Tea's single update goroutine; tests that change the
// theme restore it.
var (
	titleStyle    lipgloss.Style
	dimStyle      lipgloss.Style
	errStyle      lipgloss.Style
	warnStyle     lipgloss.Style
	helpKeyStyle  lipgloss.Style
	menuRailStyle lipgloss.Style

	accentColor color.Color
	// logoStops are the ends of the wordmark gradient.
	logoStops [2]color.Color
	// darkTheme records the background the styles were built for.
	darkTheme bool
)

func init() { setTheme(true) }

// setTheme builds the styles for a dark or light background. Named ANSI
// colours follow the user's terminal theme; the few hex colours (the logo
// gradient, the light-mode warning) are picked to stay readable on both.
func setTheme(isDark bool) {
	pick := lipgloss.LightDark(isDark)
	darkTheme = isDark
	accentColor = pick(lipgloss.Blue, lipgloss.Cyan)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	errStyle = lipgloss.NewStyle().Foreground(pick(lipgloss.Red, lipgloss.BrightRed))
	warnStyle = lipgloss.NewStyle().Foreground(pick(lipgloss.Color("#9A6700"), lipgloss.Yellow))
	// Keys in hints: bright on dark terminals, and full-strength text on
	// light ones, where the default light grey would all but vanish.
	helpKeyStyle = lipgloss.NewStyle().Foreground(pick(lipgloss.Black, lipgloss.White))
	menuRailStyle = lipgloss.NewStyle().Foreground(accentColor)
	logoStops = [2]color.Color{
		pick(lipgloss.Color("#0369A1"), lipgloss.Color("#7DCFFF")),
		pick(lipgloss.Color("#7C3AED"), lipgloss.Color("#BB9AF7")),
	}
}
