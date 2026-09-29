package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// loginBody renders the part of a sign-in step below the shared header.
func (m Model) loginBody() string {
	switch m.stage {
	case emailStage:
		return m.formBlock("Sign in with email")
	case telegramStage:
		body := titleStyle.Render("Sign in with Telegram") + "\n\nApprove access in your browser. This attempt expires in about 10 minutes."
		if m.telegramBrowserFailed {
			body += "\n\nOpen manually: " + safeText(m.telegramURL)
		}
		return body
	case codeStage:
		return m.formBlock("Code sent to " + safeText(m.email))
	case profileStage:
		return m.formBlock("Finish creating your account")
	case restoreStage:
		return "This account is scheduled for deletion.\nRestoring it keeps its projects and notes, but previously signed-in devices remain signed out.\n\nRestore this account? [y/N]"
	}
	return ""
}

// formBlock stacks a heading over the step's form. The form has a fixed width
// set by loginFieldWidth, so the heading is centered over the field itself and
// neither moves while the user types.
func (m Model) formBlock(heading string) string {
	if m.loginForm == nil {
		return titleStyle.Render(heading)
	}
	width := loginFieldWidth(m.width, m.loginBoxed())
	// A fixed width also wraps anything Huh renders past the field, such as a
	// long validation error, so it cannot widen the block and shift the card.
	card := lipgloss.NewStyle().Width(width)
	if m.loginBoxed() {
		card = card.Width(width-2).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6"))
	}
	content := strings.TrimRight(m.loginForm.View(), "\n")
	if m.busy {
		// One row, so the card grows by as little as possible and the header
		// keeps its place even on small terminals.
		content += "\n" + m.loginLoader(loginFormWidth(m.width, m.loginBoxed()))
	}
	form := card.Render(content)
	// Center whichever is narrower under the other, so the card stays on the
	// screen's axis even when a heading (a long address) is wider than it.
	title := titleStyle.Render(ansi.Truncate(heading, max(1, m.width-4), "…"))
	if lipgloss.Width(title) < lipgloss.Width(form) {
		title = lipgloss.PlaceHorizontal(lipgloss.Width(form), lipgloss.Center, title)
	} else {
		form = lipgloss.PlaceHorizontal(lipgloss.Width(title), lipgloss.Center, form)
	}
	return title + "\n" + form
}

// loginLoader is the in-card progress line shown under the submitted fields
// while their request runs. It shares the status bar's spinner clock.
func (m Model) loginLoader(width int) string {
	text := "Finishing sign-in…"
	switch {
	case m.stage == emailStage:
		text = "Sending a code to " + safeText(strings.TrimSpace(m.loginValues.email)) + "…"
	case m.stage == codeStage:
		text = "Verifying code…"
	case m.telegramLogin:
		text = "Checking Telegram approval…"
	}
	spinner := menuRailStyle.Render(spinnerFrames[m.status.frame%len(spinnerFrames)])
	return spinner + " " + dimStyle.Render(ansi.Truncate(text, max(1, width-2), "…"))
}

// loginView renders the provider selection screen. Shortcuts and transient
// messages live in the footer; only a notice carried over from another screen
// (such as a scheduled deletion) is shown here.
func (m Model) loginView() string {
	rows := []loginRow{{text: m.loginMenu(), center: true}}
	if m.message != "" {
		rows = append(rows, loginRow{text: "", center: true}, loginRow{text: m.loginNotice(), center: true})
	}
	return m.loginScreen(rows)
}

// loginStepView renders a provider step (email, code, profile, Telegram, or
// restoration) under the same header as the welcome screen.
func (m Model) loginStepView() string {
	prose := m.stage == telegramStage || m.stage == restoreStage
	rows := []loginRow{{text: m.loginBody(), center: !prose}}
	if m.message != "" {
		rows = append(rows, loginRow{text: "", center: true}, loginRow{text: m.loginNotice(), center: true})
	}
	return m.loginScreen(rows)
}

// loginHeaders lists the header variants from roomiest to tightest: each logo
// with and without its tagline, the plain wordmark, then nothing.
func (m Model) loginHeaders() []string {
	tagline := dimStyle.Italic(true).Render("A quiet place for your notes")
	var headers []string
	for _, logo := range []string{largeLoginLogo, smallLoginLogo} {
		if m.width >= lipgloss.Width(logo)+4 {
			art := renderLogo(logo)
			headers = append(headers, lipgloss.JoinVertical(lipgloss.Center, art, "", tagline), art)
		}
	}
	return append(headers, titleStyle.Render("SHORTLOG"), "")
}

// loginScreen places the shared header above a screen's rows. The header is
// anchored where the welcome screen puts it, so moving between sign-in steps
// swaps only what is below the tagline. A taller step lifts the header just
// enough to fit; if it still does not fit, the header steps down a size, and
// the rows after the first (a notice) are dropped as a last resort.
func (m Model) loginScreen(rows []loginRow) string {
	width, height := max(1, m.width), m.bodyHeight()
	block := ""
	for _, content := range [][]loginRow{rows, rows[:1]} {
		for _, header := range m.loginHeaders() {
			stack := content
			if header != "" {
				stack = append([]loginRow{{text: header, center: true}, {text: "", center: true}}, content...)
			}
			block = composeRows(stack)
			if lipgloss.Height(block) > height {
				continue
			}
			top := (height - lipgloss.Height(block)) / 2
			if header != "" {
				anchor := composeRows([]loginRow{{text: header, center: true}, {text: "", center: true}, {text: m.loginMenu(), center: true}})
				top = min(max(0, (height-lipgloss.Height(anchor))/2), height-lipgloss.Height(block))
			}
			return m.placeLogin(block, top, width, height)
		}
	}
	return m.placeLogin(block, 0, width, height)
}

// placeLogin centers a composed block horizontally and puts it top rows down,
// clamped to the space above the footer.
func (m Model) placeLogin(block string, top, width, height int) string {
	block = lipgloss.NewStyle().MaxWidth(width).Render(block)
	block = strings.Repeat("\n", max(0, top)) + block
	return lipgloss.NewStyle().MaxHeight(height).Render(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, block))
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
