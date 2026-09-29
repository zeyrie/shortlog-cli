package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// View renders the sign-in screen into the space above the footer. notice is
// a message carried over from another screen, such as a deletion deadline,
// and spinner is the status bar's current spinner frame.
func (l loginModel) View(notice, spinner string) string {
	var rows []loginRow
	switch l.step {
	case menuStep:
		rows = []loginRow{{text: l.menu(), center: true}}
	case emailStep:
		rows = []loginRow{{text: l.formBlock("Sign in with email", spinner), center: true}}
	case codeStep:
		rows = []loginRow{{text: l.formBlock("Code sent to "+safeText(l.email), spinner), center: true}}
	case profileStep:
		rows = []loginRow{{text: l.formBlock("Finish creating your account", spinner), center: true}}
	case telegramStep:
		rows = []loginRow{{text: l.telegramBlock(spinner), center: true}}
	case restoreStep:
		rows = []loginRow{{text: l.restoreBlock(spinner), center: true}}
	}
	if notice != "" {
		rows = append(rows, loginRow{text: "", center: true}, loginRow{text: l.notice(notice), center: true})
	}
	return l.screen(rows)
}

// card frames a step's content. Every step shares its width, so the card
// keeps its place from one step to the next. The fixed width also wraps
// anything longer, such as a validation error, instead of widening the card.
func (l loginModel) card(content string) string {
	width := loginFieldWidth(l.width, l.boxed())
	style := lipgloss.NewStyle().Width(width)
	if l.boxed() {
		style = style.Width(width-2).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6"))
	}
	return style.Render(content)
}

// titled stacks a heading over a card. Whichever is narrower is centered under
// the other, so the card stays on the screen's axis even when the heading (a
// long address) is wider than it.
func (l loginModel) titled(heading, card string) string {
	title := titleStyle.Render(ansi.Truncate(heading, max(1, l.width-4), "…"))
	if lipgloss.Width(title) < lipgloss.Width(card) {
		title = lipgloss.PlaceHorizontal(lipgloss.Width(card), lipgloss.Center, title)
	} else {
		card = lipgloss.PlaceHorizontal(lipgloss.Width(title), lipgloss.Center, card)
	}
	return title + "\n" + card
}

// formBlock shows a form step. While its request runs, a loader row sits under
// the submitted fields: one row, so the header keeps its place.
func (l loginModel) formBlock(heading, spinner string) string {
	if l.form == nil {
		return titleStyle.Render(heading)
	}
	content := strings.TrimRight(l.form.View(), "\n")
	if l.busy {
		text := "Finishing sign-in…"
		switch {
		case l.step == emailStep:
			text = "Sending a code to " + safeText(strings.TrimSpace(l.values.email)) + "…"
		case l.step == codeStep:
			text = "Verifying code…"
		case l.telegramLogin:
			text = "Checking Telegram approval…"
		}
		content += "\n" + l.loader(spinner, text)
	}
	return l.titled(heading, l.card(content))
}

// loader is a progress row: the shared spinner and what is happening.
func (l loginModel) loader(spinner, text string) string {
	return spinner + " " + dimStyle.Render(ansi.Truncate(text, max(1, loginFormWidth(l.width, l.boxed())-2), "…"))
}

// telegramBlock shows the approval wait: a live spinner, the time left, and,
// if the browser did not open, the link to open by hand. The link sits below
// the card so copying it does not pick up the card's border.
func (l loginModel) telegramBlock(spinner string) string {
	status := "Waiting for approval in your browser"
	if l.busy {
		status = "Checking approval…"
	}
	remaining := "Attempt expired"
	if left := time.Until(l.telegramExpires).Round(time.Second); left > 0 {
		remaining = fmt.Sprintf("Expires in %d:%02d", int(left.Minutes()), int(left.Seconds())%60)
	}
	block := l.titled("Sign in with Telegram", l.card(l.loader(spinner, status)+"\n  "+dimStyle.Render(remaining)))
	if l.telegramBrowserFailed {
		width := loginFieldWidth(l.width, l.boxed())
		link := ansi.Hardwrap(safeText(l.telegramURL), width, true)
		block += "\n\n" + lipgloss.NewStyle().Width(width).Render(dimStyle.Render("Open this link to approve:")+"\n"+link)
		block = composeRows([]loginRow{{text: block, center: true}})
	}
	return block
}

