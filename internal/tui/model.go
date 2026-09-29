package tui

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"shortlog-cli/internal/api"
)

// Client is the Shortlog API the CLI uses: sign-in, and everything the
// workspace reads and changes.
type Client interface {
	loginAPI
	workspaceClient
}

type SessionStore interface {
	Load() (string, error)
	Save(string) error
	Delete() error
}

// stage is the screen on show.
type stage int

const (
	startupStage   stage = iota // loading the saved session
	loginStage                  // the sign-in screen; its steps belong to loginModel
	workspaceStage              // the workspace; see workspace.go
)

// Model is the root of the program. It loads the saved session, shows the
// sign-in screen or the workspace, and owns what they share: the footer's
// status line and shortcuts, the session token, and the terminal size. The
// screens report to it through state it reads after each update.
type Model struct {
	api       Client
	store     SessionStore
	stage     stage
	login     loginModel
	workspace workspaceModel
	status    statusBar
	help      help.Model
	origin    string       // API origin, shown in the status line
	token     string       // never rendered or logged
	resume    *unsentDraft // text to reopen after signing in again
	demo      bool         // started with sample data and no server
	servers   Servers      // nil in the demo, which has no server to change
	// serverPopup, when set, asks for a server address over either screen.
	serverPopup *popupState
	// serverOverride is an address that failed its health check and that the
	// user may connect to anyway by accepting again.
	serverOverride string
	popupKeys      workspaceKeyMap
	busy           bool // a request is running on the sign-in screen, or the saved session is loading
	// message is a notice for the sign-in screen, such as why the session
	// ended; it is too important for the one-line status bar.
	message       string
	width, height int
}

func New(client Client, store SessionStore) Model {
	m := Model{api: client, store: store, stage: startupStage, width: 80, height: 24, busy: true, help: newHelp(), popupKeys: defaultWorkspaceKeys()}
	if origin, ok := client.(interface{ Origin() string }); ok {
		m.origin = origin.Origin()
	}
	m.login = newLogin(client, openTelegramBrowser)
	m.login.setSize(m.width, m.bodyHeight())
	return m
}

func (m Model) Init() tea.Cmd {
	if m.demo {
		return tea.Batch(statusTick(), tea.RequestBackgroundColor)
	}
	return tea.Batch(statusTick(), tea.RequestBackgroundColor, m.loadSession())
}

// loadedSession is the saved session read from the credential store.
type loadedSession struct {
	token string
	err   error
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.login.setSize(m.width, m.bodyHeight())
		m.workspace.setSize(m.width, m.bodyHeight())
		if m.serverPopup != nil {
			m.serverPopup.setSize(m.width, m.bodyHeight())
		}
		if m.stage == loginStage {
			return m.updateLogin(msg)
		}
		return m, nil
	case tea.BackgroundColorMsg:
		// The terminal answered with its background: restyle for it. The
		// sign-in screen keeps it for the Huh forms it builds later.
		setTheme(msg.IsDark())
		m.help = newHelp()
		var cmd tea.Cmd
		m.login, cmd = m.login.Update(msg)
		return m, cmd
	case accountLoaded, projectsLoaded, notesLoaded, noteCreated, noteUpdated, noteDeleted, noteMoved, projectAdded, projectArchived,
		profileUpdated, sessionEnded, accountDeletionRequested:
		if m.stage != workspaceStage {
			return m, nil // a late result for a session that has ended
		}
		return m.updateWorkspace(msg)
	case sessionSaved:
		if msg.err != nil {
			m.setStatus(statusWarn, "Could not save the session to the credential store; you may need to sign in next time.")
		}
		return m, nil
	case statusBeat:
		return m, m.status.beat(m.busy, time.Now())
	case loadedSession:
		return m.onLoadedSession(msg)
	case serverChecked:
		return m.onServerChecked(msg)
	case telegramStartResult, telegramBrowserResult, telegramTick, telegramPollResult, startResult, verifyResult, restoreResult:
		return m.updateLogin(msg)
	case tea.KeyPressMsg:
		m.status.dismiss()
		if msg.String() == "ctrl+c" {
			// Unsaved text gets one question first; a second Ctrl+C quits.
			if m.stage == workspaceStage && m.workspace.unsaved() && (m.workspace.popup == nil || m.workspace.popup.action.kind != quitAction) {
				m.workspace.confirmQuit()
				return m, nil
			}
			return m, tea.Quit
		}
	}
	if m.serverPopup != nil {
		return m.updateServerPopup(message)
	}
	switch m.stage {
	case loginStage:
		return m.updateLogin(message)
	case workspaceStage:
		return m.updateWorkspace(message)
	}
	return m, nil
}

// NewDemo starts straight in the workspace with sample data and no server,
// for trying the layout. Nothing it shows is sent anywhere. The sample data
// is served through the workspace's API interface, and the first loads are
// applied up front so the demo opens ready.
func NewDemo() Model {
	m := New(nil, nil)
	m.demo, m.stage, m.busy, m.token, m.origin = true, workspaceStage, false, "demo", "demo"
	m.workspace = newWorkspace(newDemoAPI(time.Now()), m.token, "demo")
	m.workspace.setSize(m.width, m.bodyHeight())
	m.workspace = settle(m.workspace, m.workspace.start())
	return m
}

