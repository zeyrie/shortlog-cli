package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"shortlog-cli/internal/api"
)

type fakeAPI struct {
	starts    int
	verifies  int
	restores  int
	name      string
	zone      string
	meErr     error
	inboxErr  error
	notes     []api.Note
	next      *string
	pages     map[string]api.NotesPage
	pageErr   error
	requested []string
	createErr error
	created   []string
	updateErr error
	updated   []string
	deleteErr error
	deleted   []string
	moveErr   error
	moved     []string

	projectList      []api.Project
	archivedList     []api.Project
	projectsErr      error
	archivedErr      error
	archiveErr       error
	unarchiveErr     error
	archiveCalls     []string
	unarchiveCalls   []string
	createProjectErr error
	projectNotes     []api.Note
	projectNext      *string
	projectPages     map[string]api.NotesPage
	projectPageErr   error
	projectRequested []string
	projectCreated   []string
}

type fakeStore struct {
	token   string
	saveErr error
	deletes int
}

func (s *fakeStore) Load() (string, error) { return s.token, nil }
func (s *fakeStore) Save(token string) error {
	if s.saveErr == nil {
		s.token = token
	}
	return s.saveErr
}
func (s *fakeStore) Delete() error { s.token = ""; s.deletes++; return nil }

func (f *fakeAPI) StartEmail(_ context.Context, _ string) (string, error) {
	f.starts++
	return "challenge", nil
}

func (f *fakeAPI) VerifyEmail(_ context.Context, _, _, name, zone string) (api.VerifyResult, error) {
	f.verifies++
	f.name, f.zone = name, zone
	if name == "" {
		return api.VerifyResult{}, &api.Error{Status: 422, Code: "profile_required"}
	}
	return api.VerifyResult{Status: "signed_in", Token: "secret-token"}, nil
}

func (f *fakeAPI) RestoreEmail(_ context.Context, _ string) (string, error) {
	f.restores++
	return "restored-token", nil
}

