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

var (
	ctrlS = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEsc}
)

func inboxNote(id, content string, age time.Duration) api.Note {
	return api.Note{ID: id, Content: content, CreatedAt: time.Now().Add(-age)}
}

func TestCaptureCreatesAnInboxNote(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{inboxNote("one", "Older note", time.Hour)}}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('n'))
	if m.workspace.popup == nil || m.workspace.popup.kind != capturePopup || !strings.Contains(plain(m), "New note · Inbox") {
		t.Fatal("n did not open the capture popup for the Inbox")
	}
	m = typeText(m, "Hello there")
	m, cmd := press(m, ctrlS)
	if !m.workspace.popup.busy || !strings.Contains(plain(m), "Saving…") {
		t.Fatal("saving should show progress in the popup")
	}
	m = feed(m, cmd)
	if m.workspace.popup != nil || len(f.created) != 1 || f.created[0] != "Hello there" {
		t.Fatalf("note not created: %v", f.created)
	}
	if first, _ := m.workspace.notes.selected(); first.Content != "Hello there" || m.status.current.text != "Note saved to Inbox." {
		t.Fatalf("new note not selected at the top: %q, status %q", first.Content, m.status.current.text)
	}
}

func TestCaptureKeepsTheDraftWhenASaveIsNotConfirmed(t *testing.T) {
	f := &fakeAPI{createErr: errors.New("offline")}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('n'))
	m = typeText(m, "Important")
	m, cmd := press(m, ctrlS)
	m = feed(m, cmd)
	p := m.workspace.popup
	if p == nil || p.busy || p.value() != "Important" || !strings.Contains(p.err, "avoid a duplicate") {
		t.Fatal("an unconfirmed save should keep the popup, the text, and warn about duplicates")
	}
}

func TestCaptureRefusesEmptyAndAsksBeforeDiscarding(t *testing.T) {
	m, _ := openedWorkspace(t, &fakeAPI{})
	m, _ = press(m, runeKey('n'))
	if m, cmd := press(m, ctrlS); cmd != nil || !strings.Contains(m.workspace.popup.err, "Write something") {
		t.Fatal("an empty note should not be sent")
	}
	m = typeText(m, "Draft")
	m, _ = press(m, esc)
	if !m.workspace.popup.discarding {
		t.Fatal("Esc with text should ask before discarding")
	}
	m, _ = press(m, runeKey('n'))
	if m.workspace.popup == nil || m.workspace.popup.discarding || m.workspace.popup.value() != "Draft" {
		t.Fatal("n should keep writing")
	}
	m, _ = press(m, esc)
	m, _ = press(m, runeKey('y'))
	if m.workspace.popup != nil {
		t.Fatal("y should discard")
	}
}

func TestNewNoteGoesToTheVisibleProject(t *testing.T) {
	f := &fakeAPI{projectList: []api.Project{{ID: "p1", Name: "Work"}}}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey(']')) // the notes panel's Work tab
	m, _ = press(m, runeKey('n'))
	if !strings.Contains(plain(m), "New note · Work") {
		t.Fatal("capture popup should name the project")
	}
	m = typeText(m, "For work")
	m, cmd := press(m, ctrlS)
	m = feed(m, cmd)
	if len(f.projectCreated) != 1 || len(f.created) != 0 || len(m.workspace.sets["p1"].notes) != 1 {
		t.Fatalf("note not created in the project: project %v, inbox %v", f.projectCreated, f.created)
	}
}

func TestEditSavesInTheMainPanel(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{inboxNote("one", "First draft", time.Hour)}}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('e'))
	if m.workspace.editor == nil || m.workspace.focus != focusMain || !strings.Contains(plain(m), "Editing · First draft") {
		t.Fatal("e did not open the editor in the main panel")
	}
	m, _ = press(m, esc)
	if m.workspace.editor != nil || m.workspace.focus != focusNotes {
		t.Fatal("Esc without changes should leave the editor straight away")
	}
	m, _ = press(m, runeKey('e'))
	m = typeText(m, "!")
	m, cmd := press(m, ctrlS)
	m = feed(m, cmd)
	if m.workspace.editor != nil || len(f.updated) != 1 || !strings.Contains(plain(m), "First draft!") || m.status.current.text != "Note updated." {
		t.Fatalf("edit not saved or shown: %v", f.updated)
	}
}

