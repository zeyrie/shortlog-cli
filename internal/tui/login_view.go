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
		if m.telegramBrowserFailed {
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

func (m Model) loginWordmark() string {
	if m.width >= lipgloss.Width(smallLoginLogo)+4 && m.bodyHeight() >= 12 {
		return smallLoginLogo
	}
	return "SHORTLOG"
}

// loginView renders the provider selection screen. Shortcuts and transient
// messages live in the footer; only a notice carried over from another screen
// (such as a scheduled deletion) is shown here.
func (m Model) loginView() string {
	height := m.bodyHeight()
	logo := "SHORTLOG"
	switch {
	case m.width >= lipgloss.Width(largeLoginLogo)+4 && height >= 18:
		logo = largeLoginLogo
	case m.width >= lipgloss.Width(smallLoginLogo)+4 && height >= 12:
		logo = smallLoginLogo
	}
	heading := titleStyle.Render(logo)
	if logo != "SHORTLOG" {
		heading = renderLogo(logo)
	}
	if height >= 16 {
		heading = lipgloss.JoinVertical(lipgloss.Center, heading, "", dimStyle.Italic(true).Render("A quiet place for your notes"))
	}
	options := m.loginMenu()
	rows := []loginRow{{text: heading, center: true}, {text: "", center: true}, {text: options, center: true}}
	if m.message != "" {
		rows = append(rows, loginRow{text: "", center: true}, loginRow{text: m.loginNotice(), center: true})
	}
	content := composeRows(rows)
	if lipgloss.Height(content) > height && m.message != "" {
		content = composeRows([]loginRow{{text: options, center: true}, {text: m.loginNotice(), center: true}})
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
	height := m.bodyHeight()
	body := m.loginBody()
	prose := m.stage == telegramStage || m.stage == restoreStage
	wordmark := titleStyle.Render(m.loginWordmark())
	if m.loginWordmark() != "SHORTLOG" {
		wordmark = renderLogo(m.loginWordmark())
	}
	rows := []loginRow{{text: wordmark, center: true}, {text: "", center: true}, {text: body, center: !prose}}
	if m.message != "" {
		rows = append(rows, loginRow{text: "", center: true}, loginRow{text: m.loginNotice(), center: true})
	}
	// On short terminals drop the wordmark first, then the notice.
	if height < 16 {
		rows = rows[2:]
	}
	content := composeRows(rows)
	if lipgloss.Height(content) > height && len(rows) > 2 {
		content = composeRows(rows[2:])
	}
	if lipgloss.Height(content) > height && m.message != "" {
		content = composeRows([]loginRow{{text: body, center: !prose}})
	}
	return m.centerLogin(content)
}

// centerLogin clamps the composition to the space above the footer and
// centers it, so no part of a sign-in step can spill past an edge.
func (m Model) centerLogin(content string) string {
	width, height := max(1, m.width), m.bodyHeight()
	content = lipgloss.NewStyle().MaxWidth(width).Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// loginNotice frames a message carried over from a screen that has not moved
// to the status line yet. Those are notices the user must read, such as a
// deletion deadline, so they get a panel rather than the one-line footer.
func (m Model) loginNotice() string {
	width := min(54, max(1, m.width-4))
	if m.bodyHeight() < 16 {
		return lipgloss.NewStyle().Width(width).Foreground(lipgloss.Color("6")).Render(safeText(m.message))
	}
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6")).Render(safeText(m.message))
}

var (
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	menuRailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

// loginMenu renders the provider choices. The list model still owns selection
// and key handling; this only draws it. The selected choice gets an accent
// rail, and roomy terminals show a one-line hint under each choice.
func (m Model) loginMenu() string {
	hints := []string{"We'll email you an 8-digit code", "Approve sign-in in your browser"}
	detailed := m.width >= 40 && m.bodyHeight() >= 16
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