// settle runs a command's requests at once and applies their results, for
// the demo, whose API answers locally. Requests those results start are
// settled too.
func settle(w workspaceModel, cmd tea.Cmd) workspaceModel {
	for _, msg := range runCmd(cmd) {
		var next tea.Cmd
		w, next = w.Update(msg)
		w = settle(w, next)
	}
	return w
}

// runCmd runs a command and any commands it batches, returning their
// messages.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var msgs []tea.Msg
	for _, c := range batch {
		msgs = append(msgs, runCmd(c)...)
	}
	return msgs
}

// openWorkspace shows the workspace for a session and starts loading it.
func (m *Model) openWorkspace(token string) tea.Cmd {
	m.token, m.stage, m.busy = token, workspaceStage, false
	m.status.clear()
	m.message = ""
	host := strings.TrimPrefix(strings.TrimPrefix(m.origin, "https://"), "http://")
	m.workspace = newWorkspace(m.api, token, host)
	m.workspace.account.origin = m.origin
	m.workspace.setSize(m.width, m.bodyHeight())
	cmd := m.workspace.start()
	if m.resume != nil {
		cmd = tea.Batch(cmd, m.workspace.resumeDraft(m.resume))
		m.resume = nil
		m.applyWorkspaceNote()
	}
	return cmd
}

// applyWorkspaceNote shows a status the workspace left for the root.
func (m *Model) applyWorkspaceNote() {
	if note := m.workspace.note; note != nil {
		m.workspace.note = nil
		m.setStatus(note.level, note.text)
	}
}

// updateWorkspace runs the workspace and applies what it reports: a status
// change, and an expired session, which signs out.
func (m Model) updateWorkspace(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.workspace, cmd = m.workspace.Update(msg)
	m.applyWorkspaceNote()
	if m.workspace.expired {
		m.expireSession()
		return m, nil
	}
	if m.workspace.wantServer {
		m.workspace.wantServer = false
		m.openServerPopup()
	}
	if out := m.workspace.signOut; out != nil {
		return m, m.endSession(out.notice)
	}
	return m, cmd
}

// expireSession signs out after the server refused the session: the saved
// credential is removed and sign-in explains why.
func (m *Model) expireSession() {
	if m.store != nil {
		_ = m.store.Delete()
	}
	m.token = ""
	m.resume = m.workspace.unsent
	m.workspace = workspaceModel{}
	m.openLogin()
	m.message = "Session expired. Sign in again."
	if m.resume != nil {
		m.message = "Session expired. Sign in again to save your unsaved text."
	}
}

// endSession signs out locally after the server ended the session on
// request: the saved credential goes, and sign-in shows notice. The demo has
// no sign-in to return to, so it starts over instead.
func (m *Model) endSession(notice string) tea.Cmd {
	if m.demo {
		demo := NewDemo()
		demo.width, demo.height = m.width, m.height
		demo.workspace.setSize(m.width, demo.bodyHeight())
		*m = demo
		m.setStatus(statusInfo, "The demo has no server to sign out of, so it started over.")
		return nil
	}
	err := m.store.Delete()
	m.token = ""
	m.workspace = workspaceModel{}
	m.openLogin()
	m.message = notice
	if err != nil {
		m.message += " The saved credential could not be removed; remove it from your credential store before restarting."
	}
	return nil
}

// sessionSaved reports whether the OS credential store kept a new session.
type sessionSaved struct{ err error }

// saveSession stores a new session. A failure leaves nothing half-saved: an
// older session for the same server must not linger.
func (m Model) saveSession(token string) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		err := store.Save(token)
		if err != nil {
			_ = store.Delete()
		}
		return sessionSaved{err}
	}
}

// openLogin shows the sign-in screen from its first step, with no attempt in
// progress. Callers set m.message to leave a notice there, such as why the
// session ended.
func (m *Model) openLogin() {
	m.stage = loginStage
	m.busy = false
	m.login = m.login.reset()
	m.login.setSize(m.width, m.bodyHeight())
}

// updateLogin runs the sign-in screen and applies what it reports: a status
// change, and on success the session token, which leaves the screen.
func (m Model) updateLogin(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.login, cmd = m.login.Update(msg)
	if m.stage != loginStage {
		// A late result for a finished attempt; the screen ignores it, and
		// nothing it reports concerns the screen now shown.
		m.login.note, m.login.token = nil, ""
		return m, nil
	}
	m.busy = m.login.busy
	if note := m.login.note; note != nil {
		m.login.note = nil
		if note.text == "" {
			m.clearStatus()
		} else {
			m.setStatus(note.level, note.text)
		}
	}
	if m.login.wantServer {
		m.login.wantServer = false
		m.openServerPopup()
	}
	if token := m.login.token; token != "" {
		m.login = m.login.reset()
		return m, m.signedIn(token)
	}
	return m, cmd
}

