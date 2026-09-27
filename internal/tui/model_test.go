package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
	"shortlog-cli/internal/api"
)

type fakeAPI struct {
	starts   int
	verifies int
	restores int
	name     string
	zone     string
}

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

func press(m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

func TestEmailProfileSignIn(t *testing.T) {
	f := &fakeAPI{}
	m := New(f)
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
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.stage != signedInStage || m.token != "secret-token" || f.name != "Ari" || f.zone != "Europe/London" {
		t.Fatalf("sign-in: stage=%d, name=%q, zone=%q", m.stage, f.name, f.zone)
	}
	if strings.Contains(m.View(), "secret-token") || strings.Contains(m.View(), "12345678") {
		t.Fatal("secret appeared in screen")
	}
}

func TestRestoreNeedsExplicitConsent(t *testing.T) {
	f := &fakeAPI{}
	m := New(f)
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
	if f.restores != 1 || m.stage != signedInStage || m.token != "restored-token" || m.ticket != "" {
		t.Fatal("restore did not finish securely")
	}
}

func TestInvalidCodeDoesNotCallAPI(t *testing.T) {
	f := &fakeAPI{}
	m := New(f)
	m.stage = codeStage
	m.inputs[codeInput].SetValue("123")
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || f.verifies != 0 || m.busy {
		t.Fatal("invalid code sent to API")
	}
}
