package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"shortlog-cli/internal/api"
)

// openedWorkspace signs in with the fake API and applies the workspace's
// first loads, including the ones they start.
func openedWorkspace(t *testing.T, f *fakeAPI) (Model, *fakeStore) {
	t.Helper()
	s := &fakeStore{}
	m := New(f, s)
	m.stage, m.busy = loginStage, false
	return settleAll(m, m.signedIn("token")), s
}

// settleAll runs a command and every command its results start, until none
// are left. Only for workspace loads, which end; timers never would.
func settleAll(m Model, cmd tea.Cmd) Model {
	for _, msg := range runCmd(cmd) {
		next, more := m.Update(msg)
		m = settleAll(next.(Model), more)
	}
	return m
}

func notesNamed(prefix string, n int) []api.Note {
	notes := make([]api.Note, n)
	for i := range notes {
		notes[i] = api.Note{ID: fmt.Sprintf("%s%d", prefix, i), Content: fmt.Sprintf("%s note %d", prefix, i), CreatedAt: time.Now()}
	}
	return notes
}

func TestWorkspaceLoadsOnOpen(t *testing.T) {
	f := &fakeAPI{
		projectList:  []api.Project{{ID: "p1", Name: "Work"}},
		archivedList: []api.Project{{ID: "a1", Name: "Old"}},
		notes:        notesNamed("inbox", 2),
		sessionList:  []api.Session{{ID: "s1", DeviceLabel: "Laptop", Current: true}},
	}
	m, s := openedWorkspace(t, f)
	w := m.workspace
	if s.saves != 1 || w.userName() != "Ari" || len(w.account.sessions) != 1 {
		t.Fatal("session not saved, or account and sessions not loaded")
	}
	if len(w.projects.projects[activeTab]) != 1 || len(w.projects.projects[archivedTab]) != 1 || len(w.notes.inbox) != 2 {
		t.Fatal("projects or Inbox not loaded")
	}
	if len(f.projectNoteLoads) != 1 || f.projectNoteLoads[0] != "p1" {
		t.Fatalf("the first project's notes should load for its tab: %v", f.projectNoteLoads)
	}
	if view := plain(m); !strings.Contains(view, "inbox note 0") || !strings.Contains(view, "Ari") {
		t.Fatalf("workspace view: %q", view)
	}
}

func TestProjectNotesLoadOnceAndAreCached(t *testing.T) {
	f := &fakeAPI{projectList: []api.Project{{ID: "p1", Name: "Work"}, {ID: "p2", Name: "Home"}}, projectNotes: notesNamed("p", 1)}
	m, _ := openedWorkspace(t, f)
	m, _ = press(m, runeKey('2'))
	m, cmd := press(m, runeKey('j'))
	m = settleAll(m, cmd)
	m, cmd = press(m, runeKey('k'))
	if cmd != nil {
		t.Fatal("returning to a loaded project requested its notes again")
	}
	m = feed(m, cmd)
	if strings.Join(f.projectNoteLoads, ",") != "p1,p2" {
		t.Fatalf("project note loads: %v", f.projectNoteLoads)
	}
}

func TestNotesLoadOlderPagesNearTheEnd(t *testing.T) {
	first, second := "c1", "c2"
	f := &fakeAPI{
		notes: notesNamed("a", 8),
		next:  &first,
		pages: map[string]api.NotesPage{
			"c1": {Items: notesNamed("b", 3), NextCursor: &second},
			"c2": {Items: notesNamed("c", 1), NextCursor: &second}, // repeats its cursor
		},
	}
	m, _ := openedWorkspace(t, f)
	if len(f.requested) != 0 {
		t.Fatal("older notes were requested before the cursor neared the end")
	}
	m, cmd := press(m, runeKey('j'))
	m, _ = press(m, runeKey('j'))
	m, cmd = press(m, runeKey('j'))
	if cmd == nil {
		t.Fatal("nearing the end did not request older notes")
	}
	m = feed(m, cmd)
	if len(m.workspace.notes.inbox) != 11 || strings.Join(f.requested, ",") != "c1" {
		t.Fatalf("older page not appended: %d notes, requests %v", len(m.workspace.notes.inbox), f.requested)
	}
	for i := 0; i < 8; i++ {
		m, cmd = press(m, runeKey('j'))
		m = feed(m, cmd)
	}
	warned := false
	for _, entry := range m.status.history {
		warned = warned || (entry.level == statusWarn && strings.Contains(entry.text, "repeated"))
	}
	if len(m.workspace.notes.inbox) != 12 || m.workspace.sets[inboxSource].next != "" || !warned {
		t.Fatalf("a repeated cursor should stop paging with a warning: %d notes, next %q", len(m.workspace.notes.inbox), m.workspace.sets[inboxSource].next)
	}
}

func TestLoadErrorsShowInPlaceAndRetry(t *testing.T) {
	f := &fakeAPI{projectsErr: errors.New("offline")}
	m, _ := openedWorkspace(t, f)
	if view := plain(m); !strings.Contains(view, "Could not load projects") || m.status.current.level != statusError {
		t.Fatalf("projects error not shown: %q", view)
	}
	f.projectsErr = nil
	f.projectList = []api.Project{{ID: "p1", Name: "Work"}}
	m, _ = press(m, runeKey('2'))
	m, cmd := press(m, runeKey('r'))
	if !strings.Contains(plain(m), "Loading projects") {
		t.Fatal("retry did not show loading")
	}
	m = feed(m, cmd)
	if view := plain(m); !strings.Contains(view, "Work") || strings.Contains(view, "Could not load projects") {
		t.Fatalf("retry did not recover: %q", view)
	}
}

func TestRefreshReplacesNotesFromTheNewestPage(t *testing.T) {
	f := &fakeAPI{notes: notesNamed("old", 3)}
	m, _ := openedWorkspace(t, f)
	f.notes = notesNamed("new", 1)
	m, cmd := press(m, runeKey('r'))
	m = feed(m, cmd)
	if len(m.workspace.notes.inbox) != 1 || !strings.Contains(plain(m), "new note 0") {
		t.Fatal("refresh did not replace the Inbox")
	}
}

func TestUnauthorizedWhileLoadingSignsOut(t *testing.T) {
	f := &fakeAPI{inboxErr: &api.Error{Status: 401, Code: "unauthorized"}}
	m, s := openedWorkspace(t, f)
	if m.stage != loginStage || m.token != "" || s.deletes != 1 || !strings.Contains(m.View().Content, "Session expired") {
		t.Fatal("a 401 while loading should sign out and clear the saved session")
	}
}

func TestSaveFailureWarnsButKeepsWorkspace(t *testing.T) {
	f := &fakeAPI{}
	s := &fakeStore{saveErr: errors.New("locked")}
	m := New(f, s)
	m.stage, m.busy = loginStage, false
	m = feed(m, m.signedIn("token"))
	if m.stage != workspaceStage || s.deletes != 1 || m.status.current.level != statusWarn {
		t.Fatal("a failed save should warn, clear the half-saved credential, and keep the workspace")
	}
}

func TestLateResultsAreIgnored(t *testing.T) {
	m, _ := openedWorkspace(t, &fakeAPI{})
	next, _ := m.Update(notesLoaded{token: "older-session", page: api.NotesPage{Items: notesNamed("stale", 2)}})
	if len(next.(Model).workspace.notes.inbox) != 0 {
		t.Fatal("a result from another session was applied")
	}
	m.stage = loginStage
	if next, cmd := m.Update(projectsLoaded{token: "token"}); cmd != nil || next.(Model).stage != loginStage {
		t.Fatal("a result after leaving the workspace was applied")
	}
}
