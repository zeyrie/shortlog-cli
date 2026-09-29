package tui

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"shortlog-cli/internal/api"
)

// Servers connects the CLI to a Shortlog server. The program wires it to the
// real API client, the per-server session store, and the settings file;
// tests fake it.
type Servers interface {
	// Connect makes the client and the session store for a server. Each
	// server keeps its own saved session.
	Connect(origin string) (Client, SessionStore, error)
	// Check asks the server whether it is up, without a session.
	Check(ctx context.Context, origin string) error
	// Remember makes the server the one the CLI opens with next time.
	Remember(origin string) error
}

// WithServers lets the user change servers from the sign-in screen and the
// account page.
func (m Model) WithServers(s Servers) Model {
	m.servers = s
	return m
}

// WithWarning starts the program with a warning on the status line, such as
// a settings file that could not be used.
func (m Model) WithWarning(text string) Model {
	m.setStatus(statusWarn, text)
	return m
}

// serverChecked is the answer to a server's health check.
type serverChecked struct {
	origin string
	err    error
}

// openServerPopup asks for the server to connect to. It can be opened from
// the sign-in screen, so a wrong or unreachable address never locks the user
// out, and from the account page.
func (m *Model) openServerPopup() {
	if m.servers == nil {
		m.setStatus(statusInfo, "The demo has no server to change.")
		return
	}
	field := newField("Address", "https://notes.example.com", m.origin, 256, validateServerURL)
	message := "The Shortlog server to connect to. Sign-in codes and sessions travel over it, so use https:// for anything but your own machine."
	m.serverPopup = newForm("Server", message, "connect", false, []popupField{field}, pendingAction{kind: serverAction})
	m.serverPopup.setSize(m.width, m.bodyHeight())
	m.serverOverride = ""
}

func (m Model) updateServerPopup(msg tea.Msg) (tea.Model, tea.Cmd) {
	p, cmd, outcome := m.serverPopup.Update(msg, m.popupKeys)
	m.serverPopup = &p
	switch outcome {
	case popupCancelled:
		m.serverPopup = nil
	case popupAccepted:
		origin := canonicalOrigin(p.value())
		switch origin {
		case m.origin:
			m.serverPopup = nil
			m.setStatus(statusInfo, "Already connected to that server.")
			return m, nil
		case m.serverOverride:
			// The user asked to connect even though it did not answer.
			return m.switchServer(origin)
		}
		m.serverPopup.busy = true
		servers := m.servers
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return serverChecked{origin: origin, err: servers.Check(ctx, origin)}
		}
	}
	return m, cmd
}

func (m Model) onServerChecked(msg serverChecked) (tea.Model, tea.Cmd) {
	p := m.serverPopup
	if p == nil || canonicalOrigin(p.value()) != msg.origin {
		return m, nil // the popup closed or its address changed meanwhile
	}
	if msg.err != nil {
		p.busy = false
		p.err = "That server " + unreachable(msg.err) + ". Press Enter to connect anyway, or Esc to stay."
		m.serverOverride = msg.origin
		return m, nil
	}
	return m.switchServer(msg.origin)
}

// switchServer connects to another server. The current server's saved
// session stays in the credential store for switching back; the new
// server's saved session, if any, opens the workspace, and otherwise the
// sign-in screen shows.
func (m Model) switchServer(origin string) (tea.Model, tea.Cmd) {
	client, store, err := m.servers.Connect(origin)
	if err != nil {
		m.serverPopup.busy, m.serverPopup.err = false, err.Error()
		return m, nil
	}
	background := m.login.background
	m.serverPopup, m.serverOverride = nil, ""
	m.api, m.store, m.origin = client, store, origin
	m.token, m.resume, m.message = "", nil, ""
	m.workspace = workspaceModel{}
	m.login = newLogin(client, openTelegramBrowser)
	m.login.background = background
	m.login.setSize(m.width, m.bodyHeight())
	m.stage, m.busy = startupStage, true
	m.setStatus(statusInfo, "Connected to "+hostOf(origin)+".")
	if err := m.servers.Remember(origin); err != nil {
		m.setStatus(statusWarn, "Connected, but the address could not be saved; it applies to this run only.")
	}
	if insecure(origin) {
		m.setStatus(statusWarn, "This server uses plain HTTP: sign-in codes and sessions are not encrypted.")
	}
	return m, m.loadSession()
}

// loadSession reads the saved session for the current server.
func (m Model) loadSession() tea.Cmd {
	store := m.store
	return func() tea.Msg {
		token, err := store.Load()
		return loadedSession{token, err}
	}
}

func validateServerURL(value string) error {
	if canonicalOrigin(value) == "" {
		return errors.New("enter an http(s) address such as https://notes.example.com")
	}
	return nil
}

// canonicalOrigin is the address as the API client would use it, or empty if
// it is not a valid server address.
func canonicalOrigin(value string) string {
	client, err := api.NewClient(strings.TrimSpace(value), nil)
	if err != nil {
		return ""
	}
	return client.Origin()
}

func hostOf(origin string) string {
	return strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
}

// insecure reports whether an address sends traffic unencrypted beyond this
// machine.
func insecure(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

// unreachable describes a failed health check for the popup.
func unreachable(err error) string {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		return "answered, but not as a healthy Shortlog server (HTTP " + strconv.Itoa(apiErr.Status) + ")"
	}
	return "did not answer"
}
