package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// loginHeading, loginOptions, and loginStatus compose the welcome screen; the
// provider steps reuse the same centered layout with a compact wordmark.
func (m Model) loginBody() string {
	switch m.stage {
	case emailStage:
		form := m.formView()
		return m.centerOver("Sign in with email", lipgloss.Width(form)) + "\n\n" + form
	case telegramStage:
		body := titleStyle.Render("Sign in with Telegram") + "\n\nApprove access in your browser. This attempt expires in about 10 minutes."
		if strings.Contains(m.message, "Could not open the browser") {
			body += "\n\nOpen manually: " + safeText(m.telegramURL)
		}
		return body
	case codeStage:
		form := m.formView()
		return m.centerOver("Code sent to "+safeText(m.email), lipgloss.Width(form)) + "\n\n" + form
	case profileStage:
		form := m.formView()
		return m.centerOver("Finish creating your account", lipgloss.Width(form)) + "\n\n" + form
	case restoreStage:
		return "This account is scheduled for deletion.\nRestoring it keeps its projects and notes, but previously signed-in devices remain signed out.\n\nRestore this account? [y/N]"
	}
	return ""
}

// centerOver centers a heading over a wider block so both share one axis.
func (m Model) centerOver(text string, width int) string {
	styled := titleStyle.Render(text)
	if width <= lipgloss.Width(styled) {
		return styled
	}
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(styled)
}

func (m Model) formView() string {
	if m.loginForm == nil {
		return ""
	}
	return cropVisible(m.loginForm.View())
}

// cropVisible trims a block to the columns that hold visible glyphs. Huh pads
// each field line out to the field width; left in place, that padding would
// be centered along with the text and push the input left of the heading.
func cropVisible(block string) string {
	lines := strings.Split(block, "\n")
	left, right := -1, 0
	for _, line := range lines {
		plain := ansi.Strip(line)
		trimmed := strings.TrimSpace(plain)
		if trimmed == "" {
			continue
		}
		start := lipgloss.Width(plain[:strings.Index(plain, trimmed)])
		if left < 0 || start < left {
			left = start
		}
		right = max(right, start+lipgloss.Width(trimmed))
	}
	if left < 0 {
		return block
	}
	for i, line := range lines {
		lines[i] = ansi.Cut(line, left, right)
	}
	return strings.Join(lines, "\n")
}

func (m Model) loginHelp() string {
	var full, short, tiny string
	switch m.stage {
	case emailStage:
		full, short, tiny = "Enter send code · Esc login options · Ctrl+C quit", "Enter send code · Esc back", "Enter continue · Esc back"
	case codeStage:
		full, short, tiny = "Enter verify · Esc change email · Ctrl+C quit", "Enter verify · Esc back", "Enter verify · Esc back"
	case profileStage:
		full, short, tiny = "Enter next/save · Tab switch field · Esc cancel", "Enter next · Tab switch · Esc back", "Enter next · Esc back"
	case telegramStage:
		full, short, tiny = "r check now · o reopen browser · Esc cancel", "r check now · o browser · Esc cancel", "r check · Esc cancel"
	}
	switch {
	case m.width >= lipgloss.Width(full):
		return full
	case m.width >= lipgloss.Width(short):
		return short
	}
	return tiny
}

func (m Model) loginWordmark() string {
	if m.width >= lipgloss.Width(smallLoginLogo)+4 && m.height >= 12 {
		return smallLoginLogo
	}
	return "SHORTLOG"
}

// loginView renders the provider selection screen.
func (m Model) loginView() string {
	logo := "SHORTLOG"
	switch {
	case m.width >= lipgloss.Width(largeLoginLogo)+4 && m.height >= 18:
		logo = largeLoginLogo
	case m.width >= lipgloss.Width(smallLoginLogo)+4 && m.height >= 12:
		logo = smallLoginLogo
	}
	heading := titleStyle.Render(logo)
	if logo != "SHORTLOG" {
		heading = renderLogo(logo)
	}
	if m.height >= 16 {
		heading = lipgloss.JoinVertical(lipgloss.Center, heading, "", dimStyle.Italic(true).Render("A quiet place for your notes"))
	}
	help := "↑/↓ or j/k select · Enter continue · 1/2 choose · q quit"
	if m.width < 59 {
		help = "j/k select · Enter · 1/2 direct · q quit"
	}
	if m.width < 40 {
		help = "Enter select · 1/2 direct"
	}
	if m.width < 27 {
		help = "1/2 choose"
	}
	options := m.loginMenu()
	rows := []loginRow{{text: heading, center: true}, {text: "", center: true}, {text: options, center: true}}
	if m.message == "" || max(1, m.height) >= 16 {
		rows = append(rows, loginRow{text: "", center: true}, loginRow{text: renderHelp(help), center: true})
	}
	if m.message != "" {
		rows = append(rows, loginRow{text: "", center: true}, loginRow{text: m.loginStatus(), center: true})
	}
	content := composeRows(rows)
	if lipgloss.Height(content) > max(1, m.height) && m.message != "" {
		content = composeRows([]loginRow{{text: options, center: true}, {text: m.loginStatus(), center: true}})
	}
	return m.centerLogin(content)
}