func (f *fakeAPI) Me(_ context.Context, _ string) (api.Account, error) {
	return api.Account{ID: "account", Username: "Ari"}, f.meErr
}
func (f *fakeAPI) Inbox(_ context.Context, _ string) (api.NotesPage, error) {
	return api.NotesPage{Items: f.notes, NextCursor: f.next}, f.inboxErr
}
func (f *fakeAPI) InboxPage(_ context.Context, _, cursor string) (api.NotesPage, error) {
	f.requested = append(f.requested, cursor)
	return f.pages[cursor], f.pageErr
}
func (f *fakeAPI) CreateInboxNote(_ context.Context, _, content string) (api.Note, error) {
	f.created = append(f.created, content)
	if f.createErr != nil {
		return api.Note{}, f.createErr
	}
	note := api.Note{ID: "created", Content: content, CreatedAt: time.Now()}
	f.notes = append([]api.Note{note}, f.notes...)
	return note, nil
}
func (f *fakeAPI) UpdateNote(_ context.Context, _, id, content string) (api.Note, error) {
	f.updated = append(f.updated, content)
	if f.updateErr != nil {
		return api.Note{}, f.updateErr
	}
	for i := range f.notes {
		if f.notes[i].ID == id {
			f.notes[i].Content = content
			return f.notes[i], nil
		}
	}
	return api.Note{ID: id, Content: content}, nil
}
func (f *fakeAPI) DeleteNote(_ context.Context, _, id string) error {
	f.deleted = append(f.deleted, id)
	return f.deleteErr
}
func (f *fakeAPI) MoveNote(_ context.Context, _, id, projectID string) (api.Note, error) {
	f.moved = append(f.moved, id+":"+projectID)
	if f.moveErr != nil {
		return api.Note{}, f.moveErr
	}
	for _, source := range []*[]api.Note{&f.notes, &f.projectNotes} {
		for i, note := range *source {
			if note.ID != id {
				continue
			}
			*source = append(append([]api.Note(nil), (*source)[:i]...), (*source)[i+1:]...)
			note.ProjectID = nil
			if projectID != "" {
				target := projectID
				note.ProjectID = &target
				f.projectNotes = append([]api.Note{note}, f.projectNotes...)
			} else {
				f.notes = append([]api.Note{note}, f.notes...)
			}
			return note, nil
		}
	}
	return api.Note{ID: id}, nil
}
func (f *fakeAPI) Projects(_ context.Context, _ string) ([]api.Project, error) {
	return f.projectList, f.projectsErr
}
func (f *fakeAPI) ArchivedProjects(_ context.Context, _ string) ([]api.Project, error) {
	return f.archivedList, f.archivedErr
}
func (f *fakeAPI) ArchiveProject(_ context.Context, _, id string) error {
	f.archiveCalls = append(f.archiveCalls, id)
	if f.archiveErr != nil {
		return f.archiveErr
	}
	for i, project := range f.projectList {
		if project.ID == id {
			f.projectList = append(append([]api.Project(nil), f.projectList[:i]...), f.projectList[i+1:]...)
			now := time.Now()
			project.ArchivedAt = &now
			f.archivedList = append([]api.Project{project}, f.archivedList...)
			break
		}
	}
	return nil
}
func (f *fakeAPI) UnarchiveProject(_ context.Context, _, id string) error {
	f.unarchiveCalls = append(f.unarchiveCalls, id)
	if f.unarchiveErr != nil {
		return f.unarchiveErr
	}
	for i, project := range f.archivedList {
		if project.ID == id {
			f.archivedList = append(append([]api.Project(nil), f.archivedList[:i]...), f.archivedList[i+1:]...)
			project.ArchivedAt = nil
			f.projectList = append([]api.Project{project}, f.projectList...)
			break
		}
	}
	return nil
}
func (f *fakeAPI) CreateProject(_ context.Context, _, name string) (api.Project, error) {
	if f.createProjectErr != nil {
		return api.Project{}, f.createProjectErr
	}
	project := api.Project{ID: "project-" + name, Name: name}
	f.projectList = append([]api.Project{project}, f.projectList...)
	return project, nil
}
func (f *fakeAPI) ProjectNotes(_ context.Context, _, _ string) (api.NotesPage, error) {
	return api.NotesPage{Items: f.projectNotes, NextCursor: f.projectNext}, f.inboxErr
}
func (f *fakeAPI) ProjectNotesPage(_ context.Context, _, _, cursor string) (api.NotesPage, error) {
	f.projectRequested = append(f.projectRequested, cursor)
	return f.projectPages[cursor], f.projectPageErr
}
func (f *fakeAPI) CreateProjectNote(_ context.Context, _, _, content string) (api.Note, error) {
	f.projectCreated = append(f.projectCreated, content)
	if f.createErr != nil {
		return api.Note{}, f.createErr
	}
	note := api.Note{ID: "project-created", Content: content, CreatedAt: time.Now()}
	f.projectNotes = append([]api.Note{note}, f.projectNotes...)
	return note, nil
}

