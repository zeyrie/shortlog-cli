package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeServers hands out a fake client and store per server address.
type fakeServers struct {
	clients     map[string]*fakeAPI
	stores      map[string]*fakeStore
	checkErr    error
	checks      []string
	remembered  []string
	rememberErr error
}

func newFakeServers() *fakeServers {
	return &fakeServers{clients: map[string]*fakeAPI{}, stores: map[string]*fakeStore{}}
}

func (f *fakeServers) Connect(origin string) (Client, SessionStore, error) {
	if f.clients[origin] == nil {
		f.clients[origin], f.stores[origin] = &fakeAPI{}, &fakeStore{}
	}
	return f.clients[origin], f.stores[origin], nil
}

func (f *fakeServers) Check(_ context.Context, origin string) error {
	f.checks = append(f.checks, origin)
	return f.checkErr
}

func (f *fakeServers) Remember(origin string) error {
	f.remembered = append(f.remembered, origin)
	return f.rememberErr
}

const localServer = "http://127.0.0.1:8080"

// serverModel is a signed-out model on the local server that can switch.
func serverModel(t *testing.T) (Model, *fakeServers, *fakeStore) {
	t.Helper()
	servers := newFakeServers()
	store := &fakeStore{}
	m := New(&fakeAPI{}, store).WithServers(servers)
	m.origin, m.stage, m.busy = localServer, loginStage, false
	return m, servers, store
}

// connectTo opens the server popup, enters an address, and accepts it.
func connectTo(m Model, address string) (Model, bool) {
	m, _ = press(m, runeKey('s'))
	if m.serverPopup == nil {
		return m, false
	}
	m.serverPopup.fields[0].input.SetValue(address)
	m, cmd := press(m, enter)
	return settleAll(m, cmd), true
}

func TestSwitchServerFromSignIn(t *testing.T) {
	m, servers, _ := serverModel(t)
	m, _ = press(m, runeKey('s'))
	if m.serverPopup == nil || m.serverPopup.value() != localServer || !strings.Contains(plain(m), "https://") {
		t.Fatal("s on the sign-in screen should open the server popup with the current address")
	}
	m, _ = press(m, esc)
	m, ok := connectTo(m, " https://notes.example/ ")
	if !ok || m.origin != "https://notes.example" || m.stage != loginStage || m.serverPopup != nil {
		t.Fatalf("switch: origin %q, stage %d", m.origin, m.stage)
	}
	if len(servers.checks) != 1 || len(servers.remembered) != 1 || servers.remembered[0] != "https://notes.example" {
		t.Fatalf("checks %v, remembered %v", servers.checks, servers.remembered)
	}
	if !strings.Contains(plain(m), "notes.example") {
		t.Fatal("the new server should show in the status line")
	}
}

func TestSwitchServerFromTheAccountPageKeepsTheOldSession(t *testing.T) {
	servers := newFakeServers()
	servers.clients["https://notes.example"] = &fakeAPI{}
	servers.stores["https://notes.example"] = &fakeStore{token: "other-session"}
	old := &fakeStore{token: "local-session"}
	m := New(&fakeAPI{}, old).WithServers(servers)
	m.origin, m.stage, m.busy = localServer, loginStage, false
	m = settleAll(m, m.signedIn("local-session"))
	m, _ = press(m, runeKey('1'))
	if !strings.Contains(plain(m), localServer) {
		t.Fatal("the account page should show the server address")
	}
	m, _ = connectTo(m, "https://notes.example")
	if m.stage != workspaceStage || m.token != "other-session" {
		t.Fatalf("the new server's saved session should open the workspace: stage %d", m.stage)
	}
	if old.deletes != 0 || old.token != "local-session" {
		t.Fatal("switching should keep the old server's session saved")
	}
}

func TestUnreachableServerNeedsASecondConfirmation(t *testing.T) {
	m, servers, _ := serverModel(t)
	servers.checkErr = errors.New("connection refused")
	m, _ = connectTo(m, "https://down.example")
	if m.origin != localServer || m.serverPopup == nil || !strings.Contains(m.serverPopup.err, "did not answer") {
		t.Fatal("an unreachable server should not be switched to straight away")
	}
	m, cmd := press(m, enter)
	m = settleAll(m, cmd)
	if m.origin != "https://down.example" || len(servers.checks) != 1 {
		t.Fatalf("accepting again should connect without checking again: origin %q, checks %v", m.origin, servers.checks)
	}
}

func TestServerAddressIsValidatedLocally(t *testing.T) {
	m, servers, _ := serverModel(t)
	m, _ = connectTo(m, "notes.example")
	if m.serverPopup == nil || !strings.Contains(m.serverPopup.err, "http(s)") || len(servers.checks) != 0 {
		t.Fatal("an address without a scheme should be refused before any request")
	}
	m, _ = press(m, esc)
	m, _ = connectTo(m, localServer+"/")
	if m.serverPopup != nil || m.status.current.text != "Already connected to that server." || len(servers.checks) != 0 {
		t.Fatal("the current server should not be switched to again")
	}
}

func TestPlainHTTPServersWarn(t *testing.T) {
	m, _, _ := serverModel(t)
	m, _ = connectTo(m, "http://notes.example")
	if m.status.current.level != statusWarn || !strings.Contains(m.status.current.text, "plain HTTP") {
		t.Fatalf("a remote plain-HTTP server should warn: %q", m.status.current.text)
	}
	for origin, want := range map[string]bool{"http://localhost:8080": false, "http://127.0.0.1:8080": false, "http://[::1]:8080": false, "http://notes.example": true, "https://notes.example": false} {
		if insecure(origin) != want {
			t.Errorf("insecure(%q) = %v", origin, !want)
		}
	}
}

func TestUnsavedServerChoiceWarns(t *testing.T) {
	m, servers, _ := serverModel(t)
	servers.rememberErr = errors.New("read-only")
	m, _ = connectTo(m, "https://notes.example")
	if m.origin != "https://notes.example" || !strings.Contains(m.status.current.text, "this run only") {
		t.Fatal("a server that could not be saved should still connect, with a warning")
	}
}

func TestDemoHasNoServerToChange(t *testing.T) {
	m := demoAt(t, 100, 30)
	m, _ = press(m, runeKey('1'))
	m, _ = press(m, runeKey('s'))
	if m.serverPopup != nil || !strings.Contains(m.status.current.text, "demo") {
		t.Fatal("the demo should explain it has no server to change")
	}
}
