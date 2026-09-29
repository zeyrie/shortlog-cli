package tui

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"shortlog-cli/internal/api"
)

func TestLoginMenuSelectionAndNavigation(t *testing.T) {
	f := &fakeAPI{telegramStartErr: errors.New("offline")}
	m := New(f, &fakeStore{})
	next, _ := m.Update(loadedSession{})
	m = next.(Model)
	if m.stage != loginStage || !strings.Contains(stripStatusANSI.ReplaceAllString(m.View().Content, ""), "1  Continue with email") {
		t.Fatal("empty credential did not open login menu")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.login.options.Index() != 1 {
		t.Fatal("arrow did not select Telegram")
	}
	m, _ = press(m, runeKey('k'))
	if m.login.options.Index() != 0 {
		t.Fatal("k did not move selection up")
	}
	m, _ = press(m, runeKey('j'))
	if m.login.options.Index() != 1 {
		t.Fatal("j did not move selection down")
	}
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.busy || cmd == nil {
		t.Fatal("Enter did not start Telegram")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.login.step != menuStep || !strings.Contains(m.View().Content, "Could not start Telegram") {
		t.Fatal("failed start did not return to menu")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.login.step != emailStep || m.login.form == nil {
		t.Fatal("Enter did not open the email form")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.login.step != menuStep {
		t.Fatal("Esc did not return to login options")
	}
	m, _ = press(m, runeKey('1'))
	if m.login.step != emailStep || m.login.options.Index() != 0 {
		t.Fatal("1 did not open email directly")
	}
}

func TestLoginDirectTelegramAndCancel(t *testing.T) {
	f := &fakeAPI{telegramStart: api.TelegramStart{AttemptID: "attempt", PollSecret: "private", AuthorizationURL: "https://oauth.telegram.org/auth"}}
	m := New(f, &fakeStore{})
	m.stage, m.busy = loginStage, false
	m.login.openBrowser = func(string) error { return nil }
	m, cmd := press(m, runeKey('2'))
	if cmd == nil || m.login.options.Index() != 1 || !m.busy {
		t.Fatal("2 did not start Telegram directly")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.login.step != telegramStep {
		t.Fatal("Telegram approval not opened")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.login.step != menuStep || m.login.telegramSecret != "" || m.login.options.Index() != 1 {
		t.Fatal("Telegram cancel did not return to menu")
	}
}

func TestLoginLogoResponsiveAndCentered(t *testing.T) {
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	large, small := largeLoginLogo, smallLoginLogo
	stripANSI := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	if lipgloss.Width(large) <= lipgloss.Width(small) {
		t.Fatal("large ASCII art is not larger")
	}
	for _, size := range []struct {
		width, height int
		logo          string
	}{
		{80, 24, large},
		{42, 16, small},
		{28, 14, "SHORTLOG"},
	} {
		m.width, m.height = size.width, size.height
		m.login.setSize(m.width, m.bodyHeight())
		view := stripANSI.ReplaceAllString(m.View().Content, "")
		for _, row := range strings.Split(size.logo, "\n") {
			if !strings.Contains(view, strings.TrimRight(row, " ")) {
				t.Errorf("%dx%d: wrong logo size", size.width, size.height)
				break
			}
		}
		if height := lipgloss.Height(view); height != size.height {
			t.Errorf("%dx%d: expected full-height UI, got %d", size.width, size.height, height)
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > size.width {
				t.Errorf("%dx%d: wrapped beyond terminal width", size.width, size.height)
			}
		}
	}
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(Model)
	view := stripANSI.ReplaceAllString(m.View().Content, "")
	first := strings.Index(view, strings.Split(large, "\n")[0])
	if first < 0 || strings.Count(view[:first], "\n") < 2 {
		t.Fatal("logo is not vertically centered")
	}
	m.login.options.Select(1)
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 42, Height: 16})
	m = resized.(Model)
	if m.login.options.Index() != 1 {
		t.Fatal("resizing reset the selected provider")
	}
}

func TestLoginSmallTerminalWithStatus(t *testing.T) {
	for _, size := range []struct{ width, height int }{{30, 12}, {20, 10}} {
		m := New(&fakeAPI{}, &fakeStore{})
		m.stage, m.busy = loginStage, false
		m.width, m.height = size.width, size.height
		m.message = "Telegram sign-in unavailable. Configure Telegram on the server or use email."
		m.login.setSize(m.width, m.bodyHeight())
		view := m.View().Content
		if lipgloss.Height(view) != m.height {
			t.Errorf("%dx%d: login screen exceeds terminal height: %d", m.width, m.height, lipgloss.Height(view))
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > m.width {
				t.Errorf("%dx%d: login screen exceeds terminal width", m.width, m.height)
			}
		}
	}
}

func TestLoginOptionsAlignedAtBothSelections(t *testing.T) {
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	stripANSI := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for _, width := range []int{80, 42, 30} {
		m.width = width
		m.height = 24
		m.login.setSize(m.width, m.bodyHeight())
		for selected := 0; selected < 2; selected++ {
			m.login.options.Select(selected)
			rows := strings.Split(stripANSI.ReplaceAllString(m.View().Content, ""), "\n")
			var columns []int
			emailRow, telegramRow := -1, -1
			for rowIndex, row := range rows {
				if index := strings.Index(row, "1  "); index >= 0 && (strings.Contains(row, "Email") || strings.Contains(row, "email")) {
					emailRow = rowIndex
					columns = append(columns, lipgloss.Width(row[:index]))
				}
				if index := strings.Index(row, "2  "); index >= 0 && strings.Contains(row, "Telegram") {
					telegramRow = rowIndex
					columns = append(columns, lipgloss.Width(row[:index]))
				}
			}
			if len(columns) != 2 || columns[0] != columns[1] || emailRow < 0 || telegramRow <= emailRow {
				t.Errorf("width %d selection %d: option text columns = %v, rows = %q", width, selected, columns, stripANSI.ReplaceAllString(m.login.options.View(), ""))
			}
		}
	}
}

func TestValidateEmail(t *testing.T) {
	for address, valid := range map[string]bool{
		"a@example.com":       true,
		" a@example.com ":     true,
		"a.b+tag@sub.io":      true,
		"":                    false,
		"a@":                  false,
		"@example.com":        false,
		"@@":                  false,
		"a b@example.com":     false,
		"Ari <a@example.com>": false,
	} {
		if err := validateEmail(address); (err == nil) != valid {
			t.Errorf("validateEmail(%q) = %v, want valid=%v", address, err, valid)
		}
	}
}

func TestFailedEmailRequestKeepsFormEditable(t *testing.T) {
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	m.login.showForm(emailStep)
	m = typeText(m, "a@example.co")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.busy || !strings.Contains(m.View().Content, "a@example.co") {
		t.Fatal("submitted entry hidden while the request runs")
	}
	next, _ := m.Update(startResult{"", errors.New("offline")})
	m = next.(Model)
	m = typeText(m, "m")
	if m.login.step != emailStep || m.login.values.email != "a@example.com" || !strings.Contains(m.View().Content, "a@example.com") {
		t.Fatalf("entry not editable after failure: stage %d, value %q", m.stage, m.login.values.email)
	}
	if m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter}); !m.busy || cmd == nil {
		t.Fatal("corrected entry was not resubmitted")
	}
}