// restoreBlock asks before restoring an account scheduled for deletion. The
// ticket that authorizes restoration is never shown.
func (l loginModel) restoreBlock(spinner string) string {
	text := "This account is scheduled for deletion. Restoring it keeps its projects and notes; devices that were signed out stay signed out."
	prompt := dimStyle.Render("Press ") + helpKeyStyle.Render("y") + dimStyle.Render(" to restore it, or ") + helpKeyStyle.Render("n") + dimStyle.Render(" to keep the deletion.")
	if l.busy {
		prompt = l.loader(spinner, "Restoring account…")
	}
	return l.titled("Restore this account?", l.card(text+"\n\n"+prompt))
}

// headers lists the header variants from roomiest to tightest: each logo
// with and without its tagline, the plain wordmark, then nothing.
func (l loginModel) headers() []string {
	tagline := dimStyle.Italic(true).Render("A quiet place for your notes")
	var headers []string
	for _, logo := range []string{largeLoginLogo, smallLoginLogo} {
		if l.width >= lipgloss.Width(logo)+4 {
			art := renderLogo(logo)
			headers = append(headers, lipgloss.JoinVertical(lipgloss.Center, art, "", tagline), art)
		}
	}
	return append(headers, titleStyle.Render("SHORTLOG"), "")
}

// screen places the shared header above a screen's rows. The header is
// anchored where the welcome screen puts it, so moving between sign-in steps
// swaps only what is below the tagline. A taller step lifts the header just
// enough to fit; if it still does not fit, the header steps down a size, and
// the rows after the first (a notice) are dropped as a last resort.
func (l loginModel) screen(rows []loginRow) string {
	width, height := max(1, l.width), max(1, l.height)
	block := ""
	for _, content := range [][]loginRow{rows, rows[:1]} {
		for _, header := range l.headers() {
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
				anchor := composeRows([]loginRow{{text: header, center: true}, {text: "", center: true}, {text: l.menu(), center: true}})
				top = min(max(0, (height-lipgloss.Height(anchor))/2), height-lipgloss.Height(block))
			}
			return placeLogin(block, top, width, height)
		}
	}
	return placeLogin(block, 0, width, height)
}

// placeLogin centers a composed block horizontally and puts it top rows down,
// clamped to the space above the footer.
func placeLogin(block string, top, width, height int) string {
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

// notice frames a message carried over from a screen that has not moved to
// the status line yet. Those are notices the user must read, such as a
// deletion deadline, so they get a panel rather than the one-line footer.
func (l loginModel) notice(text string) string {
	width := min(54, max(1, l.width-4))
	if l.height < 16 {
		return lipgloss.NewStyle().Width(width).Foreground(lipgloss.Color("6")).Render(safeText(text))
	}
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6")).Render(safeText(text))
}

var (
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	menuRailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

// menu renders the provider choices. The list model still owns selection
// and key handling; this only draws it. The selected choice gets an accent
// rail, and roomy terminals show a one-line hint under each choice.
func (l loginModel) menu() string {
	hints := []string{"We'll email you an 8-digit code", "Approve sign-in in your browser"}
	detailed := l.width >= 40 && l.height >= 16
	var rows []string
	for i, item := range l.options.Items() {
		number, label, _ := strings.Cut(item.(loginOption).label, "  ")
		rail, numberStyle, labelStyle := " ", dimStyle, lipgloss.NewStyle()
		if i == l.options.Index() {
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
