package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Wordmarks are assembled from per-letter glyphs so every row is exactly as
// wide as its neighbours; layout and responsive sizing are handled by Lip Gloss.
var (
	largeLoginLogo = wordmark(0, []string{
		"███████╗", "██╔════╝", "███████╗", "╚════██║", "███████║", "╚══════╝", // S
	}, []string{
		"██╗  ██╗", "██║  ██║", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝", // H
	}, []string{
		" ██████╗ ", "██╔═══██╗", "██║   ██║", "██║   ██║", "╚██████╔╝", " ╚═════╝ ", // O
	}, []string{
		"██████╗ ", "██╔══██╗", "██████╔╝", "██╔══██╗", "██║  ██║", "╚═╝  ╚═╝", // R
	}, []string{
		"████████╗", "╚══██╔══╝", "   ██║   ", "   ██║   ", "   ██║   ", "   ╚═╝   ", // T
	}, []string{
		"██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝", // L
	}, []string{
		" ██████╗ ", "██╔═══██╗", "██║   ██║", "██║   ██║", "╚██████╔╝", " ╚═════╝ ", // O
	}, []string{
		" ██████╗ ", "██╔════╝ ", "██║  ███╗", "██║   ██║", "╚██████╔╝", " ╚═════╝ ", // G
	})

	// Half blocks give the compact logo the same solid weight as the large
	// one in two rows: each cell holds two vertical pixels.
	smallLoginLogo = wordmark(1,
		[]string{"█▀▀", "▄▄█"}, // S
		[]string{"█ █", "█▀█"}, // H
		[]string{"█▀█", "█▄█"}, // O
		[]string{"█▀█", "█▀▄"}, // R
		[]string{"▀█▀", " █ "}, // T
		[]string{"█  ", "█▄▄"}, // L
		[]string{"█▀█", "█▄█"}, // O
		[]string{"█▀▀", "█▄█"}, // G
	)
)

// wordmark joins letter glyphs row by row with gap spaces between letters.
// Rows keep their trailing spaces so the logo stays a rectangle when centered.
func wordmark(gap int, letters ...[]string) string {
	rows := make([]string, len(letters[0]))
	for i := range rows {
		parts := make([]string, len(letters))
		for j, letter := range letters {
			parts[j] = letter[i]
		}
		rows[i] = strings.Join(parts, strings.Repeat(" ", gap))
	}
	return strings.Join(rows, "\n")
}

// renderLogo paints a wordmark with a left-to-right gradient. Block glyphs get
// the full colour; the box-drawing shadow of the large logo is pushed toward
// the background (darker on dark terminals, lighter on light ones) so the
// letters read as raised.
func renderLogo(logo string) string {
	colors := lipgloss.Blend1D(max(2, lipgloss.Width(logo)), logoStops[0], logoStops[1])
	var b strings.Builder
	for i, line := range strings.Split(logo, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		for col, r := range []rune(line) {
			if r == ' ' {
				b.WriteRune(r)
				continue
			}
			c := colors[min(col, len(colors)-1)]
			if r >= '─' && r <= '╿' { // box drawing: the large logo's shadow
				if darkTheme {
					c = lipgloss.Darken(c, 0.55)
				} else {
					c = lipgloss.Lighten(c, 0.55)
				}
			}
			b.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(r)))
		}
	}
	return b.String()
}
