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
	landing.resizeLoginOptions()

	email := New(&fakeAPI{}, &fakeStore{})
	email.stage, email.busy = loginStage, false
	email.resizeLoginOptions()
	email.showLoginForm(emailStage)

	if column(landing.View(), "Continue with email") == 0 || column(email.View(), "Email address") == 0 {
		t.Fatal("expected both screens to render their content")
	}
	for name, view := range map[string]string{"landing": stripANSI.ReplaceAllString(landing.View(), ""), "email": stripANSI.ReplaceAllString(email.View(), "")} {
		if height := lipgloss.Height(view); height != 24 {
			t.Errorf("%s: height %d, want 24", name, height)
		}
		left, right := margins(view)
		if abs(left-right) > 1 {
			t.Errorf("%s: not horizontally centered, left=%d right=%d", name, left, right)
		}
		// Vertically centered: the content block must start below the top row.
		if leading(view) < 2 {
			t.Errorf("%s: not vertically centered, starts at row %d", name, leading(view))
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
		m.resizeLoginOptions()
		m.showLoginForm(emailStage)
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

func TestLoginFormInputAlignsWithHeading(t *testing.T) {
	stripANSI := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 30}, {42, 16}} {
		m := New(&fakeAPI{}, &fakeStore{})
		m.stage, m.busy = loginStage, false
		m.width, m.height = size.width, size.height
		m.resizeLoginOptions()
		m.showLoginForm(emailStage)
		view := stripANSI.ReplaceAllString(m.View(), "")

		heading, ok := find(view, "Sign in with email")
		if !ok {
			t.Fatalf("%dx%d: heading missing", m.width, m.height)
		}
		input, ok := find(view, "you@example.com")
		if !ok {
			t.Fatalf("%dx%d: input missing", m.width, m.height)
		}
		// The field prompt and the heading should sit in the same column, so the
		// form reads as one centered column rather than a left-hanging input.
		// The field is a column of text; centering means the heading and the
		// field share a midpoint, not a left edge (the field holds the prompt).
		headingCenter := centerOf(view, "Sign in with email")
		inputCenter := centerOf(view, "you@example.com")
		if abs(headingCenter-inputCenter) > 2 {
			t.Errorf("%dx%d: field centered at %d, heading at %d", m.width, m.height, inputCenter, headingCenter)
		}
		if abs(input.start-heading.start) > 3 {
			t.Errorf("%dx%d: input starts at column %d, heading at %d", m.width, m.height, input.start, heading.start)
		}
		if abs(center(view)-m.width/2) > 2 {
			t.Errorf("%dx%d: composition center %d, want %d", m.width, m.height, center(view), m.width/2)
		}
	}
}

// hit is the column where the row containing a needle begins.
type hit struct{ start int }

func find(view, needle string) (hit, bool) {
	for _, line := range strings.Split(view, "\n") {
		if !strings.Contains(line, needle) {
			continue
		}
		// Report the display column where the row's visible content begins, so
		// a field is measured from its bar, not from its placeholder text.
		return hit{start: len(line) - len(strings.TrimLeft(line, " "))}, true
	}
	return hit{}, false
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
	m.showLoginForm(emailStage)
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 24, Height: 20})
	m = resized.(Model)
	for _, line := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(line) > 24 {
			t.Errorf("resized terminal overflow: %q", line)
		}
	}
}
