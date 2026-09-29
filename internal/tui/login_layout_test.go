package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestLoginStepsShareCenteredLandingLayout(t *testing.T) {
	stripANSI := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	landing := New(&fakeAPI{}, &fakeStore{})
	landing.stage, landing.busy = loginStage, false
	landing.login.setSize(landing.width, landing.bodyHeight())

	email := New(&fakeAPI{}, &fakeStore{})
	email.stage, email.busy = loginStage, false
	email.login.setSize(email.width, email.bodyHeight())
	email.login.showForm(emailStep)

	if column(landing.View(), "Continue with email") == 0 || column(email.View(), "Email address") == 0 {
		t.Fatal("expected both screens to render their content")
	}
	for name, view := range map[string]string{"landing": stripANSI.ReplaceAllString(landing.View(), ""), "email": stripANSI.ReplaceAllString(email.View(), "")} {
		body := withoutFooter(view, landing.footerHeight())
		if height := lipgloss.Height(view); height != 24 {
			t.Errorf("%s: height %d, want 24", name, height)
		}
		left, right := margins(body)
		if abs(left-right) > 1 {
			t.Errorf("%s: not horizontally centered, left=%d right=%d", name, left, right)
		}
		// Vertically centered: the content block must start below the top row.
		if leading(body) < 2 {
			t.Errorf("%s: not vertically centered, starts at row %d", name, leading(body))
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > 80 {
				t.Errorf("%s: line wider than terminal: %q", name, line)
			}
		}
	}
}

// margins returns the left and right padding around all rendered content.
func margins(view string) (int, int) {
	width := lipgloss.Width(strings.Split(view, "\n")[0]) // lines are padded to the terminal width
	left, right := width, 0
	for _, line := range strings.Split(view, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		start := strings.Index(line, trimmed)
		if start < left {
			left = start
		}
		if end := start + lipgloss.Width(trimmed); end > right {
			right = end
		}
	}
	return left, width - right
}

// withoutFooter drops the footer rows, which are left-aligned by design, so a
// layout check sees only the centered sign-in composition above them.
func withoutFooter(view string, rows int) string {
	lines := strings.Split(view, "\n")
	return strings.Join(lines[:max(0, len(lines)-rows)], "\n")
}

// leading returns the number of blank rows before the first content row.
func leading(view string) int {
	for i, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(line) != "" {
			return i
		}
	}
	return 0
}

func column(view, needle string) int {
	stripANSI := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for _, line := range strings.Split(stripANSI.ReplaceAllString(view, ""), "\n") {
		if index := strings.Index(line, needle); index >= 0 {
			return index
		}
	}
	return 0
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestLoginStepsFitShortTerminals(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {42, 16}, {30, 12}, {20, 10}} {
		m := New(&fakeAPI{}, &fakeStore{})
		m.stage, m.busy = loginStage, false
		m.width, m.height = size.width, size.height
		m.login.setSize(m.width, m.bodyHeight())
		m.login.showForm(emailStep)
		m.message = "Could not send a code. Check your connection and try again."
		view := m.View()
		if height := lipgloss.Height(view); height != m.height {
			t.Errorf("%dx%d: height %d", m.width, m.height, height)
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > m.width {
				t.Errorf("%dx%d: overflow %q", m.width, m.height, line)
			}
		}
	}
}

func TestLoginFormCardIsStable(t *testing.T) {
	stripANSI := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 30}, {42, 16}} {
		m := New(&fakeAPI{}, &fakeStore{})
		m.stage, m.busy = loginStage, false
		m.width, m.height = size.width, size.height
		m.login.setSize(m.width, m.bodyHeight())
		logoRow := row(stripANSI.ReplaceAllString(m.View(), ""), "A quiet place")
		m.login.showForm(emailStep)
		view := stripANSI.ReplaceAllString(m.View(), "")
		left, right := card(view)
		if left < 0 {
			t.Fatalf("%dx%d: no card: %q", m.width, m.height, view)
		}
		// The heading is centered over the card, and the whole composition is
		// centered in the terminal.
		if heading := centerOf(view, "Sign in with email"); abs(heading-(left+right)/2) > 1 {
			t.Errorf("%dx%d: heading centered at %d, card at %d", m.width, m.height, heading, (left+right)/2)
		}
		if body := withoutFooter(view, m.footerHeight()); abs(center(body)-m.width/2) > 2 {
			t.Errorf("%dx%d: composition center %d, want %d", m.width, m.height, center(body), m.width/2)
		}
		// Typing, loading, and the next step must not move or resize the card,
		// nor move the header away from where the welcome screen put it.
		m = typeText(m, "a.very.long.address.for.testing@example")
		typed := stripANSI.ReplaceAllString(m.View(), "")
		m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
		loading := stripANSI.ReplaceAllString(m.View(), "")
		if !strings.Contains(loading, "Sending a code") {
			t.Errorf("%dx%d: no loader while sending: %q", m.width, m.height, loading)
		}
		next, _ := m.Update(cmd())
		m = next.(Model)
		code := stripANSI.ReplaceAllString(m.View(), "")
		for name, v := range map[string]string{"typed": typed, "loading": loading, "code": code} {
			if l, r := card(v); l != left || r != right {
				t.Errorf("%dx%d %s: card moved from %d–%d to %d–%d", m.width, m.height, name, left, right, l, r)
			}
			if got := row(v, "A quiet place"); got != logoRow {
				t.Errorf("%dx%d %s: tagline on row %d, welcome screen row %d", m.width, m.height, name, got, logoRow)
			}
		}
	}
}

// card returns the columns of the rounded card's left and right corners.
func card(view string) (int, int) {
	for _, line := range strings.Split(view, "\n") {
		if l, r := strings.Index(line, "╭"), strings.Index(line, "╮"); l >= 0 && r > l {
			return lipgloss.Width(line[:l]), lipgloss.Width(line[:r])
		}
	}
	return -1, -1
}

// row returns the index of the first line containing a needle.
func row(view, needle string) int {
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

// centerOf returns the midpoint column of the line containing a needle.
func centerOf(view, needle string) int {
	for _, line := range strings.Split(view, "\n") {
		index := strings.Index(line, needle)
		if index < 0 {
			continue
		}
		lead := len(line) - len(strings.TrimLeft(line, " "))
		return lead + (lipgloss.Width(strings.TrimRight(line, " "))-lead)/2
	}
	return 0
}

// center returns the midpoint of all rendered content on the widest row.
func center(view string) int {
	left, right := 1<<30, 0
	for _, line := range strings.Split(view, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lead := len(line) - len(strings.TrimLeft(line, " "))
		if lead < left {
			left = lead
		}
		if end := lead + lipgloss.Width(trimmed); end > right {
			right = end
		}
	}
	return (left + right) / 2
}

func TestLoginFormResizesWithTerminal(t *testing.T) {
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	m.login.showForm(emailStep)
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 24, Height: 20})
	m = resized.(Model)
	for _, line := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(line) > 24 {
			t.Errorf("resized terminal overflow: %q", line)
		}
	}
}
