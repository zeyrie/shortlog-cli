package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"shortlog-cli/internal/api"
)

// The sign-in screen runs on its own: it needs only the sign-in API and a
// size, and reports status and the session token for its parent to act on.
func TestLoginComponentRunsStandalone(t *testing.T) {
	f := &fakeAPI{}
	l := newLogin(f, func(string) error { return nil })
	l.setSize(80, 22)
	l, _ = l.Update(runeKey('1'))
	if l.step != emailStep || l.form == nil {
		t.Fatalf("1 did not open the email step: %d", l.step)
	}
	for _, r := range "a@example.com" {
		l, _ = l.Update(runeKey(r))
	}
	l, cmd := l.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !l.busy || l.note == nil || l.note.text != "Sending email code…" {
		t.Fatalf("submit: busy %v, note %+v", l.busy, l.note)
	}
	l, _ = l.Update(cmd())
	for _, r := range "12345678" {
		l, _ = l.Update(runeKey(r))
	}
	l.values.zone = "" // start the profile form empty whatever the machine's TZ
	l, cmd = l.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	l, _ = l.Update(cmd())
	if l.step != profileStep {
		t.Fatalf("new account did not ask for a profile: step %d", l.step)
	}
	for _, r := range "Ari" {
		l, _ = l.Update(runeKey(r))
	}
	l, _ = l.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	for _, r := range "UTC" {
		l, _ = l.Update(runeKey(r))
	}
	l, cmd = l.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	m, cmd := press(m, runeKey('2'))
	next, _ := m.Update(cmd())
	m = next.(Model)
	view := stripStatusANSI.ReplaceAllString(m.View().Content, "")
	if !strings.Contains(view, "Waiting for approval") || !strings.Contains(view, "Expires in 10:00") || strings.Contains(view, "Open this link") {
		t.Fatalf("waiting card: %q", view)
	}
	next, _ = m.Update(telegramBrowserResult{attempt: "a", err: errors.New("no browser")})
	m = next.(Model)
	view = stripStatusANSI.ReplaceAllString(m.View().Content, "")
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
	if view := stripStatusANSI.ReplaceAllString(m.View().Content, ""); !strings.Contains(view, "Attempt expired") {
		t.Fatal("expired attempt not shown")
	}
}

func TestRestoreCardAsksAndShowsProgress(t *testing.T) {
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	next, _ := m.Update(verifyResult{result: api.VerifyResult{Status: "restore_required", RecoveryTicket: "secret-ticket"}})
	m = next.(Model)
	view := stripStatusANSI.ReplaceAllString(m.View().Content, "")
	if !strings.Contains(view, "Restore this account?") || !strings.Contains(view, "Press y to restore") || strings.Contains(view, "secret-ticket") {
		t.Fatalf("restore card: %q", view)
	}
	m, _ = press(m, runeKey('Y'))
	if view := stripStatusANSI.ReplaceAllString(m.View().Content, ""); !m.busy || !strings.Contains(view, "Restoring account…") {
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
			view := stripStatusANSI.ReplaceAllString(m.View().Content, "")
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

func TestThemeFollowsTerminalBackground(t *testing.T) {
	t.Cleanup(func() { setTheme(true) })
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	darkLogo := renderLogo(smallLoginLogo)
	next, _ := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	m = next.(Model)
	if darkTheme || titleStyle.GetForeground() != lipgloss.Blue || renderLogo(smallLoginLogo) == darkLogo {
		t.Fatal("light background did not restyle the screen")
	}
	if m.login.background == nil {
		t.Fatal("sign-in screen did not keep the background for later forms")
	}
	m.login.showForm(emailStep)
	if view := m.View().Content; !strings.Contains(stripStatusANSI.ReplaceAllString(view, ""), "Email address") {
		t.Fatal("form did not render after the theme changed")
	}
	next, _ = m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#101010")})
	if m = next.(Model); !darkTheme || titleStyle.GetForeground() != lipgloss.Cyan {
		t.Fatal("dark background did not restore the dark styles")
	}
}

func TestPastedCodeKeepsOnlyDigits(t *testing.T) {
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	m.login.challenge = "challenge"
	m.login.showForm(codeStep)
	next, _ := m.Update(tea.PasteMsg{Content: " 1234-5678\n"})
	m = next.(Model)
	if m.login.values.code != "12345678" {
		t.Fatalf("pasted code = %q", m.login.values.code)
	}
	if cleanPaste(emailStep, "  a@example.com\n") != "a@example.com" {
		t.Fatal("pasted email kept its surrounding whitespace")
	}
}

func TestCopyTelegramLink(t *testing.T) {
	var copied string
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	m.login.step, m.login.telegramLogin, m.login.telegramURL = telegramStep, true, "https://oauth.telegram.org/auth?bot_id=1"
	m.login.copyText = func(s string) error { copied = s; return nil }
	m, cmd := press(m, runeKey('c'))
	next, _ := m.Update(cmd())
	m = next.(Model)
	if copied != m.login.telegramURL || m.status.current.text != "Link copied to the clipboard." {
		t.Fatalf("copy: got %q, status %q", copied, m.status.current.text)
	}
	m.login.copyText = func(string) error { return errors.New("no clipboard") }
	m, cmd = press(m, runeKey('c'))
	next, fallback := m.Update(cmd())
	m = next.(Model)
	if fallback == nil || !strings.Contains(m.status.current.text, "terminal") {
		t.Fatal("no terminal clipboard fallback without a system clipboard")
	}
}

func TestViewDeclaresTitleAndProgress(t *testing.T) {
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	if v := m.View(); !v.AltScreen || v.WindowTitle != "Shortlog · Sign in" || v.ProgressBar != nil {
		t.Fatalf("idle sign-in view: title %q, progress %v", v.WindowTitle, v.ProgressBar)
	}
	m, _ = press(m, runeKey('2'))
	if v := m.View(); v.ProgressBar == nil || v.ProgressBar.State != tea.ProgressBarIndeterminate {
		t.Fatal("no progress indicator while Telegram sign-in starts")
	}
	m.stage, m.busy = inboxStage, false
	if v := m.View(); v.WindowTitle != "Shortlog" || v.ProgressBar != nil {
		t.Fatalf("inbox view: title %q", v.WindowTitle)
	}
}
