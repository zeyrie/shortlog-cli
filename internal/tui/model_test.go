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
	starts   int
	verifies int
	restores int
	name     string
	zone     string
	meErr    error
	inboxErr error
	notes    []api.Note
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
	return api.NotesPage{Items: f.notes}, f.inboxErr
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
