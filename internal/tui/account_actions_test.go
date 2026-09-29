package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"shortlog-cli/internal/api"
)

func twoSessions() []api.Session {
	return []api.Session{{ID: "s1", DeviceLabel: "Laptop", Current: true}, {ID: "s2", DeviceLabel: "Phone"}}
}

// accountPage opens a workspace and focuses the account panel.
func accountPage(t *testing.T, f *fakeAPI) (Model, *fakeStore) {
	t.Helper()
	m, s := openedWorkspace(t, f)
	m, _ = press(m, runeKey('1'))
	return m, s
}

func TestEditProfile(t *testing.T) {
	f := &fakeAPI{}
	m, _ := accountPage(t, f)
	m, _ = press(m, runeKey('e'))
	p := m.workspace.popup
	if p == nil || p.kind != formPopup || p.values()[0] != "Ari" {
		t.Fatal("e should open the profile form filled in")
	}
	m = typeText(m, " B")
	m, _ = press(m, enter) // on to the time zone
	if m, cmd := press(m, enter); cmd != nil || !strings.Contains(m.workspace.popup.err, "IANA") {
		t.Fatal("an empty time zone should be refused locally")
	}
	m = typeText(m, "UTC")
	m, cmd := press(m, enter)
	m = feed(m, cmd)
	if f.profileCalls != 1 || m.workspace.popup != nil || m.workspace.account.account.Username != "Ari B" || m.workspace.account.account.TimeZone != "UTC" {
		t.Fatalf("profile not saved: %+v", m.workspace.account.account)
	}
	if m.status.current.text != "Profile updated." || !strings.Contains(plain(m), "Ari B") {
		t.Fatal("updated profile not shown")
	}
}

func TestProfileRejectedByTheServerKeepsTheForm(t *testing.T) {
	f := &fakeAPI{account: api.Account{ID: "a", Username: "Ari", TimeZone: "UTC"}, profileErr: &api.Error{Status: 422, Code: "invalid_request"}}
	m, _ := accountPage(t, f)
	m, _ = press(m, runeKey('e'))
	m, _ = press(m, enter)
	m, cmd := press(m, enter)
	m = feed(m, cmd)
	if p := m.workspace.popup; p == nil || !strings.Contains(p.err, "rejected") || p.values()[1] != "UTC" {
		t.Fatal("a rejected profile should keep the form and explain")
	}
}

func TestRevokeAnotherSession(t *testing.T) {
	f := &fakeAPI{sessionList: twoSessions()}
	m, s := accountPage(t, f)
	m, _ = press(m, runeKey('j'))
	m, _ = press(m, runeKey('x'))
	if !strings.Contains(plain(m), "“Phone”") {
		t.Fatal("revoke should name the device")
	}
	m, cmd := press(m, runeKey('y'))
	m = feed(m, cmd)
	if len(f.revokeCalls) != 1 || f.revokeCalls[0] != "s2" || len(m.workspace.account.sessions) != 1 || m.stage != workspaceStage || s.deletes != 0 {
		t.Fatalf("another device's session should be revoked without signing out: %v", f.revokeCalls)
	}
}

func TestEndingThisSessionSignsOut(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys []rune
		call func(*fakeAPI) int
	}{
		{"revoke this device", []rune{'x', 'y'}, func(f *fakeAPI) int { return len(f.revokeCalls) }},
		{"revoke all", []rune{'a', 'y'}, func(f *fakeAPI) int { return f.revokeAllCalls }},
		{"sign out", []rune{'l', 'y'}, func(f *fakeAPI) int { return f.logoutCalls }},
	} {
		f := &fakeAPI{sessionList: twoSessions()}
		m, s := accountPage(t, f)
		var cmd tea.Cmd
		for _, k := range tc.keys {
			m, cmd = press(m, runeKey(k))
		}
		m = feed(m, cmd)
		if tc.call(f) != 1 || m.stage != loginStage || m.token != "" || s.deletes != 1 || !strings.Contains(m.View().Content, "Signed out.") {
			t.Errorf("%s: should end the session and return to sign-in", tc.name)
		}
	}
}

func TestSignOutWarnsWhenTheCredentialStays(t *testing.T) {
	f := &fakeAPI{}
	m, s := accountPage(t, f)
	s.deleteErr = errors.New("keychain locked")
	m, _ = press(m, runeKey('l'))
	m, cmd := press(m, runeKey('y'))
	m = feed(m, cmd)
	if m.stage != loginStage || !strings.Contains(m.message, "could not be removed") {
		t.Fatal("a credential left behind should be reported")
	}
}

func TestUnconfirmedSessionActionKeepsTheWorkspace(t *testing.T) {
	f := &fakeAPI{sessionList: twoSessions(), actionErr: errors.New("offline")}
	m, s := accountPage(t, f)
	m, _ = press(m, runeKey('a'))
	m, cmd := press(m, runeKey('y'))
	m = feed(m, cmd)
	if m.stage != workspaceStage || s.deletes != 0 || m.status.current.level != statusError {
		t.Fatal("an unconfirmed action should keep the session and explain")
	}
}

func TestDeleteAccountNeedsDELETE(t *testing.T) {
	f := &fakeAPI{}
	m, s := accountPage(t, f)
	m, _ = press(m, runeKey('D'))
	if !strings.Contains(plain(m), "30 days") {
		t.Fatal("deletion should explain the restore window")
	}
	m = typeText(m, "delete")
	if m, cmd := press(m, enter); cmd != nil || !strings.Contains(m.workspace.popup.err, "DELETE") {
		t.Fatal("anything but DELETE should be refused")
	}
	m, _ = press(m, esc)
	m, _ = press(m, runeKey('D'))
	m = typeText(m, "DELETE")
	m, cmd := press(m, enter)
	m = feed(m, cmd)
	view := stripStatusANSI.ReplaceAllString(m.View().Content, "")
	if f.deletionCalls != 1 || m.stage != loginStage || s.deletes != 1 || !strings.Contains(view, "Oct 29") || !strings.Contains(view, "restore") {
		t.Fatal("deletion should sign out with the restore deadline")
	}
}

func TestUnconfirmedDeletionKeepsTheAccount(t *testing.T) {
	f := &fakeAPI{deletionErr: errors.New("offline")}
	m, s := accountPage(t, f)
	m, _ = press(m, runeKey('D'))
	m = typeText(m, "DELETE")
	m, cmd := press(m, enter)
	m = feed(m, cmd)
	if m.stage != workspaceStage || s.deletes != 0 || m.workspace.popup == nil || !strings.Contains(m.workspace.popup.err, "not confirmed") {
		t.Fatal("an unconfirmed deletion should keep the session and explain")
	}
}

func TestAccountActionsWaitForTheAccount(t *testing.T) {
	m, _ := accountPage(t, &fakeAPI{meErr: errors.New("offline")})
	for _, k := range []rune{'e', 'x', 'a', 'l', 'D'} {
		if m, _ = press(m, runeKey(k)); m.workspace.popup != nil {
			t.Fatalf("%c acted on an account that has not loaded", k)
		}
	}
}

func TestDemoSignOutStartsOver(t *testing.T) {
	m := demoAt(t, 100, 30)
	m, _ = press(m, runeKey('1'))
	m, _ = press(m, runeKey('l'))
	m, cmd := press(m, runeKey('y'))
	m = settleAll(m, cmd)
	if m.stage != workspaceStage || !m.demo || !strings.Contains(m.status.current.text, "started over") || m.width != 100 {
		t.Fatal("signing out of the demo should start it over")
	}
}