func press(m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

func TestEmailProfileSignIn(t *testing.T) {
	f := &fakeAPI{}
	s := &fakeStore{}
	m := New(f, s)
	m.stage, m.busy = emailStage, false
	m.inputs[emailInput].SetValue("a@example.com")
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy || cmd == nil {
		t.Fatal("expected async email start")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != codeStage || f.starts != 1 {
		t.Fatalf("start: stage=%d, requests=%d", m.stage, f.starts)
	}
	m.inputs[codeInput].SetValue("12345678")
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.stage != profileStage || m.inputs[codeInput].Value() != "12345678" {
		t.Fatal("expected profile form with code retained")
	}
	m.inputs[usernameInput].SetValue("Ari")
	m.inputs[zoneInput].SetValue("Europe/London")
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, cmd = m.Update(cmd())
	m = next.(Model)
	if m.stage != inboxStage || m.token != "secret-token" || f.name != "Ari" || f.zone != "Europe/London" {
		t.Fatalf("sign-in: stage=%d, name=%q, zone=%q", m.stage, f.name, f.zone)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if s.token != "secret-token" || m.busy || m.username != "Ari" {
		t.Fatal("session and Inbox not loaded")
	}
	if strings.Contains(m.View(), "secret-token") || strings.Contains(m.View(), "12345678") {
		t.Fatal("secret appeared in screen")
	}
}

func TestRestoreNeedsExplicitConsent(t *testing.T) {
	f := &fakeAPI{}
	m := New(f, &fakeStore{})
	m.busy = false
	next, _ := m.Update(verifyResult{result: api.VerifyResult{Status: "restore_required", RecoveryTicket: "secret-ticket"}})
	m = next.(Model)
	if m.stage != restoreStage || strings.Contains(m.View(), "secret-ticket") {
		t.Fatal("expected restoration confirmation without ticket exposure")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if f.restores != 0 || m.ticket != "" || m.stage != emailStage {
		t.Fatal("decline must not restore")
	}
	next, _ = m.Update(verifyResult{result: api.VerifyResult{Status: "restore_required", RecoveryTicket: "secret-ticket"}})
	m = next.(Model)
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil || f.restores != 0 {
		t.Fatal("restore should be asynchronous after consent")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if f.restores != 1 || m.stage != inboxStage || m.token != "restored-token" || m.ticket != "" {
		t.Fatal("restore did not finish securely")
	}
}

func TestInvalidCodeDoesNotCallAPI(t *testing.T) {
	f := &fakeAPI{}
	m := New(f, &fakeStore{})
	m.stage = codeStage
	m.busy = false
	m.inputs[codeInput].SetValue("123")
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || f.verifies != 0 || m.busy {
		t.Fatal("invalid code sent to API")
	}
}

func TestSavedSessionAndReadInbox(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{{ID: "one", Content: "first\nline", CreatedAt: time.Now()}, {ID: "two", Content: "second", CreatedAt: time.Now()}}}
	s := &fakeStore{token: "saved-token"}
	m := New(f, s)
	next, cmd := m.Update(loadedSession{token: s.token})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected validation request")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.stage != inboxStage || len(m.notes) != 2 || m.token != "saved-token" {
		t.Fatal("saved session did not open Inbox")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.selected != 1 {
		t.Fatal("did not select second note")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.stage != readingStage || !strings.Contains(m.View(), "second") {
		t.Fatal("did not open note")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.stage != inboxStage {
		t.Fatal("did not return to list")
	}
}

func TestExpiredSessionRemoved(t *testing.T) {
	f := &fakeAPI{meErr: &api.Error{Status: 401, Code: "unauthorized"}}
	s := &fakeStore{token: "expired-token"}
	m := New(f, s)
	next, cmd := m.Update(loadedSession{token: s.token})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.stage != emailStage || m.token != "" || s.token != "" || s.deletes != 1 {
		t.Fatal("expired session not cleared")
	}
}

func TestNetworkErrorKeepsSavedSession(t *testing.T) {
	f := &fakeAPI{meErr: errors.New("offline")}
	s := &fakeStore{token: "saved-token"}
	m := New(f, s)
	next, cmd := m.Update(loadedSession{token: s.token})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.stage != inboxStage || s.token != "saved-token" || s.deletes != 0 || !strings.Contains(m.View(), "retry") {
		t.Fatal("network error lost session")
	}
}

func TestSanitizeNoteContent(t *testing.T) {
	if got := safeText("ok\x1b[31m\nnext"); got != "ok[31m\nnext" {
		t.Fatalf("unsafe text: %q", got)
	}
}

func TestQuickCaptureAndRefresh(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{{ID: "older", Content: "older", CreatedAt: time.Now()}}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = inboxStage, false, "token"
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.stage != captureStage || cmd == nil {
		t.Fatal("n did not open quick capture")
	}
	m.draft.SetValue("first line\nsecond line")
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.draft.Value(), "second line\n") {
		t.Fatal("Enter did not insert a newline")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if !m.busy || cmd == nil || len(f.created) != 0 {
		t.Fatal("save did not start asynchronously")
	}
	next, refresh := m.Update(cmd())
	m = next.(Model)
	if m.stage != inboxStage || m.draft.Value() != "" || len(f.created) != 1 || f.created[0] != "first line\nsecond line\n" || refresh == nil {
		t.Fatalf("save: stage=%d, drafts=%q, created=%v", m.stage, m.draft.Value(), f.created)
	}
	next, _ = m.Update(refresh())
	m = next.(Model)
	if m.busy || m.notes[m.selected].ID != "created" || !strings.Contains(m.View(), "Note saved") {
		t.Fatal("new note was not selected after refresh")
	}
}

func TestQuickCaptureEmptyAndFailureRetainDraft(t *testing.T) {
	f := &fakeAPI{createErr: &api.Error{Status: 503, Code: "service_unavailable"}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = inboxStage, false, "token"
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m.draft.SetValue(" \n ")
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || len(f.created) != 0 {
		t.Fatal("blank note submitted")
	}
	m.draft.SetValue("Keep me")
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != captureStage || m.busy || m.draft.Value() != "Keep me" || !strings.Contains(m.View(), "not confirmed") {
		t.Fatal("failed save lost draft")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.stage != discardStage || m.draft.Value() != "Keep me" {
		t.Fatal("Esc should confirm before discarding draft")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.stage != inboxStage || m.draft.Value() != "" {
		t.Fatal("confirmed discard did not clear draft")
	}
}

func TestQuickCaptureUnauthorizedResumesAfterSignIn(t *testing.T) {
	f := &fakeAPI{createErr: &api.Error{Status: 401, Code: "unauthorized"}}
	s := &fakeStore{token: "expired"}
	m := New(f, s)
	m.stage, m.busy, m.token = captureStage, false, "expired"
	m.draft.SetValue("Unsent note")
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != emailStage || !m.resumeDraft || m.draft.Value() != "Unsent note" || s.token != "" {
		t.Fatal("expired session lost draft")
	}
	cmd = m.signedIn("new-token")
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.stage != captureStage || m.draft.Value() != "Unsent note" || m.resumeDraft {
		t.Fatal("did not resume draft")
	}
}

func TestInboxPaginationAndRefresh(t *testing.T) {
	first, second := "first-cursor", "second-cursor"
	f := &fakeAPI{
		notes: []api.Note{{ID: "new", Content: "new"}, {ID: "middle", Content: "middle"}},
		next:  &first,
		pages: map[string]api.NotesPage{
			first:  {Items: []api.Note{{ID: "middle", Content: "middle"}, {ID: "old", Content: "old"}}, NextCursor: &second},
			second: {Items: []api.Note{{ID: "oldest", Content: "oldest"}}},
		},
	}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = inboxStage, false, "token"
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if len(m.notes) != 2 || m.nextCursor != first {
		t.Fatal("first page did not load")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if !m.busy || cmd == nil {
		t.Fatal("older page did not start")
	}
	m, blocked := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if blocked != nil {
		t.Fatal("sent duplicate page request while loading")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.notes) != 3 || m.selected != 2 || m.notes[2].ID != "old" || m.nextCursor != second || len(f.requested) != 1 || f.requested[0] != first {
		t.Fatalf("page 2: %+v, %v", m.notes, f.requested)
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.notes) != 4 || m.selected != 3 || m.nextCursor != "" || f.requested[1] != second {
		t.Fatal("final page did not load")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if cmd != nil || len(f.requested) != 2 {
		t.Fatal("requested beyond final page")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.notes) != 2 || m.selected != 0 || m.nextCursor != first {
		t.Fatal("refresh did not reset to newest page")
	}
}

func TestOlderPageFailurePreservesListAndCursor(t *testing.T) {
	cursor := "retry-me"
	f := &fakeAPI{pageErr: errors.New("offline")}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = inboxStage, false, "token"
	m.notes = []api.Note{{ID: "existing", Content: "existing"}}
	m.nextCursor = cursor
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.busy || len(m.notes) != 1 || m.nextCursor != cursor || !strings.Contains(m.View(), "retry") {
		t.Fatal("failed page lost existing notes or cursor")
	}
	f.pageErr = nil
	f.pages = map[string]api.NotesPage{cursor: {Items: []api.Note{{ID: "older", Content: "older"}}}}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.notes) != 2 || m.notes[1].ID != "older" || m.nextCursor != "" {
		t.Fatal("retry did not append page")
	}
}

func TestEditFromReaderUpdatesNoteWithoutReordering(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{{ID: "one", Content: "before", CreatedAt: time.Now()}, {ID: "two", Content: "second", CreatedAt: time.Now()}}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token, m.notes = inboxStage, false, "token", append([]api.Note(nil), f.notes...)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.stage != captureStage || m.draft.Value() != "before" || m.editorID != "one" {
		t.Fatal("editor did not load selected note")
	}
	m.draft.SetValue("after\nline")
	if !strings.Contains(m.View(), "10 / 20,000 characters") || !strings.Contains(m.View(), "unsaved") {
		t.Fatal("editor did not show character count and dirty status")
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil || len(f.updated) != 0 {
		t.Fatal("edit not dispatched asynchronously")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != readingStage || m.busy || m.notes[0].Content != "after\nline" || m.notes[1].ID != "two" || !strings.Contains(m.View(), "after") || len(f.updated) != 1 {
		t.Fatal("edit did not update reader and preserve list order")
	}
}

func TestEditDiscardConfirmationAndNoOpSave(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{{ID: "one", Content: "original"}}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token, m.notes = inboxStage, false, "token", append([]api.Note(nil), f.notes...)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || len(f.updated) != 0 || m.stage != inboxStage {
		t.Fatal("unchanged edit sent API request")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m.draft.SetValue("different")
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.stage != discardStage || len(f.updated) != 0 {
		t.Fatal("Esc silently lost edits")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.stage != captureStage || m.draft.Value() != "different" {
		t.Fatal("cancel discard did not retain edits")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.stage != inboxStage || m.draft.Value() != "" || f.notes[0].Content != "original" {
		t.Fatal("confirmed discard changed note")
	}
}

func TestEditFailureRetainsDraft(t *testing.T) {
	f := &fakeAPI{updateErr: &api.Error{Status: 404, Code: "not_found"}, notes: []api.Note{{ID: "one", Content: "old"}}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token, m.notes = inboxStage, false, "token", append([]api.Note(nil), f.notes...)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m.draft.SetValue("important changes")
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != captureStage || m.draft.Value() != "important changes" || m.notes[0].Content != "old" || !strings.Contains(m.View(), "no longer available") {
		t.Fatal("failed edit lost draft")
	}
}

func TestDeleteRequiresConfirmationAndPreservesCursor(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{{ID: "one", Content: "first"}, {ID: "two", Content: "second"}}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token, m.notes = inboxStage, false, "token", append([]api.Note(nil), f.notes...)
	m.selected, m.nextCursor = 1, "older-cursor"
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.stage != deleteStage || !strings.Contains(m.View(), "no Trash") || len(f.deleted) != 0 {
		t.Fatal("delete did not require confirmation")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.stage != inboxStage || len(f.deleted) != 0 {
		t.Fatal("cancel triggered deletion")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if !m.busy || cmd == nil || len(f.deleted) != 0 {
		t.Fatal("delete not asynchronous")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != inboxStage || len(m.notes) != 1 || m.notes[0].ID != "one" || m.selected != 0 || m.nextCursor != "older-cursor" || len(f.deleted) != 1 || f.deleted[0] != "two" {
		t.Fatal("delete did not remove selected note while preserving pagination")
	}
}

func TestDeleteFailureKeepsNote(t *testing.T) {
	f := &fakeAPI{deleteErr: errors.New("connection lost"), notes: []api.Note{{ID: "one", Content: "first"}}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token, m.notes = inboxStage, false, "token", append([]api.Note(nil), f.notes...)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != inboxStage || len(m.notes) != 1 || !strings.Contains(m.View(), "not confirmed") {
		t.Fatal("delete failure removed note")
	}
}

func TestQuitWithDirtyEditorNeedsConfirmation(t *testing.T) {
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = inboxStage, false
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m.draft.SetValue("do not lose")
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil || m.stage != discardStage || !m.quitAfterDiscard {
		t.Fatal("quit lost unsaved draft")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.stage != captureStage || m.quitAfterDiscard || m.draft.Value() != "do not lose" {
		t.Fatal("cancel quit lost draft")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("confirmed quit did not exit")
	}
}

func TestProjectNavigationAndInboxRestore(t *testing.T) {
	f := &fakeAPI{
		notes:        []api.Note{{ID: "inbox-one", Content: "inbox one"}, {ID: "inbox-two", Content: "inbox two"}},
		projectList:  []api.Project{{ID: "p1", Name: "First"}, {ID: "p2", Name: "Second"}},
		projectNotes: []api.Note{{ID: "pn1", Content: "project note"}},
	}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = inboxStage, false, "token"
	loaded, _ := m.Update(inboxResult{account: api.Account{Username: "Ari"}, page: api.NotesPage{Items: f.notes}})
	m = loaded.(Model)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.selected != 1 {
		t.Fatal("setup: expected second Inbox note selected")
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.stage != projectsStage || !m.busy || cmd == nil {
		t.Fatal("p did not open loading project list")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.busy || len(m.projects) != 2 || m.projectSelected != 0 {
		t.Fatal("projects list did not load")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy || m.activeProject == nil || m.activeProject.ID != "p2" || !m.inboxStashed {
		t.Fatal("Enter did not open the selected project with Inbox stashed")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.busy || len(m.notes) != 1 || m.notes[0].ID != "pn1" || !strings.Contains(m.View(), "Second") {
		t.Fatal("project notes did not load under the project title")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.stage != readingStage || !strings.Contains(m.View(), "project note") {
		t.Fatal("project note reader failed")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.stage != projectsStage || m.activeProject != nil {
		t.Fatal("Esc did not return from project notes to the project list")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if m.stage != inboxStage || m.activeProject != nil || m.notes[m.selected].ID != "inbox-two" || m.nextCursor != "" {
		t.Fatal("Inbox snapshot was not restored with selection")
	}
	if !strings.Contains(m.View(), "Inbox") {
		t.Fatal("Inbox header not restored")
	}
}

func TestProjectNoteCreateEditDeletePaginate(t *testing.T) {
	cursor := "project-cursor"
	f := &fakeAPI{
		projectList:  []api.Project{{ID: "p1", Name: "Work"}},
		projectNotes: []api.Note{{ID: "pn1", Content: "existing"}},
		projectNext:  &cursor,
		projectPages: map[string]api.NotesPage{cursor: {Items: []api.Note{{ID: "pn-old", Content: "older"}}}},
	}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = projectsStage, false, "token"
	m.projects = f.projectList
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.activeProject == nil {
		t.Fatal("setup: project did not open")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.notes) != 2 || f.projectRequested[0] != cursor {
		t.Fatal("older project notes did not load through the project cursor")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.stage != captureStage {
		t.Fatal("n did not open the editor inside the project")
	}
	m.draft.SetValue("for the project")
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	next, refresh := m.Update(cmd())
	m = next.(Model)
	if f.projectCreated[0] != "for the project" || len(f.created) != 0 {
		t.Fatal("new note was not created in the project")
	}
	next, _ = m.Update(refresh())
	m = next.(Model)
	if m.notes[m.selected].Content != "for the project" {
		t.Fatal("created project note not selected after refresh")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m.draft.SetValue("edited in project")
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.notes[m.selected].Content != "edited in project" {
		t.Fatal("project note edit failed")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(f.deleted) != 1 || len(m.notes) != 1 || m.notes[0].ID != "pn1" {
		t.Fatal("project note delete failed")
	}
}

func TestCreateProjectFlow(t *testing.T) {
	f := &fakeAPI{}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = projectsStage, false, "token"
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if m.stage != newProjectStage {
		t.Fatal("a did not open the new-project form")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if strings.Contains(m.View(), "Enter a project name") == false {
		t.Fatal("empty name accepted")
	}
	m.inputs[projectNameInput].SetValue("Research")
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.busy || cmd == nil {
		t.Fatal("project creation not dispatched")
	}
	next, load := m.Update(cmd())
	m = next.(Model)
	if m.activeProject == nil || m.activeProject.Name != "Research" || len(m.projects) != 1 || m.inputs[projectNameInput].Value() != "" {
		t.Fatal("created project did not open")
	}
	next, _ = m.Update(load())
	m = next.(Model)
	if m.stage != inboxStage || m.busy {
		t.Fatal("project notes screen did not settle")
	}
}

func TestCreateProjectFailureKeepsForm(t *testing.T) {
	f := &fakeAPI{createProjectErr: &api.Error{Status: 400, Code: "invalid_request"}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = newProjectStage, false, "token"
	m.inputs[projectNameInput].SetValue("Nope")
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != newProjectStage || m.inputs[projectNameInput].Value() != "Nope" || !strings.Contains(m.View(), "1–120") {
		t.Fatal("failed creation lost the form")
	}
}

func TestMoveInboxNoteFromReaderAndCancel(t *testing.T) {
	cursor := "older-inbox"
	f := &fakeAPI{
		projectList: []api.Project{{ID: "p1", Name: "Work"}, {ID: "p2", Name: "Home"}},
		notes:       []api.Note{{ID: "one", Content: "first"}, {ID: "two", Content: "second"}},
		next:        &cursor,
	}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = inboxStage, false, "token"
	m.notes = append([]api.Note(nil), f.notes...)
	m.nextCursor = cursor
	m.selected = 1
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.stage != moveStage || !m.busy || cmd == nil {
		t.Fatal("move picker not opened from reader")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if len(m.moveTargets) != 2 || m.moveTargets[0].id != "p1" || m.moveTargets[1].id != "p2" || strings.Contains(m.View(), "Inbox\n\nInbox") {
		t.Fatal("Inbox picker has wrong targets")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.stage != readingStage || len(f.moved) != 0 {
		t.Fatal("Esc did not cancel move")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.busy || len(f.moved) != 0 {
		t.Fatal("move was not asynchronous")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.stage != inboxStage || len(m.notes) != 1 || m.notes[0].ID != "one" || m.selected != 0 || m.nextCursor != cursor || len(f.moved) != 1 || f.moved[0] != "two:p2" || len(f.projectNotes) != 1 {
		t.Fatalf("move failed: notes=%+v moved=%v", m.notes, f.moved)
	}
}

func TestMoveProjectNoteToInboxRefreshesStaleSnapshot(t *testing.T) {
	f := &fakeAPI{
		projectList:  []api.Project{{ID: "p1", Name: "Work"}, {ID: "p2", Name: "Other"}},
		projectNotes: []api.Note{{ID: "project-one", Content: "from project"}, {ID: "project-two", Content: "stay"}},
		notes:        []api.Note{{ID: "inbox-one", Content: "old inbox"}},
	}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = inboxStage, false, "token"
	m.notes = append([]api.Note(nil), f.notes...)
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	next, _ := m.Update(cmd())
	m = next.(Model)
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.activeProject == nil || !m.inboxStashed {
		t.Fatal("project did not open with Inbox stashed")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.moveTargets) != 2 || m.moveTargets[0].name != "Inbox" || m.moveTargets[1].id != "p2" {
		t.Fatal("project picker failed to exclude source project")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.stage != inboxStage || len(m.notes) != 1 || m.notes[0].ID != "project-two" || !m.inboxNeedsRefresh || f.moved[0] != "project-one:" {
		t.Fatal("move to Inbox failed or snapshot not invalidated")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.stage != projectsStage {
		t.Fatal("did not return to project list")
	}
	// Visiting a second project must not re-stash the old Inbox.
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	m, other := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(other())
	m = next.(Model)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if !m.inboxNeedsRefresh {
		t.Fatal("visiting another project cleared Inbox invalidation")
	}
	m, reload := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if reload == nil || !m.busy {
		t.Fatal("stale Inbox snapshot used instead of fetching")
	}
	next, _ = m.Update(reload())
	m = next.(Model)
	if len(m.notes) != 2 || m.notes[0].ID != "project-one" {
		t.Fatal("moved note missing from Inbox")
	}
}

func TestMoveFailureAndNoDestinationKeepNote(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{{ID: "one", Content: "keep me"}}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = inboxStage, false, "token"
	m.notes = append([]api.Note(nil), f.notes...)
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if len(m.moveTargets) != 0 || !strings.Contains(m.View(), "No other active locations") {
		t.Fatal("empty destination picker is unclear")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || len(f.moved) != 0 || len(m.notes) != 1 {
		t.Fatal("move sent without destination")
	}
	f.projectList = []api.Project{{ID: "p1", Name: "Work"}}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	f.moveErr = &api.Error{Status: 409, Code: "conflict"}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.stage != moveStage || len(m.notes) != 1 || m.notes[0].ID != "one" || !strings.Contains(m.View(), "archived") {
		t.Fatal("failed move lost note or picker")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.stage != inboxStage || len(m.notes) != 1 {
		t.Fatal("cancel after failure lost note")
	}
}

func TestArchiveBrowseReadOnlyAndUnarchive(t *testing.T) {
	cursor := "older-archived-notes"
	f := &fakeAPI{
		projectList:  []api.Project{{ID: "p1", Name: "Work"}, {ID: "p2", Name: "Personal"}},
		projectNotes: []api.Note{{ID: "n1", Content: "historical note"}},
		projectNext:  &cursor,
		projectPages: map[string]api.NotesPage{cursor: {Items: []api.Note{{ID: "n2", Content: "older"}}}},
	}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = projectsStage, false, "token"
	m.projects = append([]api.Project(nil), f.projectList...)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.stage != archiveStage || len(f.archiveCalls) != 0 || !strings.Contains(m.View(), "cannot edit") {
		t.Fatal("archive did not require an informative confirmation")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.stage != projectsStage || len(f.archiveCalls) != 0 {
		t.Fatal("cancel archived the project")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if !m.busy || cmd == nil || len(f.archiveCalls) != 0 {
		t.Fatal("archive not asynchronous")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != projectsStage || m.busy || len(m.projects) != 1 || m.projects[0].ID != "p2" || f.archiveCalls[0] != "p1" {
		t.Fatal("archived project not removed from active list")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !m.showArchived || !m.busy || cmd == nil {
		t.Fatal("t did not switch to archived")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.projects) != 1 || m.projects[0].ArchivedAt == nil || !strings.Contains(m.View(), "Archived projects") {
		t.Fatal("archived list did not load")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if !m.archivedProject() || !strings.Contains(m.View(), "read-only") || m.nextCursor != cursor {
		t.Fatal("archived project did not open read-only")
	}
	for _, key := range []rune{'n', 'e', 'd', 'v'} {
		m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		if cmd != nil || m.stage != inboxStage || len(f.created)+len(f.updated)+len(f.deleted)+len(f.moved) != 0 {
			t.Fatalf("archived note write key %c was not blocked", key)
		}
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.notes) != 2 {
		t.Fatal("archived notes pagination failed")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyUp})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.stage != readingStage || !strings.Contains(m.View(), "historical note") {
		t.Fatal("archived note not readable")
	}
	for _, key := range []rune{'e', 'd', 'v'} {
		m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		if cmd != nil || m.stage != readingStage {
			t.Fatalf("archived reader write key %c was not blocked", key)
		}
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.stage != projectsStage || !m.showArchived {
		t.Fatal("return from archived notes lost list state")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if !m.busy || cmd == nil {
		t.Fatal("unarchive not dispatched")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.stage != projectsStage || len(m.projects) != 0 || f.unarchiveCalls[0] != "p1" {
		t.Fatal("unarchived project not removed from archived list")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.showArchived || len(m.projects) != 2 {
		t.Fatal("active projects did not show unarchived project")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.archivedProject() {
		t.Fatal("unarchived project remained read-only")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.stage != captureStage || cmd == nil {
		t.Fatal("unarchived project did not allow note creation")
	}
}

func TestArchiveFailureKeepsListAndSelection(t *testing.T) {
	f := &fakeAPI{projectList: []api.Project{{ID: "p1", Name: "Work"}}, archiveErr: errors.New("offline")}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = projectsStage, false, "token"
	m.projects = append([]api.Project(nil), f.projectList...)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != projectsStage || len(m.projects) != 1 || m.projectSelected != 0 || !strings.Contains(m.View(), "not confirmed") {
		t.Fatal("failed archive lost project list")
	}
}

func TestUnarchiveFailureKeepsArchivedList(t *testing.T) {
	now := time.Now()
	f := &fakeAPI{archivedList: []api.Project{{ID: "p1", Name: "Old", ArchivedAt: &now}}, unarchiveErr: errors.New("offline")}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token, m.showArchived = projectsStage, false, "token", true
	m.projects = append([]api.Project(nil), f.archivedList...)
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.stage != projectsStage || len(m.projects) != 1 || !m.showArchived || m.projects[0].ID != "p1" || !strings.Contains(m.View(), "not confirmed") {
		t.Fatal("failed unarchive lost archived project")
	}
}

func TestMoveTargetsRemainActiveWhenViewingArchivedProjects(t *testing.T) {
	now := time.Now()
	f := &fakeAPI{
		projectList:  []api.Project{{ID: "active", Name: "Active"}},
		archivedList: []api.Project{{ID: "archived", Name: "Archived", ArchivedAt: &now}},
		notes:        []api.Note{{ID: "note", Content: "move me"}},
	}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.token = projectsStage, false, "token"
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if !m.showArchived || len(m.projects) != 1 || m.projects[0].ID != "archived" {
		t.Fatal("setup failed")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(m.moveTargets) != 1 || m.moveTargets[0].id != "active" || m.projects[0].ID != "archived" {
		t.Fatal("archived project leaked into move targets or changed archived list")
	}
}

func TestEditorFitsPaddedLayoutWithoutBlackCursorLine(t *testing.T) {
	m := New(&fakeAPI{}, &fakeStore{})
	if _, ok := m.draft.FocusedStyle.CursorLine.GetBackground().(lipgloss.NoColor); !ok {
		t.Fatal("focused editor line has a background color")
	}
	m.stage, m.busy = captureStage, false
	m.draft.SetValue(strings.Repeat("x", m.innerWidth()-1))
	m.draft.Focus()
	for _, line := range strings.Split(m.draft.View(), "\n") {
		if width := lipgloss.Width(line); width > m.innerWidth() {
			t.Fatalf("textarea line width %d exceeds inner width %d", width, m.innerWidth())
		}
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if width := lipgloss.Width(line); width > m.contentWidth() {
			t.Fatalf("rendered line width %d exceeds frame width %d", width, m.contentWidth())
		}
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 52, Height: 18})
	m = next.(Model)
	if m.draft.Width() != m.innerWidth() || m.reader.Width != m.innerWidth() {
		t.Fatal("editor or reader width did not track resized inner content area")
	}
}
