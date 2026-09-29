package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"shortlog-cli/internal/api"
)

// The sign-in screen runs on its own: it needs only the sign-in API and a
// size, and reports status and the session token for its parent to act on.
func TestLoginComponentRunsStandalone(t *testing.T) {
	f := &fakeAPI{}
	l := newLogin(f, func(string) error { return nil })
	l.setSize(80, 22)
	l, _ = l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if l.step != emailStep || l.form == nil {
		t.Fatalf("1 did not open the email step: %d", l.step)
	}
	for _, r := range "a@example.com" {
		l, _ = l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	l, cmd := l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !l.busy || l.note == nil || l.note.text != "Sending email code…" {
		t.Fatalf("submit: busy %v, note %+v", l.busy, l.note)
	}
	l, _ = l.Update(cmd())
	for _, r := range "12345678" {
		l, _ = l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	l.values.zone = "" // start the profile form empty whatever the machine's TZ
	l, cmd = l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	l, _ = l.Update(cmd())
	if l.step != profileStep {
		t.Fatalf("new account did not ask for a profile: step %d", l.step)
	}
	for _, r := range "Ari" {
		l, _ = l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	l, _ = l.Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "UTC" {
		l, _ = l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	l, cmd = l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("profile not submitted: zone %q, note %+v", l.values.zone, l.note)
	}
	l, _ = l.Update(cmd())
	if l.token != "secret-token" || l.form != nil || l.values.code != "" {
		t.Fatalf("sign-in not reported or secrets kept: token %q, code %q", l.token, l.values.code)
	}
	if fresh := l.reset(); fresh.token != "" || fresh.step != menuStep || fresh.width != 80 {
		t.Fatal("reset kept the finished attempt or lost the size")
	}
}

func TestTelegramCardShowsWaitAndFallbackLink(t *testing.T) {
	address := "https://oauth.telegram.org/auth?bot_id=1&origin=https%3A%2F%2Fexample.org"
	f := &fakeAPI{telegramStart: api.TelegramStart{AttemptID: "a", PollSecret: "private-secret", AuthorizationURL: address}}
	m := New(f, &fakeStore{})
	m.stage, m.busy = loginStage, false
	m.login.openBrowser = func(string) error { return errors.New("no browser") }
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	next, _ := m.Update(cmd())
	m = next.(Model)
	view := stripStatusANSI.ReplaceAllString(m.View(), "")
	if !strings.Contains(view, "Waiting for approval") || !strings.Contains(view, "Expires in 10:00") || strings.Contains(view, "Open this link") {
		t.Fatalf("waiting card: %q", view)
	}
	next, _ = m.Update(telegramBrowserResult{attempt: "a", err: errors.New("no browser")})
	m = next.(Model)
	view = stripStatusANSI.ReplaceAllString(m.View(), "")
	if !strings.Contains(view, "Open this link") || !strings.Contains(strings.ReplaceAll(view, "\n", ""), "bot_id=1") || strings.Contains(view, "private-secret") {
		t.Fatalf("fallback link missing or secret shown: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("link overflows the terminal: %q", line)
		}
	}
	lines := strings.Split(view, "\n")
	if status := lines[len(lines)-1]; !strings.Contains(status, "Could not open the browser") || strings.ContainsAny(status, strings.Join(spinnerFrames, "")) {
		t.Fatalf("footer should keep the error but not a second spinner: %q", status)
	}
	m.login.telegramExpires = time.Now().Add(-time.Second)
	if view := stripStatusANSI.ReplaceAllString(m.View(), ""); !strings.Contains(view, "Attempt expired") {
		t.Fatal("expired attempt not shown")
	}
}

func TestRestoreCardAsksAndShowsProgress(t *testing.T) {
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	next, _ := m.Update(verifyResult{result: api.VerifyResult{Status: "restore_required", RecoveryTicket: "secret-ticket"}})
	m = next.(Model)
	view := stripStatusANSI.ReplaceAllString(m.View(), "")
	if !strings.Contains(view, "Restore this account?") || !strings.Contains(view, "Press y to restore") || strings.Contains(view, "secret-ticket") {
		t.Fatalf("restore card: %q", view)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	if view := stripStatusANSI.ReplaceAllString(m.View(), ""); !m.busy || !strings.Contains(view, "Restoring account…") {
		t.Fatal("uppercase Y did not start restoring, or no progress shown")
	}
}

func TestFooterFitsEverySignInStep(t *testing.T) {
	for step := menuStep; step <= restoreStep; step++ {
		for _, size := range []struct{ width, height int }{{80, 24}, {42, 16}, {20, 10}} {
			m := New(&fakeAPI{}, &fakeStore{})
			m.stage, m.busy = loginStage, false
			m.width, m.height = size.width, size.height
			m.login.setSize(m.width, m.bodyHeight())
			m.login.step = step
			view := stripStatusANSI.ReplaceAllString(m.View(), "")
			if lipgloss.Height(view) != m.height {
				t.Errorf("step %d %dx%d: height %d", step, size.width, size.height, lipgloss.Height(view))
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > m.width {
					t.Errorf("step %d %dx%d: overflow %q", step, size.width, size.height, line)
				}
			}
			if len(m.login.ShortHelp()) == 0 {
				t.Errorf("step %d: no shortcuts", step)
			}
		}
	}
}