func TestEditAsksBeforeDiscardingAndReportsAMissingNote(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{inboxNote("one", "Text", time.Hour)}, updateErr: &api.Error{Status: 404, Code: "not_found"}}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('e'))
	m = typeText(m, " more")
	m, cmd := press(m, ctrlS)
	m = feed(m, cmd)
	if m.workspace.editor == nil || !strings.Contains(m.workspace.editor.err, "no longer exists") {
		t.Fatal("a missing note should keep the editor open with the text")
	}
	m, _ = press(m, esc)
	if m.workspace.popup == nil || m.workspace.popup.action.kind != discardEditAction {
		t.Fatal("Esc with changes should ask before discarding")
	}
	m, _ = press(m, runeKey('y'))
	if m.workspace.editor != nil || m.workspace.popup != nil || m.workspace.focus != focusNotes {
		t.Fatal("confirming should close the editor")
	}
}

func TestMoveNoteToAProject(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{inboxNote("one", "Move me", time.Hour)}, projectList: []api.Project{{ID: "p1", Name: "Work"}}}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('v'))
	if m.workspace.popup == nil || m.workspace.popup.kind != menuPopup || len(m.workspace.popup.options) != 1 {
		t.Fatal("v should offer the other places")
	}
	m, cmd := press(m, enter)
	m = feed(m, cmd)
	if len(f.moved) != 1 || f.moved[0] != "one:p1" || len(m.workspace.notes.inbox) != 0 || len(m.workspace.sets["p1"].notes) != 1 {
		t.Fatalf("note not moved: %v", f.moved)
	}
	if m.status.current.text != "Moved to Work." {
		t.Fatalf("status %q", m.status.current.text)
	}
}

func TestMoveConflictExplainsAndKeepsTheNote(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{inboxNote("one", "Stay", time.Hour)}, projectList: []api.Project{{ID: "p1", Name: "Work"}}, moveErr: &api.Error{Status: 409, Code: "conflict"}}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('v'))
	m, cmd := press(m, enter)
	m = feed(m, cmd)
	if len(m.workspace.notes.inbox) != 1 || m.status.current.level != statusError || !strings.Contains(m.status.current.text, "archived") {
		t.Fatal("a conflict should keep the note and explain")
	}
}

func TestDeleteAsksFirst(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{inboxNote("one", "Doomed", time.Hour)}}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('d'))
	if !strings.Contains(plain(m), "There is no Trash") {
		t.Fatal("delete should warn that it cannot be undone")
	}
	m, cmd := press(m, runeKey('n'))
	if cmd != nil || len(f.deleted) != 0 || m.workspace.popup != nil {
		t.Fatal("n should cancel without deleting")
	}
	m, _ = press(m, runeKey('d'))
	m, cmd = press(m, runeKey('y'))
	m = feed(m, cmd)
	if len(f.deleted) != 1 || len(m.workspace.notes.inbox) != 0 || m.status.current.text != "Note deleted." {
		t.Fatal("y should delete the note")
	}
}

func TestArchivedProjectsAreReadOnly(t *testing.T) {
	archived := time.Now()
	f := &fakeAPI{archivedList: []api.Project{{ID: "a1", Name: "Old", ArchivedAt: &archived}}, projectNotes: []api.Note{inboxNote("x", "Kept", time.Hour)}}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('2'))
	m, cmd := press(m, runeKey(']'))
	m = feed(m, cmd)
	m, _ = press(m, runeKey('3'))
	for _, k := range []rune{'n', 'e', 'v', 'd'} {
		m, _ = press(m, runeKey(k))
		if m.workspace.popup != nil || m.workspace.editor != nil || m.status.current.level != statusWarn {
			t.Fatalf("%c changed an archived project's notes", k)
		}
	}
	for _, b := range m.workspace.ShortHelp() {
		if b.Help().Desc == "new" || b.Help().Desc == "edit" {
			t.Fatal("hints offer changes to an archived project")
		}
	}
}

func TestCreateProject(t *testing.T) {
	f := &fakeAPI{projectList: []api.Project{{ID: "p1", Name: "Work"}}}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('2'))
	m, _ = press(m, runeKey('a'))
	if m, cmd := press(m, enter); cmd != nil || !strings.Contains(m.workspace.popup.err, "1–120") {
		t.Fatal("an empty name should be refused locally")
	}
	m = typeText(m, "Garden")
	m, cmd := press(m, enter)
	m = settleAll(m, cmd)
	if m.workspace.popup != nil || m.workspace.projects.items()[0].Name != "Garden" || m.workspace.notes.current.Name != "Garden" {
		t.Fatal("new project not created, selected, and shown")
	}
	for _, id := range f.projectNoteLoads {
		if id == "project-Garden" {
			t.Fatal("a new project's notes were requested; it has none")
		}
	}
}

