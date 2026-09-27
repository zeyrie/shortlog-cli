package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
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
	if m.stage != inboxStage || m.draft.Value() != "" {
		t.Fatal("Esc did not discard draft")
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