func (m Model) onLoadedSession(msg loadedSession) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.openLogin()
		m.setStatus(statusWarn, "Credential store unavailable; this session may not be saved.")
		return m, nil
	}
	if msg.token == "" {
		m.openLogin()
		return m, nil
	}
	// The workspace checks the saved session as it loads: a 401 signs out
	// and clears it, while a network failure keeps it for a retry.
	return m, m.openWorkspace(msg.token)
}

// signedIn opens the workspace for a new session and saves it.
func (m *Model) signedIn(token string) tea.Cmd {
	return tea.Batch(m.openWorkspace(token), m.saveSession(token))
}

// setStatus shows a message on the status line and drops any body message, so
// the two never disagree about what is current.
func (m *Model) setStatus(level statusLevel, text string) {
	m.message = ""
	m.status.set(level, text, time.Now())
}

func (m *Model) clearStatus() {
	m.message = ""
	m.status.clear()
}

// footerHeight reserves two rows for shortcuts and status, or only the status
// row on very short terminals.
func (m Model) footerHeight() int {
	if m.height < 8 {
		return 1
	}
	return 2
}

// cardLoading reports whether a sign-in card is showing its own loader.
func (m Model) cardLoading() bool {
	return m.stage == loginStage && m.login.cardLoading()
}

func (m Model) bodyHeight() int { return max(1, m.height-m.footerHeight()) }

// footer renders the shortcut line above the status line.
func (m Model) footer() string {
	status := m.status.view(m.width, m.busy, m.statusContext())
	if m.cardLoading() {
		// The card already shows the spinner and what is happening, so drop
		// the footer's spinner and progress text. Problems still show here.
		quiet := m.status
		if quiet.current.level == statusInfo {
			quiet.clear()
		}
		status = quiet.view(m.width, false, m.statusContext())
	}
	if m.footerHeight() == 1 {
		return status
	}
	h := m.help
	h.SetWidth(max(1, m.width-2*statusGutter))
	// help drops trailing hints to fit, but when even its ellipsis would not
	// fit it keeps the next hint anyway; cut the line so it never overflows.
	line := ansi.Truncate(h.ShortHelpView(m.shortcuts()), h.Width(), "…")
	shortcuts := lipgloss.NewStyle().PaddingLeft(statusGutter).Render(line)
	return shortcuts + "\n" + status
}

// statusContext names who is signed in and where, for the status line's
// right edge.
func (m Model) statusContext() string {
	host := strings.TrimPrefix(strings.TrimPrefix(m.origin, "https://"), "http://")
	name := m.workspace.userName()
	if m.token != "" && name != "" {
		if host == "" {
			return preview(name, 24)
		}
		return preview(name, 24) + " · " + host
	}
	return host
}

func cursorValue(cursor *string) string {
	if cursor == nil {
		return ""
	}
	return *cursor
}

// Notes are untrusted terminal text; remove control sequences before rendering.
func safeText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

func preview(text string, limit int) string {
	first, _, _ := strings.Cut(safeText(text), "\n")
	first = strings.TrimSpace(first)
	runes := []rune(first)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return first
}

func eightDigits(code string) bool {
	if len(code) != 8 {
		return false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func friendlyError(err error, fallback string) string {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "rate_limited":
			return "Too many attempts. Wait before trying again."
		case "service_unavailable":
			return "Email sign-in is temporarily unavailable."
		case "invalid_request":
			return "Invalid or expired code or details. Check them and try again."
		}
		if apiErr.Status == http.StatusUnprocessableEntity {
			return "Check your account details and try again."
		}
	}
	return fallback + " Check your connection and try again."
}

// View declares the screen: its content and the full-screen alternate
// buffer. Terminal features live here in Bubble Tea v2, not in options.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Shortlog"
	if m.stage == loginStage {
		v.WindowTitle = "Shortlog · Sign in"
	}
	// Terminals that support it (such as Windows Terminal and Ghostty) show
	// their own progress indicator in the tab while a request runs or while
	// Telegram approval is pending; others ignore it.
	if m.busy || (m.stage == loginStage && m.login.step == telegramStep) || (m.stage == workspaceStage && m.workspace.loading()) {
		v.ProgressBar = tea.NewProgressBar(tea.ProgressBarIndeterminate, 0)
	}
	return v
}

// render draws the current screen above the footer.
func (m Model) render() string {
	var body string
	switch m.stage {
	case loginStage:
		body = m.login.View(m.message, m.status.spinner())
	case workspaceStage:
		body = m.workspace.View(m.status.spinner())
	default:
		body = lipgloss.Place(max(m.width, 1), m.bodyHeight(), lipgloss.Center, lipgloss.Center, dimStyle.Render("Opening your saved session…"))
	}
	body = lipgloss.NewStyle().Height(m.bodyHeight()).MaxHeight(m.bodyHeight()).Render(body)
	if m.serverPopup != nil {
		body = overlay(body, m.serverPopup.View(m.width, m.bodyHeight(), m.status.spinner()), max(m.width, 1), m.bodyHeight())
	}
	return body + "\n" + m.footer()
}
