package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"shortlog-cli/internal/api"
)

// accountPanel is panel [1]: one line naming who is signed in and where.
// Focusing it shows the account page in the main panel, which holds
// everything about the account: profile, sessions, sign-out, and deletion.
type accountPanel struct {
	account     api.Account
	sessions    []api.Session
	sessionsErr string
	server      string     // host, for the panel line
	origin      string     // full address, for the page
	list        scrollList // the selected session
}

func (a accountPanel) selectedSession() (api.Session, bool) {
	if len(a.sessions) == 0 {
		return api.Session{}, false
	}
	return a.sessions[min(a.list.cursor, len(a.sessions)-1)], true
}

func (a accountPanel) View(width, height int, focused bool, state loadState, spinner string) string {
	line := " " + titleStyle.Render(safeText(a.account.Username))
	switch {
	case !state.loaded && state.loading:
		line = " " + spinner + " " + dimStyle.Render("Loading account…")
	case !state.loaded && state.err != "":
		line = " " + errStyle.Render("✗ Not loaded") + dimStyle.Render(" · r to retry")
	}
	if a.server != "" && state.loaded {
		line += dimStyle.Render(" · " + a.server)
	}
	return frame{number: 1, title: "Account", focused: focused}.render(line, width, height)
}

// page is the account page shown in the main panel. For now it only shows the
// account; editing, revoking sessions, and signing out arrive with the
// account step of the workspace.
func (a accountPanel) page(width, height int, focused bool, state loadState, spinner string, now time.Time) string {
	f := frame{number: 0, title: "Account", focused: focused}
	switch {
	case !state.loaded && state.loading:
		return f.render(" "+spinner+" "+dimStyle.Render("Loading your account…"), width, height)
	case !state.loaded && state.err != "":
		return f.render(" "+errStyle.Render("✗ "+state.err)+"\n\n "+dimStyle.Render("Press r to retry."), width, height)
	}
	label := lipgloss.NewStyle().Width(14).Foreground(lipgloss.BrightBlack)
	section := lipgloss.NewStyle().Bold(true)
	hint := func(k, action string) string { return "  " + helpKeyStyle.Render(k) + dimStyle.Render(" "+action) }
	var b strings.Builder
	b.WriteString(" " + section.Render("Profile") + hint("e", "edit") + "\n")
	b.WriteString("  " + label.Render("Name") + safeText(a.account.Username) + "\n")
	b.WriteString("  " + label.Render("Time zone") + safeText(a.account.TimeZone) + "\n")
	if !a.account.CreatedAt.IsZero() {
		b.WriteString("  " + label.Render("Member since") + a.account.CreatedAt.Local().Format("Jan 2006") + "\n")
	}
	b.WriteString("\n " + section.Render("Sessions") + hint("x", "revoke") + hint("a", "revoke all") + "\n")
	if a.sessionsErr != "" {
		b.WriteString("  " + errStyle.Render("✗ "+a.sessionsErr) + "\n")
	}
	for i, s := range a.sessions {
		name := sessionName(s)
		if s.Current {
			name += dimStyle.Render(" (this device)")
		}
		row := lipgloss.NewStyle().Width(max(width-26, 12)).Render(name) + dimStyle.Render(lastUsed(s.LastUsedAt, now))
		b.WriteString(listRow(row, i == a.list.cursor, focused, width-2) + "\n")
	}
	if a.origin != "" {
		b.WriteString("\n " + section.Render("Server") + hint("s", "change") + "\n")
		b.WriteString("  " + safeText(a.origin) + "\n")
	}
	b.WriteString("\n " + section.Render("Sign out of this device") + hint("l", "sign out") + "\n")
	b.WriteString(" " + errStyle.Render("Delete account") + hint("D", "delete…"))
	return f.render(b.String(), width, height)
}

// lastUsed says how long ago a session was used, in the coarsest unit that
// still reads naturally.
func lastUsed(t, now time.Time) string {
	switch d := now.Sub(t); {
	case d < time.Minute:
		return "used just now"
	case d < time.Hour:
		return fmt.Sprintf("used %d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("used %d h ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "used yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("used %d days ago", int(d.Hours()/24))
	default:
		return "used " + t.Local().Format("Jan 02")
	}
}
