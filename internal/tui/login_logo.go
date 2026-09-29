package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
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

	smallLoginLogo = wordmark(1,
		[]string{"┏━┓", "┗━┓", "┗━┛"}, // S
		[]string{"╻ ╻", "┣━┫", "╹ ╹"}, // H
		[]string{"┏━┓", "┃ ┃", "┗━┛"}, // O
		[]string{"┏━┓", "┣┳┛", "╹┗╸"}, // R
		[]string{"╺┳╸", " ┃ ", " ╹ "}, // T
		[]string{"╻  ", "┃  ", "┗━╸"}, // L
		[]string{"┏━┓", "┃ ┃", "┗━┛"}, // O
		[]string{"┏━╸", "┃╺┓", "┗━┛"}, // G
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

// Gradient endpoints for the wordmark. Lip Gloss downsamples them on terminals
// without true color and drops them entirely when color is unavailable.
var (
	logoFrom = mustHex("#7DCFFF")
	logoTo   = mustHex("#BB9AF7")
)

func mustHex(hex string) colorful.Color {
	c, err := colorful.Hex(hex)
	if err != nil {
		panic(err)
	}
	return c
}

// renderLogo paints a wordmark with a left-to-right gradient. Solid blocks get
// the full color; the box-drawing shadow of the large logo is dimmed so the
// letters read as raised.
func renderLogo(logo string) string {
	width := max(1, lipgloss.Width(logo)-1)
	shadow := strings.Contains(logo, "█")
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
			c := logoFrom.BlendLuv(logoTo, float64(col)/float64(width)).Clamped()
			if shadow && r != '█' {
				c = c.BlendLab(colorful.Color{}, 0.55).Clamped()
			}
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex())).Render(string(r)))
		}
	}
	return b.String()
}
