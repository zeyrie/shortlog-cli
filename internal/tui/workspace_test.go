package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func demoAt(t *testing.T, width, height int) Model {
	t.Helper()
	next, _ := NewDemo().Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Model)
}

func plain(m Model) string { return stripStatusANSI.ReplaceAllString(m.View().Content, "") }

func TestWorkspaceFitsTheTerminal(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 40}, {100, 30}, {80, 24}, {60, 20}, {50, 12}} {
		m := demoAt(t, size.width, size.height)
		for _, focus := range []rune{'0', '1', '2', '3'} {
			m, _ = press(m, runeKey(focus))
			view := plain(m)
			if lipgloss.Height(view) != size.height {
				t.Errorf("%dx%d focus %c: height %d", size.width, size.height, focus, lipgloss.Height(view))
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size.width {
					t.Errorf("%dx%d focus %c: overflow %q", size.width, size.height, focus, line)
				}
			}
		}
	}
}

func TestWorkspaceFocusKeys(t *testing.T) {
	m := demoAt(t, 100, 30)
	if m.workspace.focus != focusNotes {
		t.Fatal("the workspace should open on the notes panel")
	}
	for _, step := range []struct {
		key  tea.KeyPressMsg
		want focusArea
	}{
		{runeKey('2'), focusProjects},
		{tea.KeyPressMsg{Code: tea.KeyTab}, focusNotes},
		{tea.KeyPressMsg{Code: tea.KeyTab}, focusMain},
		{tea.KeyPressMsg{Code: tea.KeyTab}, focusAccount},
		{tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, focusMain},
		{runeKey('3'), focusNotes},
		{tea.KeyPressMsg{Code: tea.KeyEnter}, focusMain},
		{tea.KeyPressMsg{Code: tea.KeyEsc}, focusNotes},
		{runeKey('1'), focusAccount},
		{tea.KeyPressMsg{Code: tea.KeyEnter}, focusMain},
		{tea.KeyPressMsg{Code: tea.KeyEsc}, focusAccount},
	} {
		m, _ = press(m, step.key)
		if m.workspace.focus != step.want {
			t.Fatalf("after %s: focus %d, want %d", step.key.String(), m.workspace.focus, step.want)
		}
	}
	if !strings.Contains(plain(m), "Sessions") {
		t.Fatal("the account panel did not show the account page")
	}
}

func TestProjectsPanelCapsAndScrolls(t *testing.T) {
	m := demoAt(t, 100, 30)
	m, _ = press(m, runeKey('2'))
	_, projects, _ := m.workspace.columnHeights()
	if projects-2 != projectRows {
		t.Fatalf("projects panel shows %d rows, want the cap of %d", projects-2, projectRows)
	}
	if !strings.Contains(plain(m), "1 of 11") {
		t.Fatal("no position counter for a list longer than the panel")
	}
	for i := 0; i < 9; i++ {
		m, _ = press(m, runeKey('j'))
	}
	view := plain(m)
	if !strings.Contains(view, "10 of 11") || !strings.Contains(view, "Health") || strings.Contains(view, "  Work  ") {
		t.Fatalf("list did not scroll with the cursor: %q", view)
	}
	m, _ = press(m, runeKey(']'))
	if view := plain(m); !strings.Contains(view, "Wedding planning") || !strings.Contains(view, "(archived)") || strings.Contains(view, " of ") {
		t.Fatalf("archived tab: %q", view)
	}
}

func TestNotesTabsFollowTheSelectedProject(t *testing.T) {
	m := demoAt(t, 100, 30)
	if m.workspace.notes.tab != inboxTab || !strings.Contains(plain(m), "Call the bank") {
		t.Fatal("the notes panel should open on the Inbox")
	}
	m, _ = press(m, runeKey('2'))
	m, _ = press(m, runeKey('j')) // Reading list
	if m.workspace.notes.tab != projectTab || m.workspace.notes.current.Name != "Reading list" {
		t.Fatal("moving in projects did not show that project's notes")
	}
	if view := plain(m); !strings.Contains(view, "The Pragmatic Programmer") || !strings.Contains(view, "Reading list") {
		t.Fatalf("project notes or reader missing: %q", view)
	}
	m, _ = press(m, runeKey('3'))
	m, _ = press(m, runeKey('['))
	if m.workspace.notes.tab != inboxTab || !strings.Contains(plain(m), "· Inbox") {
		t.Fatal("[ did not switch the notes panel back to the Inbox")
	}
	m, _ = press(m, runeKey('j'))
	if note, _ := m.workspace.notes.selected(); m.workspace.reader.note != note.ID {
		t.Fatal("the reader did not follow the selected note")
	}
}

func TestReaderScrollsLongNotes(t *testing.T) {
	m := demoAt(t, 100, 20)
	m, _ = press(m, runeKey('3'))
	m, _ = press(m, runeKey(']')) // Work, whose first note is long
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(plain(m), " 0% ") {
		t.Fatalf("a long note should show its scroll position: %q", plain(m))
	}
	for i := 0; i < 5; i++ {
		m, _ = press(m, runeKey('j'))
	}
	if strings.Contains(plain(m), " 0% ") {
		t.Fatal("j did not scroll the reader")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if strings.Contains(plain(m), " 0% ") {
		t.Fatal("returning to the same note lost its scroll position")
	}
}

func TestNarrowTerminalShowsOneColumn(t *testing.T) {
	m := demoAt(t, 60, 20)
	if view := plain(m); !strings.Contains(view, "[3]") || strings.Contains(view, "[0]") {
		t.Fatal("narrow terminal should show only the panels")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if view := plain(m); !strings.Contains(view, "[0]") || strings.Contains(view, "[3]") {
		t.Fatal("narrow terminal should show only the main panel once it has focus")
	}
}

func TestPanelFrameKeepsItsSize(t *testing.T) {
	f := frame{number: 2, tabs: []string{"Active", "Archived"}, footer: " 3 of 12 ", focused: true}
	for _, width := range []int{4, 12, 40} {
		view := stripStatusANSI.ReplaceAllString(f.render(strings.Repeat("x", 100)+"\nshort", width, 5), "")
		lines := strings.Split(view, "\n")
		if len(lines) != 5 {
			t.Fatalf("width %d: %d lines", width, len(lines))
		}
		for _, line := range lines {
			if lipgloss.Width(line) != width {
				t.Fatalf("width %d: line %q is %d wide", width, line, lipgloss.Width(line))
			}
		}
	}
}

func TestNoteTitleAndLastUsed(t *testing.T) {
	if noteTitle("\n  \n  First line  \nsecond") != "First line" || noteTitle(" ") != "(empty note)" {
		t.Fatal("note title is the first non-blank line")
	}
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for ago, want := range map[time.Duration]string{0: "used just now", 5 * time.Minute: "used 5 min ago", 3 * time.Hour: "used 3 h ago", 30 * time.Hour: "used yesterday", 72 * time.Hour: "used 3 days ago"} {
		if got := lastUsed(now.Add(-ago), now); got != want {
			t.Errorf("%v ago: %q, want %q", ago, got, want)
		}
	}
}