func TestArchiveAndUnarchiveProject(t *testing.T) {
	f := &fakeAPI{projectList: []api.Project{{ID: "p1", Name: "Work"}}}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('2'))
	m, _ = press(m, runeKey('x'))
	if m.workspace.popup == nil || !strings.Contains(plain(m), "Archive “Work”?") {
		t.Fatal("x should ask before archiving")
	}
	m, cmd := press(m, runeKey('y'))
	m = settleAll(m, cmd)
	if len(f.archiveCalls) != 1 || len(m.workspace.projects.projects[activeTab]) != 0 || len(m.workspace.projects.projects[archivedTab]) != 1 {
		t.Fatal("project not archived")
	}
	m, _ = press(m, runeKey(']'))
	m, cmd = press(m, runeKey('u'))
	m = settleAll(m, cmd)
	if len(f.unarchiveCalls) != 1 || len(m.workspace.projects.projects[activeTab]) != 1 || m.status.current.text != "Unarchived Work." {
		t.Fatal("project not unarchived")
	}
}

func TestUnsentNoteReturnsAfterSigningInAgain(t *testing.T) {
	f := &fakeAPI{createErr: &api.Error{Status: 401, Code: "unauthorized"}}
	m, s := openedWorkspace(t, f)
	m, _ = press(m, runeKey('n'))
	m = typeText(m, "Don't lose me")
	m, cmd := press(m, ctrlS)
	m = feed(m, cmd)
	if m.stage != loginStage || s.deletes != 1 || m.resume == nil || !strings.Contains(m.View().Content, "unsaved text") {
		t.Fatal("a 401 should sign out and keep the unsent note")
	}
	f.createErr = nil
	m = settleAll(m, m.signedIn("new-token"))
	if m.workspace.popup == nil || m.workspace.popup.value() != "Don't lose me" || m.resume != nil {
		t.Fatal("the unsent note did not reopen after signing in")
	}
}

func TestCtrlCAsksBeforeDiscardingUnsavedText(t *testing.T) {
	m, _ := openedWorkspace(t, &fakeAPI{})
	m, _ = press(m, runeKey('n'))
	m = typeText(m, "Unsaved")
	m, cmd := press(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd != nil || m.workspace.popup == nil || m.workspace.popup.action.kind != quitAction {
		t.Fatal("Ctrl+C with unsaved text should ask first")
	}
	m, cmd = press(m, runeKey('y'))
	if cmd == nil {
		t.Fatal("y should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("y did not quit")
	}
}

func TestPopupsFitTheTerminal(t *testing.T) {
	for _, size := range []struct{ width, height int }{{100, 30}, {80, 24}, {60, 16}} {
		m := demoAt(t, size.width, size.height)
		for _, open := range []rune{'n', 'v', 'd'} {
			m, _ = press(m, runeKey('3'))
			m, _ = press(m, runeKey(open))
			view := plain(m)
			if lipgloss.Height(view) != size.height {
				t.Errorf("%dx%d popup %c: height %d", size.width, size.height, open, lipgloss.Height(view))
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size.width {
					t.Errorf("%dx%d popup %c: overflow %q", size.width, size.height, open, line)
				}
			}
			m, _ = press(m, esc)
			if m.workspace.popup != nil {
				m, _ = press(m, runeKey('n'))
			}
		}
	}
}

func TestMoveMenuScrolls(t *testing.T) {
	m := demoAt(t, 100, 20)
	m, _ = press(m, runeKey('v'))
	for i := 0; i < 10; i++ {
		m, _ = press(m, runeKey('j'))
	}
	if view := plain(m); !strings.Contains(view, "11 of 11") || !strings.Contains(view, "Gift ideas") {
		t.Fatalf("move menu did not scroll: %q", view)
	}
}

func TestDemoChangesStayForTheRun(t *testing.T) {
	m := demoAt(t, 100, 30)
	m, _ = press(m, runeKey('n'))
	m = typeText(m, "Demo note")
	m, cmd := press(m, ctrlS)
	m = settleAll(m, cmd)
	m, cmd = press(m, runeKey('r'))
	m = settleAll(m, cmd)
	if first, _ := m.workspace.notes.selected(); first.Content != "Demo note" {
		t.Fatal("the demo lost a note it created")
	}
}