// loginRow is one block of a sign-in screen. Centered rows sit on the shared
// axis of the composition; left rows are prose, which reads better flush left.
type loginRow struct {
	text   string
	center bool
}

// composeRows pads every row to one column width before stacking, so every
// centered row lands on exactly the same axis.
func composeRows(rows []loginRow) string {
	column := 0
	for _, row := range rows {
		column = max(column, lipgloss.Width(row.text))
	}
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		align := lipgloss.Left
		if row.center {
			align = lipgloss.Center
		}
		// Both Style.Align and PlaceHorizontal center each line on its own.
		// Pad the block to a rectangle first so it moves as one unit and the
		// logo, menu, and form keep their internal alignment.
		block := lipgloss.NewStyle().Width(lipgloss.Width(row.text)).Render(row.text)
		parts = append(parts, lipgloss.PlaceHorizontal(column, align, block))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// loginStepView renders a provider step (email, code, profile, Telegram, or
// restoration) with the same centered composition as the welcome screen.
func (m Model) loginStepView() string {
	body := m.loginBody()
	prose := m.stage == telegramStage || m.stage == restoreStage
	wordmark := titleStyle.Render(m.loginWordmark())
	if m.loginWordmark() != "SHORTLOG" {
		wordmark = renderLogo(m.loginWordmark())
	}
	rows := []loginRow{{text: wordmark, center: true}, {text: "", center: true}, {text: body, center: !prose}}
	if help := m.loginHelp(); help != "" {
		rows = append(rows, loginRow{text: "", center: true}, loginRow{text: renderHelp(help), center: true})
	}
	if m.message != "" {
		rows = append(rows, loginRow{text: "", center: true}, loginRow{text: m.loginStatus(), center: true})
	}
	// On short terminals drop the wordmark first, then the status panel.
	if max(1, m.height) < 16 {
		rows = rows[2:]
	}
	content := composeRows(rows)
	if lipgloss.Height(content) > max(1, m.height) && len(rows) > 2 {
		content = composeRows(rows[2:])
	}
	if lipgloss.Height(content) > max(1, m.height) && m.message != "" {
		content = composeRows([]loginRow{{text: body, center: !prose}})
	}
	return m.centerLogin(content)
}

// centerLogin clamps the composition to the terminal and centers it, so no
// part of a sign-in step can spill past the right edge.
func (m Model) centerLogin(content string) string {
	width, height := max(1, m.width), max(1, m.height)
	content = lipgloss.NewStyle().MaxWidth(width).Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) loginStatus() string {
	issue := strings.Contains(m.message, "Could not") || strings.Contains(m.message, "expired") || strings.Contains(m.message, "unavailable") || strings.Contains(m.message, "unsafe")
	label, color := "STATUS", lipgloss.Color("6")
	if issue {
		label, color = "SIGN-IN ISSUE", lipgloss.Color("9")
	}
	width := min(54, max(1, m.width-4))
	if max(1, m.height) < 16 {
		return lipgloss.NewStyle().Width(width).Foreground(color).Render(label + ": " + safeText(m.message))
	}
	text := lipgloss.NewStyle().Width(max(1, width-4)).Render(safeText(m.message))
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(color).Render(
		lipgloss.JoinVertical(lipgloss.Left, lipgloss.NewStyle().Bold(true).Foreground(color).Render(label), text),
	)
}

var (
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	menuRailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

// renderHelp styles a "key action · key action" hint line: keys stand out,
// actions and separators recede. The plain text, and so its width, is unchanged.
func renderHelp(help string) string {
	parts := strings.Split(help, " · ")
	for i, part := range parts {
		key, action, found := strings.Cut(part, " ")
		if !found {
			parts[i] = helpKeyStyle.Render(key)
			continue
		}
		parts[i] = helpKeyStyle.Render(key) + dimStyle.Render(" "+action)
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}

// loginMenu renders the provider choices. The list model still owns selection
// and key handling; this only draws it. The selected choice gets an accent
// rail, and roomy terminals show a one-line hint under each choice.
func (m Model) loginMenu() string {
	hints := []string{"We'll email you an 8-digit code", "Approve sign-in in your browser"}
	detailed := m.width >= 40 && m.height >= 16
	var rows []string
	for i, item := range m.loginOptions.Items() {
		number, label, _ := strings.Cut(item.(loginOption).label, "  ")
		rail, numberStyle, labelStyle := " ", dimStyle, lipgloss.NewStyle()
		if i == m.loginOptions.Index() {
			rail, numberStyle, labelStyle = menuRailStyle.Render("▌"), titleStyle, titleStyle
		}
		if detailed && i > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, rail+" "+numberStyle.Render(number)+"  "+labelStyle.Render(label))
		if detailed && i < len(hints) {
			rows = append(rows, rail+"    "+dimStyle.Render(hints[i]))
		}
	}
	return strings.Join(rows, "\n")
}
