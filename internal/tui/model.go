package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"shortlog-cli/internal/api"
)

type emailAPI interface {
	StartEmail(context.Context, string) (string, error)
	StartTelegram(context.Context) (api.TelegramStart, error)
	PollTelegram(context.Context, string, string, string, string) (api.VerifyResult, error)
	RestoreTelegram(context.Context, string) (string, error)
	VerifyEmail(context.Context, string, string, string, string) (api.VerifyResult, error)
	RestoreEmail(context.Context, string) (string, error)
	Me(context.Context, string) (api.Account, error)
	UpdateProfile(context.Context, string, string, string) (api.Account, error)
	RequestAccountDeletion(context.Context, string) (time.Time, error)
	Sessions(context.Context, string) ([]api.Session, error)
	Logout(context.Context, string) error
	RevokeSession(context.Context, string, string) error
	RevokeAllSessions(context.Context, string) error
	Inbox(context.Context, string) (api.NotesPage, error)
	InboxPage(context.Context, string, string) (api.NotesPage, error)
	CreateInboxNote(context.Context, string, string) (api.Note, error)
	Projects(context.Context, string) ([]api.Project, error)
	ArchivedProjects(context.Context, string) ([]api.Project, error)
	ArchiveProject(context.Context, string, string) error
	UnarchiveProject(context.Context, string, string) error
	CreateProject(context.Context, string, string) (api.Project, error)
	ProjectNotes(context.Context, string, string) (api.NotesPage, error)
	ProjectNotesPage(context.Context, string, string, string) (api.NotesPage, error)
	CreateProjectNote(context.Context, string, string, string) (api.Note, error)
	UpdateNote(context.Context, string, string, string) (api.Note, error)
	MoveNote(context.Context, string, string, string) (api.Note, error)
	DeleteNote(context.Context, string, string) error
}

type sessionStore interface {
	Load() (string, error)
	Save(string) error
	Delete() error
}

type stage int

const (
	startupStage stage = iota
	loginStage         // the sign-in screen; its steps belong to loginModel
	inboxStage
	readingStage
	captureStage
	discardStage
	deleteStage
	projectsStage
	newProjectStage
	moveStage
	archiveStage
	sessionsStage
	sessionConfirmStage
	accountStage
	accountEditStage
	accountDiscardStage
	accountDeleteStage
	workspaceStage // the lazygit-style workspace; see workspace.go
)

type sessionAction int

const (
	revokeOne sessionAction = iota
	revokeAll
	logout
)

type moveTarget struct {
	id   string // Empty ID represents the Inbox.
	name string
}

// noteList snapshots the Inbox while a project's notes are open.
type noteList struct {
	notes    []api.Note
	cursor   string
	selected int
}

type Model struct {
	api               emailAPI
	store             sessionStore
	stage             stage
	inputs            [4]textinput.Model
	focus             int
	status            statusBar
	help              help.Model
	origin            string // API origin, shown in the status line
	login             loginModel
	workspace         workspaceModel
	resume            *unsentDraft // text to reopen after signing in again
	demo              bool         // started with sample data and no server
	token             string       // Never rendered or logged.
	username          string
	account           api.Account
	accountBack       stage
	accountStartName  string
	accountStartZone  string
	notes             []api.Note
	inboxList         noteList
	activeProject     *api.Project
	inboxStashed      bool
	inboxNeedsRefresh bool
	projects          []api.Project
	projectSelected   int
	showArchived      bool
	archiveID         string
	sessions          []api.Session
	sessionSelected   int
	sessionBack       stage
	sessionAction     sessionAction
	sessionID         string
	nextCursor        string
	selected          int
	reader            viewport.Model
	draft             textarea.Model
	editorID          string
	editorStart       string
	editorBack        stage
	deleteID          string
	deleteBack        stage
	moveID            string
	moveBack          stage
	moveTargets       []moveTarget
	moveSelected      int
	quitAfterDiscard  bool
	resumeDraft       bool
	selectID          string
	busy              bool
	message           string
	width             int
	height            int
}

const (
	projectNameInput = iota
	accountNameInput
	accountZoneInput
	deletePhraseInput
)

func New(client emailAPI, store sessionStore) Model {
	m := Model{api: client, store: store, stage: startupStage, width: 80, height: 24, busy: true, reader: viewport.New(viewport.WithWidth(64), viewport.WithHeight(14))}
	m.help = newHelp()
	m.login = newLogin(client, openTelegramBrowser)
	m.login.setSize(m.width, m.bodyHeight())
	if origin, ok := client.(interface{ Origin() string }); ok {
		m.origin = origin.Origin()
	}
	m.draft = textarea.New()
	m.draft.Placeholder = "What's on your mind?"
	m.draft.CharLimit = 20000
	m.draft.ShowLineNumbers = false
	m.draft.Prompt = ""
	// The default focused-line background is black in dark terminals.
	draftStyles := m.draft.Styles()
	draftStyles.Focused.CursorLine = lipgloss.NewStyle()
	m.draft.SetStyles(draftStyles)
	m.draft.SetWidth(m.innerWidth())
	m.draft.SetHeight(12)
	for i := range m.inputs {
		m.inputs[i] = textinput.New()
		m.inputs[i].CharLimit = 320
		m.inputs[i].SetWidth(42)
	}
	m.inputs[projectNameInput].Placeholder = "Project name"
	m.inputs[projectNameInput].CharLimit = 120
	m.inputs[accountNameInput].Placeholder = "Your name"
	m.inputs[accountNameInput].CharLimit = 80
	m.inputs[accountZoneInput].Placeholder = "e.g. Europe/London"
	m.inputs[accountZoneInput].CharLimit = 64
	m.inputs[deletePhraseInput].Placeholder = "Type DELETE"
	m.inputs[deletePhraseInput].CharLimit = 6
	m.focusInput(projectNameInput)
	return m
}

func (m Model) Init() tea.Cmd {
	if m.demo {
		return tea.Batch(statusTick(), tea.RequestBackgroundColor)
	}
	return tea.Batch(textinput.Blink, statusTick(), tea.RequestBackgroundColor, func() tea.Msg {
		token, err := m.store.Load()
		return loadedSession{token, err}
	})
}

type startResult struct {
	id  string
	err error
}
type verifyResult struct {
	result api.VerifyResult
	err    error
}
type restoreResult struct {
	token string
	err   error
}
type telegramStartResult struct {
	start api.TelegramStart
	err   error
}
type telegramPollResult struct {
	attempt string
	result  api.VerifyResult
	err     error
}
type telegramTick struct {
	attempt    string
	generation uint64
}
type telegramBrowserResult struct {
	attempt string
	err     error
}
type loadedSession struct {
	token string
	err   error
}
type inboxResult struct {
	account api.Account
	page    api.NotesPage
	err     error
	saveErr error
}
type sessionsResult struct {
	sessions []api.Session
	err      error
}
type sessionActionResult struct {
	action sessionAction
	id     string
	err    error
}
type accountResult struct {
	account api.Account
	err     error
}
type profileResult struct {
	account api.Account
	err     error
}
type deletionResult struct {
	deadline time.Time
	err      error
}
type createdNote struct {
	note api.Note
	err  error
}
type updatedNote struct {
	note api.Note
	err  error
}
type deletedNote struct {
	id  string
	err error
}
type olderNotes struct {
	page   api.NotesPage
	cursor string
	err    error
}
type projectsResult struct {
	projects []api.Project
	err      error
}
type projectArchiveResult struct {
	id       string
	archived bool
	err      error
}
type projectCreated struct {
	project api.Project
	err     error
}
type movedNote struct {
	note        api.Note
	destination moveTarget
	err         error
}

func (m Model) fetchNotes(ctx context.Context, token string) (api.NotesPage, error) {
	if m.activeProject != nil {
		return m.api.ProjectNotes(ctx, token, m.activeProject.ID)
	}
	return m.api.Inbox(ctx, token)
}

func (m Model) fetchOlder(ctx context.Context, token, cursor string) (api.NotesPage, error) {
	if m.activeProject != nil {
		return m.api.ProjectNotesPage(ctx, token, m.activeProject.ID, cursor)
	}
	return m.api.InboxPage(ctx, token, cursor)
}

func (m Model) fetchProjects() tea.Cmd {
	return func() tea.Msg {
		var projects []api.Project
		var err error
		if m.stage == projectsStage && m.showArchived {
			projects, err = m.api.ArchivedProjects(context.Background(), m.token)
		} else {
			projects, err = m.api.Projects(context.Background(), m.token)
		}
		return projectsResult{projects, err}
	}
}

func (m Model) fetchSessions() tea.Cmd {
	return func() tea.Msg {
		items, err := m.api.Sessions(context.Background(), m.token)
		return sessionsResult{items, err}
	}
}

func (m *Model) openSessions(back stage) tea.Cmd {
	m.sessionBack = back
	m.stage = sessionsStage
	m.busy = true
	m.sessions = nil
	m.sessionSelected = 0
	m.message = "Loading sessions…"
	return m.fetchSessions()
}

func (m *Model) finishSession() {
	err := m.store.Delete()
	m.token = ""
	m.notes = nil
	m.projects = nil
	m.sessions = nil
	m.activeProject = nil
	m.account = api.Account{}
	m.username = ""
	m.inboxList = noteList{}
	m.inboxStashed = false
	m.inboxNeedsRefresh = false
	m.showArchived = false
	m.openLogin()
	m.message = "Signed out."
	if err != nil {
		m.message = "Signed out on the server, but the saved credential could not be removed. Remove it from your credential store before restarting."
	}
}

func (m *Model) openAccount(back stage) tea.Cmd {
	m.accountBack = back
	m.account = api.Account{}
	m.stage = accountStage
	m.busy = true
	m.message = "Loading account…"
	return m.fetchAccount()
}

func (m Model) fetchAccount() tea.Cmd {
	token := m.token
	return func() tea.Msg {
		account, err := m.api.Me(context.Background(), token)
		return accountResult{account, err}
	}
}

func (m *Model) editAccount() tea.Cmd {
	m.accountStartName, m.accountStartZone = m.account.Username, m.account.TimeZone
	m.inputs[accountNameInput].SetValue(m.accountStartName)
	m.inputs[accountZoneInput].SetValue(m.accountStartZone)
	m.stage = accountEditStage
	m.message = ""
	m.focusInput(accountNameInput)
	return textinput.Blink
}

func (m Model) accountChanged() bool {
	return m.inputs[accountNameInput].Value() != m.accountStartName || m.inputs[accountZoneInput].Value() != m.accountStartZone
}

func (m Model) loadInbox(token string, save bool) tea.Cmd {
	return func() tea.Msg {
		var saveErr error
		if save {
			saveErr = m.store.Save(token)
			if saveErr != nil {
				// Do not leave an older session saved under the same origin.
				_ = m.store.Delete()
			}
		}
		account, err := m.api.Me(context.Background(), token)
		if err != nil {
			return inboxResult{err: err, saveErr: saveErr}
		}
		page, err := m.fetchNotes(context.Background(), token)
		return inboxResult{account: account, page: page, err: err, saveErr: saveErr}
	}
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.login.setSize(m.width, m.bodyHeight())
		m.workspace.setSize(m.width, m.bodyHeight())
		m.resizeReader()
		m.draft.SetWidth(m.innerWidth())
		m.draft.SetHeight(max(3, m.height-10))
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
	case accountLoaded, projectsLoaded, notesLoaded, noteCreated, noteUpdated, noteDeleted, noteMoved, projectAdded, projectArchived:
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
	case telegramStartResult, telegramBrowserResult, telegramTick, telegramPollResult, startResult, verifyResult, restoreResult:
		return m.updateLogin(msg)
	case accountResult:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				m.finishSession()
				if m.message == "Signed out." {
					m.message = "Session expired. Sign in again."
				}
				return m, nil
			}
			m.message = "Could not load account. Press r to retry or Esc to return."
			return m, nil
		}
		m.account = msg.account
		m.username = msg.account.Username
		m.message = ""
		return m, nil
	case profileResult:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				m.finishSession()
				if m.message == "Signed out." {
					m.message = "Session expired. Sign in again."
				}
				return m, nil
			}
			if errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request" {
				m.message = "Server rejected the profile. Check the name and time zone."
			} else {
				m.message = "Update not confirmed. Press Esc to review your account before retrying."
			}
			return m, nil
		}
		m.account = msg.account
		m.username = msg.account.Username
		m.inputs[m.focus].Blur()
		m.stage = accountStage
		m.message = "Profile updated."
		return m, nil
	case deletionResult:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				m.finishSession()
				if m.message == "Signed out." {
					m.message = "Session expired. Sign in again."
				}
				return m, nil
			}
			m.message = "Deletion not confirmed. Check your account before trying again."
			return m, nil
		}
		m.finishSession()
		warning := ""
		if m.message != "Signed out." {
			warning = " The saved credential could not be removed; remove it from your credential store before restarting."
		}
		m.message = "Deletion scheduled for " + msg.deadline.Local().Format("Jan 02, 2006 15:04") + ". All devices were signed out. Sign in with the same identity before then to restore your account." + warning
		return m, nil
	case sessionsResult:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				m.finishSession()
				if m.message == "Signed out." {
					m.message = "Session expired. Sign in again."
				}
				return m, nil
			}
			m.message = "Could not load sessions. Press r to retry or Esc to return."
			return m, nil
		}
		m.sessions = msg.sessions
		if m.sessionSelected >= len(m.sessions) {
			m.sessionSelected = max(0, len(m.sessions)-1)
		}
		m.message = ""
		return m, nil
	case sessionActionResult:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				m.finishSession()
				if m.message == "Signed out." {
					m.message = "Session expired. Sign in again."
				}
				return m, nil
			}
			m.stage = sessionsStage
			m.message = "Action not confirmed. Press r to refresh sessions before retrying."
			return m, nil
		}
		if msg.action == logout || msg.action == revokeAll || (msg.action == revokeOne && m.currentSessionID() == msg.id) {
			m.finishSession()
			return m, nil
		}
		m.stage = sessionsStage
		for i, item := range m.sessions {
			if item.ID == msg.id {
				m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
				if m.sessionSelected >= len(m.sessions) && m.sessionSelected > 0 {
					m.sessionSelected--
				}
				break
			}
		}
		m.message = "Session revoked."
		return m, nil
	case projectsResult:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.activeProject = nil
				m.openLogin()
				m.message = "Session expired. Sign in again."
				return m, nil
			}
			if m.stage == moveStage {
				m.message = "Could not load destinations. Press r to retry or Esc to cancel."
			} else {
				m.message = "Could not load projects. Press r to retry."
			}
			return m, nil
		}
		if m.stage == moveStage {
			m.moveTargets = nil
			if m.activeProject != nil {
				m.moveTargets = append(m.moveTargets, moveTarget{name: "Inbox"})
			}
			for _, project := range msg.projects {
				if m.activeProject == nil || project.ID != m.activeProject.ID {
					m.moveTargets = append(m.moveTargets, moveTarget{id: project.ID, name: project.Name})
				}
			}
			m.moveSelected = 0
			m.message = ""
			return m, nil
		}
		m.projects = msg.projects
		if m.projectSelected >= len(m.projects) {
			m.projectSelected = max(0, len(m.projects)-1)
		}
		m.message = ""
		return m, nil
	case projectArchiveResult:
		m.busy = false
		m.stage = projectsStage
		m.archiveID = ""
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.openLogin()
				m.message = "Session expired. Sign in again."
				return m, nil
			}
			m.message = "Project change not confirmed. Press r to refresh the list before retrying."
			return m, nil
		}
		for i, project := range m.projects {
			if project.ID == msg.id {
				m.projects = append(m.projects[:i], m.projects[i+1:]...)
				if m.projectSelected >= len(m.projects) && m.projectSelected > 0 {
					m.projectSelected--
				}
				break
			}
		}
		if msg.archived {
			m.message = "Project archived. Its notes are now read-only; press t to view archived projects."
		} else {
			m.message = "Project unarchived. Press t to view active projects."
		}
		return m, nil
	case projectCreated:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.openLogin()
				m.message = "Session expired. Sign in again."
				return m, nil
			}
			if errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request" {
				m.message = "Project name must be 1–120 characters."
			} else {
				m.message = "Creation not confirmed. Check the project list before retrying."
			}
			return m, nil
		}
		m.projects = append([]api.Project{msg.project}, m.projects...)
		m.projectSelected = 0
		m.inputs[projectNameInput].SetValue("")
		m.inputs[projectNameInput].Blur()
		return m, m.openProject(msg.project)
	case inboxResult:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.openLogin()
				m.message = "Session expired. Sign in again."
				return m, nil
			}
			m.stage = inboxStage
			if m.selectID != "" {
				m.message = "Note saved, but notes refresh failed. Press r to retry."
			} else {
				m.message = friendlyError(msg.err, "Could not load notes.") + " Press r to retry."
			}
			return m, nil
		}
		m.stage = inboxStage
		m.username = msg.account.Username
		m.account = msg.account
		m.notes = msg.page.Items
		m.nextCursor = cursorValue(msg.page.NextCursor)
		m.selected = 0
		m.message = ""
		if m.selectID != "" {
			for i, note := range m.notes {
				if note.ID == m.selectID {
					m.selected = i
					break
				}
			}
			m.selectID = ""
			m.message = "Note saved."
		}
		if msg.saveErr != nil {
			m.message = "Could not save session to the OS credential store; you may need to sign in next time."
		}
		if m.resumeDraft {
			m.resumeDraft = false
			m.stage = captureStage
			m.message = "Sign-in complete. Your unsaved draft is ready."
			return m, m.draft.Focus()
		}
		return m, nil
	case olderNotes:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.openLogin()
				m.message = "Session expired. Sign in again."
				return m, nil
			}
			m.message = "Could not load older notes. Press m to retry; loaded notes are unchanged."
			return m, nil
		}
		seen := make(map[string]bool, len(m.notes))
		for _, note := range m.notes {
			seen[note.ID] = true
		}
		firstNew := len(m.notes)
		for _, note := range msg.page.Items {
			if !seen[note.ID] {
				seen[note.ID] = true
				m.notes = append(m.notes, note)
			}
		}
		m.nextCursor = cursorValue(msg.page.NextCursor)
		if m.nextCursor == msg.cursor {
			m.nextCursor = ""
			m.message = "Server repeated a page cursor; stopped loading older notes. Refresh to start over."
			return m, nil
		}
		m.message = ""
		if firstNew < len(m.notes) {
			m.selected = firstNew
		} else if m.nextCursor == "" {
			m.message = "No more notes."
		}
		return m, nil
	case createdNote:
		if msg.err != nil {
			m.busy = false
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.resumeDraft = true
				m.openLogin()
				m.message = "Session expired. Sign in again to resume your draft."
				return m, nil
			}
			if errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request" {
				m.message = "The server rejected this note. Check its content and try again."
			} else {
				m.message = "Save not confirmed. Check Inbox before retrying to avoid a duplicate. Your draft is still here."
			}
			return m, nil
		}
		m.stage = inboxStage
		m.selectID = msg.note.ID
		m.notes = append([]api.Note{msg.note}, m.notes...)
		m.selected = 0
		m.draft.SetValue("")
		m.draft.Blur()
		m.message = "Note saved. Refreshing notes…"
		return m, m.loadInbox(m.token, false)
	case updatedNote:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.resumeDraft = true
				m.editorBack = inboxStage
				m.openLogin()
				m.message = "Session expired. Sign in again to resume your unsaved edits."
				return m, nil
			}
			if errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request" {
				m.message = "The server rejected these changes. Check the note and try again."
			} else if errors.As(msg.err, &apiErr) && apiErr.Code == "not_found" {
				m.message = "This note is no longer available. Copy your draft before leaving the editor."
			} else {
				m.message = "Update not confirmed. Check the note before retrying; your draft is still here."
			}
			return m, nil
		}
		for i := range m.notes {
			if m.notes[i].ID == msg.note.ID {
				m.notes[i] = msg.note
				m.selected = i
				break
			}
		}
		m.stage = m.editorBack
		m.editorID = ""
		m.draft.Blur()
		if m.stage == readingStage {
			m.resizeReader()
		}
		m.message = "Note updated."
		return m, nil
	case deletedNote:
		m.busy = false
		m.stage = inboxStage
		m.deleteID = ""
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.openLogin()
				m.message = "Session expired. Sign in again."
				return m, nil
			}
			m.message = "Delete not confirmed. Refresh Inbox before trying again."
			return m, nil
		}
		for i, note := range m.notes {
			if note.ID == msg.id {
				m.notes = append(m.notes[:i], m.notes[i+1:]...)
				if m.selected >= len(m.notes) && m.selected > 0 {
					m.selected--
				}
				break
			}
		}
		m.message = "Note permanently deleted."
		return m, nil
	case movedNote:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.openLogin()
				m.message = "Session expired. Sign in again."
				return m, nil
			}
			switch {
			case errors.As(msg.err, &apiErr) && apiErr.Code == "conflict":
				m.message = "A project may have been archived. Press r to reload destinations, or Esc then r to refresh notes."
			case errors.As(msg.err, &apiErr) && apiErr.Code == "not_found":
				m.message = "Note or destination not found. Press r to reload destinations, or Esc then r to refresh notes."
			default:
				m.message = "Move not confirmed. Press Esc then r to refresh notes before retrying."
			}
			return m, nil
		}
		m.stage = inboxStage
		m.moveID = ""
		m.moveTargets = nil
		for i, note := range m.notes {
			if note.ID == msg.note.ID {
				m.notes = append(m.notes[:i], m.notes[i+1:]...)
				if m.selected >= len(m.notes) && m.selected > 0 {
					m.selected--
				}
				break
			}
		}
		if m.activeProject != nil && msg.destination.id == "" {
			// A moved note belongs in the Inbox; its earlier snapshot is stale.
			m.inboxNeedsRefresh = true
		}
		m.message = "Note moved to " + safeText(msg.destination.name) + "."
		return m, nil
	case tea.KeyPressMsg:
		m.status.dismiss()
		if msg.String() == "ctrl+c" && m.stage == workspaceStage && m.workspace.unsaved() {
			if m.workspace.popup == nil || m.workspace.popup.action.kind != quitAction {
				m.workspace.confirmQuit()
				return m, nil
			}
			return m, tea.Quit // a second Ctrl+C quits without asking again
		}
		if msg.String() == "ctrl+c" {
			if m.stage == captureStage && !m.busy && m.draft.Value() != m.editorStart {
				m.stage = discardStage
				m.quitAfterDiscard = true
				m.message = ""
				return m, nil
			}
			return m, tea.Quit
		}
		if m.stage == loginStage {
			return m.updateLogin(msg)
		}
		if m.stage == workspaceStage {
			return m.updateWorkspace(msg)
		}
		if m.busy {
			return m, nil
		}
		if msg.String() == "esc" {
			switch m.stage {
			case captureStage:
				if m.draft.Value() != m.editorStart {
					m.stage = discardStage
					m.quitAfterDiscard = false
					m.message = ""
					return m, nil
				}
				m.closeEditor("No changes saved.")
				return m, nil
			case discardStage:
				m.stage = captureStage
				m.quitAfterDiscard = false
				return m, nil
			case deleteStage:
				m.stage = m.deleteBack
				m.deleteID = ""
				return m, nil
			case moveStage:
				m.stage = m.moveBack
				m.moveID = ""
				m.moveTargets = nil
				m.message = ""
				return m, nil
			case startupStage:
				m.openLogin()
				m.token = ""
			case readingStage:
				m.stage = inboxStage
				return m, nil
			case inboxStage:
				// Leaving a project's notes returns to the project list; the
				// Inbox snapshot stays stashed for openInbox.
				if m.activeProject != nil {
					m.activeProject = nil
					m.notes = nil
					m.nextCursor = ""
					m.selected = 0
					m.stage = projectsStage
					m.message = ""
					return m, nil
				}
			case projectsStage:
				return m, m.openInbox()
			case newProjectStage:
				m.inputs[projectNameInput].Blur()
				m.inputs[projectNameInput].SetValue("")
				m.stage = projectsStage
				return m, nil
			case archiveStage:
				m.archiveID = ""
				m.stage = projectsStage
				m.message = ""
				return m, nil
			case sessionsStage:
				m.stage = m.sessionBack
				m.message = ""
				return m, nil
			case sessionConfirmStage:
				m.stage = sessionsStage
				m.message = ""
				return m, nil
			case accountStage:
				m.stage = m.accountBack
				m.message = ""
				return m, nil
			case accountEditStage:
				if m.accountChanged() {
					m.stage = accountDiscardStage
					return m, nil
				}
				m.inputs[m.focus].Blur()
				m.stage = accountStage
				m.message = ""
				return m, nil
			case accountDiscardStage:
				m.stage = accountEditStage
				return m, nil
			case accountDeleteStage:
				m.inputs[deletePhraseInput].Blur()
				m.stage = accountStage
				m.message = ""
				return m, nil
			}
			m.message = ""
			return m, nil
		}
		switch m.stage {
		case startupStage:
			if msg.String() == "r" && m.token != "" {
				m.busy = true
				return m, m.loadInbox(m.token, false)
			}
		case inboxStage:
			if m.archivedProject() {
				switch msg.String() {
				case "n", "e", "d", "v":
					m.message = "Archived projects are read-only. Unarchive this project to change its notes."
					return m, nil
				}
			}
			switch msg.String() {
			case "s":
				return m, m.openSessions(inboxStage)
			case "g":
				return m, m.openAccount(inboxStage)
			case "p":
				if m.activeProject != nil {
					m.activeProject = nil
					m.notes = nil
					m.nextCursor = ""
					m.selected = 0
				}
				m.stage = projectsStage
				m.busy = true
				m.message = "Loading projects…"
				return m, m.fetchProjects()
			case "m":
				if m.nextCursor != "" {
					m.busy = true
					m.message = "Loading older notes…"
					cursor := m.nextCursor
					return m, func() tea.Msg {
						page, err := m.fetchOlder(context.Background(), m.token, cursor)
						return olderNotes{page: page, cursor: cursor, err: err}
					}
				}
				m.message = "No more notes to load."
				return m, nil
			case "n":
				return m, m.openEditor("", "", inboxStage)
			case "e":
				if len(m.notes) > 0 {
					note := m.notes[m.selected]
					return m, m.openEditor(note.ID, note.Content, inboxStage)
				}
			case "d":
				if len(m.notes) > 0 {
					m.deleteID = m.notes[m.selected].ID
					m.deleteBack = inboxStage
					m.stage = deleteStage
					m.message = ""
				}
			case "v":
				if len(m.notes) > 0 {
					return m, m.startMove(inboxStage)
				}
			case "q":
				return m, tea.Quit
			case "r":
				m.busy = true
				return m, m.loadInbox(m.token, false)
			case "j", "down":
				if m.selected+1 < len(m.notes) {
					m.selected++
				}
			case "k", "up":
				if m.selected > 0 {
					m.selected--
				}
			case "enter":
				if len(m.notes) > 0 {
					m.stage = readingStage
					m.resizeReader()
					m.reader.SetContent(lipgloss.NewStyle().Width(m.reader.Width()).Render(safeText(m.notes[m.selected].Content)))
					m.reader.GotoTop()
				}
			}
			return m, nil
		case readingStage:
			if m.archivedProject() {
				switch msg.String() {
				case "e", "d", "v":
					m.message = "Archived projects are read-only. Unarchive this project to change its notes."
					return m, nil
				}
			}
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "e":
				note := m.notes[m.selected]
				return m, m.openEditor(note.ID, note.Content, readingStage)
			case "d":
				m.deleteID = m.notes[m.selected].ID
				m.deleteBack = readingStage
				m.stage = deleteStage
				m.message = ""
				return m, nil
			case "v":
				return m, m.startMove(readingStage)
			}
			var cmd tea.Cmd
			m.reader, cmd = m.reader.Update(msg)
			return m, cmd
		case captureStage:
			if msg.String() == "ctrl+s" {
				content := m.draft.Value()
				if m.editorID != "" && content == m.editorStart {
					m.closeEditor("No changes to save.")
					return m, nil
				}
				if strings.TrimSpace(content) == "" {
					m.message = "Write something before saving."
					return m, nil
				}
				if utf8.RuneCountInString(content) > 20000 {
					m.message = "Notes must be at most 20,000 characters."
					return m, nil
				}
				m.busy = true
				m.message = "Saving note…"
				if m.editorID != "" {
					id := m.editorID
					return m, func() tea.Msg {
						note, err := m.api.UpdateNote(context.Background(), m.token, id, content)
						return updatedNote{note, err}
					}
				}
				if m.activeProject != nil {
					project := *m.activeProject
					return m, func() tea.Msg {
						note, err := m.api.CreateProjectNote(context.Background(), m.token, project.ID, content)
						return createdNote{note, err}
					}
				}
				return m, func() tea.Msg {
					note, err := m.api.CreateInboxNote(context.Background(), m.token, content)
					return createdNote{note, err}
				}
			}
			var cmd tea.Cmd
			m.draft, cmd = m.draft.Update(msg)
			if m.message != "" {
				m.message = ""
			}
			return m, cmd
		case discardStage:
			switch msg.String() {
			case "y":
				if m.quitAfterDiscard {
					return m, tea.Quit
				}
				m.closeEditor("Changes discarded.")
			case "n":
				m.stage = captureStage
				m.quitAfterDiscard = false
				return m, m.draft.Focus()
			}
			return m, nil
		case deleteStage:
			switch msg.String() {
			case "n":
				m.stage = m.deleteBack
				m.deleteID = ""
				return m, nil
			case "y":
				m.busy = true
				id := m.deleteID
				return m, func() tea.Msg {
					return deletedNote{id: id, err: m.api.DeleteNote(context.Background(), m.token, id)}
				}
			}
			return m, nil
		case projectsStage:
			switch msg.String() {
			case "s":
				return m, m.openSessions(projectsStage)
			case "g":
				return m, m.openAccount(projectsStage)
			case "j", "down":
				if m.projectSelected+1 < len(m.projects) {
					m.projectSelected++
				}
			case "k", "up":
				if m.projectSelected > 0 {
					m.projectSelected--
				}
			case "a":
				if m.showArchived {
					m.message = "Switch to Active projects before creating a project."
					return m, nil
				}
				m.stage = newProjectStage
				m.inputs[projectNameInput].SetValue("")
				m.message = ""
				m.focusInput(projectNameInput)
				return m, textinput.Blink
			case "i":
				return m, m.openInbox()
			case "r":
				m.busy = true
				m.message = "Loading projects…"
				return m, m.fetchProjects()
			case "t":
				m.showArchived = !m.showArchived
				m.projects = nil
				m.projectSelected = 0
				m.busy = true
				m.message = "Loading projects…"
				return m, m.fetchProjects()
			case "x":
				if !m.showArchived && len(m.projects) > 0 {
					m.archiveID = m.projects[m.projectSelected].ID
					m.stage = archiveStage
					m.message = ""
				}
				return m, nil
			case "u":
				if m.showArchived && len(m.projects) > 0 {
					id := m.projects[m.projectSelected].ID
					m.busy = true
					m.message = "Unarchiving project…"
					return m, func() tea.Msg {
						return projectArchiveResult{id: id, err: m.api.UnarchiveProject(context.Background(), m.token, id)}
					}
				}
				return m, nil
			case "enter":
				if len(m.projects) > 0 {
					return m, m.openProject(m.projects[m.projectSelected])
				}
			case "q":
				return m, tea.Quit
			}
			return m, nil
		case newProjectStage:
			if msg.String() == "enter" {
				name := strings.TrimSpace(m.inputs[projectNameInput].Value())
				if name == "" {
					m.message = "Enter a project name."
					return m, nil
				}
				if utf8.RuneCountInString(name) > 120 {
					m.message = "Project names must be at most 120 characters."
					return m, nil
				}
				m.busy = true
				m.message = "Creating project…"
				return m, func() tea.Msg {
					project, err := m.api.CreateProject(context.Background(), m.token, name)
					return projectCreated{project, err}
				}
			}
		case moveStage:
			switch msg.String() {
			case "j", "down":
				if m.moveSelected+1 < len(m.moveTargets) {
					m.moveSelected++
				}
			case "k", "up":
				if m.moveSelected > 0 {
					m.moveSelected--
				}
			case "r":
				m.busy = true
				m.moveTargets = nil
				m.message = "Loading destinations…"
				return m, m.fetchProjects()
			case "enter":
				if len(m.moveTargets) == 0 {
					m.message = "No destinations available. Create an active project first."
					return m, nil
				}
				target := m.moveTargets[m.moveSelected]
				m.busy = true
				m.message = "Moving note…"
				id := m.moveID
				return m, func() tea.Msg {
					note, err := m.api.MoveNote(context.Background(), m.token, id, target.id)
					return movedNote{note: note, destination: target, err: err}
				}
			}
			return m, nil
		case archiveStage:
			switch msg.String() {
			case "n":
				m.stage = projectsStage
				m.archiveID = ""
				return m, nil
			case "y":
				id := m.archiveID
				m.busy = true
				m.message = "Archiving project…"
				return m, func() tea.Msg {
					return projectArchiveResult{id: id, archived: true, err: m.api.ArchiveProject(context.Background(), m.token, id)}
				}
			}
			return m, nil
		case sessionsStage:
			switch msg.String() {
			case "j", "down":
				if m.sessionSelected+1 < len(m.sessions) {
					m.sessionSelected++
				}
			case "k", "up":
				if m.sessionSelected > 0 {
					m.sessionSelected--
				}
			case "r":
				m.busy = true
				m.message = "Loading sessions…"
				return m, m.fetchSessions()
			case "x":
				if len(m.sessions) > 0 {
					m.sessionID = m.sessions[m.sessionSelected].ID
					m.sessionAction = revokeOne
					m.stage = sessionConfirmStage
					m.message = ""
				}
			case "a":
				m.sessionAction = revokeAll
				m.stage = sessionConfirmStage
				m.message = ""
			case "l":
				m.sessionAction = logout
				m.stage = sessionConfirmStage
				m.message = ""
			}
			return m, nil
		case sessionConfirmStage:
			switch msg.String() {
			case "n":
				m.stage = sessionsStage
				return m, nil
			case "y":
				action, id, token := m.sessionAction, m.sessionID, m.token
				m.busy = true
				m.message = "Updating sessions…"
				return m, func() tea.Msg {
					var err error
					switch action {
					case revokeOne:
						err = m.api.RevokeSession(context.Background(), token, id)
					case revokeAll:
						err = m.api.RevokeAllSessions(context.Background(), token)
					case logout:
						err = m.api.Logout(context.Background(), token)
					}
					return sessionActionResult{action, id, err}
				}
			}
			return m, nil
		case accountStage:
			switch msg.String() {
			case "r":
				m.busy = true
				m.message = "Loading account…"
				return m, m.fetchAccount()
			case "e":
				if m.account.ID != "" {
					return m, m.editAccount()
				}
			case "d":
				if m.account.ID != "" {
					m.stage = accountDeleteStage
					m.inputs[deletePhraseInput].SetValue("")
					m.focusInput(deletePhraseInput)
					m.message = ""
					return m, textinput.Blink
				}
			}
			return m, nil
		case accountEditStage:
			switch msg.String() {
			case "tab", "shift+tab":
				if m.focus == accountNameInput {
					m.focusInput(accountZoneInput)
				} else {
					m.focusInput(accountNameInput)
				}
				return m, nil
			case "enter":
				name := strings.TrimSpace(m.inputs[accountNameInput].Value())
				zone := strings.TrimSpace(m.inputs[accountZoneInput].Value())
				if name == "" || utf8.RuneCountInString(name) > 80 || strings.ContainsFunc(name, unicode.IsControl) {
					m.message = "Name must be 1–80 characters."
					return m, nil
				}
				if zone == "" || len(zone) > 64 {
					m.message = "Enter an IANA time zone (at most 64 characters)."
					return m, nil
				}
				if zone == "Local" {
					m.message = "Choose an IANA time zone such as Europe/London, not Local."
					return m, nil
				}
				if _, err := time.LoadLocation(zone); err != nil {
					m.message = "Enter a valid IANA time zone (for example, Europe/London)."
					return m, nil
				}
				if name == m.accountStartName && zone == m.accountStartZone {
					m.stage = accountStage
					m.inputs[m.focus].Blur()
					m.message = "No changes to save."
					return m, nil
				}
				m.busy = true
				m.message = "Updating profile…"
				token := m.token
				return m, func() tea.Msg {
					account, err := m.api.UpdateProfile(context.Background(), token, name, zone)
					return profileResult{account, err}
				}
			}
			var cmd tea.Cmd
			m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
			m.message = ""
			return m, cmd
		case accountDiscardStage:
			switch msg.String() {
			case "y":
				m.inputs[m.focus].Blur()
				m.stage = accountStage
				m.message = "Changes discarded."
			case "n":
				m.stage = accountEditStage
			}
			return m, nil
		case accountDeleteStage:
			if msg.String() == "enter" {
				if m.inputs[deletePhraseInput].Value() != "DELETE" {
					m.message = "Type DELETE exactly to confirm, or Esc to cancel."
					return m, nil
				}
				m.busy = true
				m.message = "Requesting account deletion…"
				token := m.token
				return m, func() tea.Msg {
					deadline, err := m.api.RequestAccountDeletion(context.Background(), token)
					return deletionResult{deadline, err}
				}
			}
			var cmd tea.Cmd
			m.inputs[deletePhraseInput], cmd = m.inputs[deletePhraseInput].Update(msg)
			m.message = ""
			return m, cmd
		}
	}
	if m.stage == newProjectStage && !m.busy {
		var cmd tea.Cmd
		m.inputs[m.focus], cmd = m.inputs[m.focus].Update(message)
		return m, cmd
	}
	if m.stage == loginStage {
		return m.updateLogin(message)
	}
	if m.stage == workspaceStage {
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
	m.inputs[m.focus].Blur()
	host := strings.TrimPrefix(strings.TrimPrefix(m.origin, "https://"), "http://")
	m.workspace = newWorkspace(m.api, token, host)
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
	m.inputs[m.focus].Blur()
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
	name := m.username
	if m.stage == workspaceStage {
		name = m.workspace.userName()
	}
	if m.token != "" && name != "" {
		if host == "" {
			return preview(name, 24)
		}
		return preview(name, 24) + " · " + host
	}
	return host
}

func (m *Model) focusInput(index int) {
	m.inputs[m.focus].Blur()
	m.focus = index
	m.inputs[index].Focus()
}

func (m *Model) openEditor(id, content string, back stage) tea.Cmd {
	m.editorID = id
	m.editorStart = content
	m.editorBack = back
	m.draft.SetValue(content)
	m.stage = captureStage
	m.message = ""
	return m.draft.Focus()
}

func (m *Model) startMove(back stage) tea.Cmd {
	m.moveID = m.notes[m.selected].ID
	m.moveBack = back
	m.moveTargets = nil
	m.moveSelected = 0
	m.stage = moveStage
	m.busy = true
	m.message = "Loading destinations…"
	return m.fetchProjects()
}

// openProject shows a project's notes. The Inbox list is snapshotted so it
// can be restored without refetching when the user returns.
func (m *Model) openProject(p api.Project) tea.Cmd {
	if !m.inboxStashed && !m.inboxNeedsRefresh {
		m.inboxList = noteList{notes: m.notes, cursor: m.nextCursor, selected: m.selected}
		m.inboxStashed = true
	}
	project := p
	m.activeProject = &project
	m.notes = nil
	m.nextCursor = ""
	m.selected = 0
	m.stage = inboxStage
	m.busy = true
	m.message = "Loading project notes…"
	return m.loadInbox(m.token, false)
}

// openInbox returns to the Inbox, restoring the snapshotted list when present.
func (m *Model) openInbox() tea.Cmd {
	m.activeProject = nil
	m.stage = inboxStage
	if m.inboxStashed && !m.inboxNeedsRefresh {
		m.notes = m.inboxList.notes
		m.nextCursor = m.inboxList.cursor
		m.selected = m.inboxList.selected
		m.inboxList = noteList{}
		m.inboxStashed = false
		m.message = ""
		return nil
	}
	m.inboxList = noteList{}
	m.inboxStashed = false
	m.inboxNeedsRefresh = false
	m.notes = nil
	m.nextCursor = ""
	m.selected = 0
	m.busy = true
	m.message = "Loading Inbox…"
	return m.loadInbox(m.token, false)
}

func (m Model) listTitle() string {
	if m.activeProject != nil {
		return safeText(m.activeProject.Name)
	}
	return "Inbox"
}

func (m Model) archivedProject() bool {
	return m.activeProject != nil && m.activeProject.ArchivedAt != nil
}

func (m Model) currentSessionID() string {
	for _, session := range m.sessions {
		if session.Current {
			return session.ID
		}
	}
	return ""
}

func (m *Model) closeEditor(message string) {
	m.stage = m.editorBack
	m.editorID = ""
	m.editorStart = ""
	m.draft.Blur()
	m.draft.SetValue("")
	m.message = message
}

func (m *Model) resizeReader() {
	width := m.innerWidth()
	m.reader.SetWidth(width)
	m.reader.SetHeight(max(3, m.height-10))
	if m.stage == readingStage && len(m.notes) > m.selected {
		m.reader.SetContent(lipgloss.NewStyle().Width(width).Render(safeText(m.notes[m.selected].Content)))
	}
}

func (m Model) contentWidth() int {
	return max(20, min(68, m.width-4))
}

// View adds two columns of padding on either side. Lip Gloss subtracts that
// padding from Width before wrapping, so children must use the inner width.
func (m Model) innerWidth() int {
	return m.contentWidth() - 4
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

// render draws the current screen.
func (m Model) render() string {
	if m.stage == loginStage {
		return m.login.View(m.message, m.status.spinner()) + "\n" + m.footer()
	}
	if m.stage == workspaceStage {
		return lipgloss.NewStyle().Height(m.bodyHeight()).MaxHeight(m.bodyHeight()).Render(m.workspace.View(m.status.spinner())) + "\n" + m.footer()
	}
	var body string
	switch m.stage {
	case startupStage:
		body = "Checking saved session…"
	case inboxStage:
		body = m.listTitle()
		if m.archivedProject() {
			body += " · archived (read-only)"
		}
		if m.activeProject == nil && m.username != "" {
			body += " · " + safeText(m.username)
		}
		body += "\n\n"
		if len(m.notes) == 0 && !m.busy && m.message == "" {
			if m.activeProject != nil {
				body += "No notes in this project yet.\n"
			} else {
				body += "No Inbox notes yet.\n"
			}
		}
		rows := max(3, m.height-11)
		start := max(0, m.selected-rows+1)
		for i := start; i < len(m.notes) && i < start+rows; i++ {
			mark := "  "
			if i == m.selected {
				mark = "› "
			}
			body += fmt.Sprintf("%s%s  %s\n", mark, m.notes[i].CreatedAt.Local().Format("Jan 02"), preview(m.notes[i].Content, max(12, m.innerWidth()-16)))
		}
		if m.nextCursor != "" {
			body += fmt.Sprintf("\n%s", dimStyle.Render(fmt.Sprintf("%d loaded · m load older", len(m.notes))))
		} else if len(m.notes) > 0 {
			body += fmt.Sprintf("\n%s", dimStyle.Render(fmt.Sprintf("%d loaded · end of %s", len(m.notes), m.listTitle())))
		}
	case readingStage:
		body = m.listTitle() + " · " + m.notes[m.selected].CreatedAt.Local().Format("Jan 02, 2006 15:04") + "\n\n" + m.reader.View()
		if m.archivedProject() {
			body += "\n\n" + dimStyle.Render("Archived · read-only")
		}
	case captureStage:
		heading := "New note"
		if m.editorID != "" {
			heading = "Edit note"
		}
		status := "unchanged"
		if m.editorID == "" {
			status = "empty"
		}
		if m.draft.Value() != m.editorStart {
			status = "unsaved"
		}
		body = fmt.Sprintf("%s\n\n%s\n\n%s", heading, m.draft.View(), dimStyle.Render(fmt.Sprintf("%d / 20,000 characters · %s", utf8.RuneCountInString(m.draft.Value()), status)))
	case discardStage:
		prompt := "Discard unsaved changes?"
		if m.quitAfterDiscard {
			prompt = "Discard unsaved changes and quit?"
		}
		body = prompt
	case deleteStage:
		title := "this note"
		for _, note := range m.notes {
			if note.ID == m.deleteID {
				title = preview(note.Content, 44)
				break
			}
		}
		body = "Permanently delete “" + title + "”?\nThere is no Trash and this cannot be undone."
	case projectsStage:
		if m.showArchived {
			body = "Archived projects\n\n"
		} else {
			body = "Active projects\n\n"
		}
		if len(m.projects) == 0 && !m.busy && m.message == "" {
			body += "No projects in this view.\n"
		}
		rows := max(3, m.height-11)
		start := max(0, m.projectSelected-rows+1)
		for i := start; i < len(m.projects) && i < start+rows; i++ {
			mark := "  "
			if i == m.projectSelected {
				mark = "› "
			}
			body += fmt.Sprintf("%s%s\n", mark, preview(m.projects[i].Name, max(12, m.innerWidth()-6)))
		}
	case accountStage:
		body = "Account settings\n\n"
		if m.account.ID != "" {
			body += "Name: " + preview(m.account.Username, m.innerWidth()-7) + "\n"
			body += "Time zone: " + preview(m.account.TimeZone, m.innerWidth()-12) + "\n"
			if !m.account.CreatedAt.IsZero() {
				body += "Created: " + m.account.CreatedAt.Local().Format("Jan 02, 2006") + "\n"
			}
		}
	case accountEditStage:
		body = "Edit profile\n\nName\n" + m.inputs[accountNameInput].View() + "\n\nIANA time zone\n" + m.inputs[accountZoneInput].View()
	case accountDiscardStage:
		body = "Discard unsaved profile changes?"
	case accountDeleteStage:
		body = "Schedule account deletion?\n\nAll devices will be signed out immediately. Your projects and notes will become inaccessible. You can restore your account by signing in with the same identity within 30 days. After that, your data is permanently erased by a scheduled job.\n\nType DELETE to confirm:\n" + m.inputs[deletePhraseInput].View()
	case sessionsStage:
		body = "Sessions\n\n"
		if len(m.sessions) == 0 && !m.busy && m.message == "" {
			body += "No active sessions.\n"
		}
		rows := max(3, m.height-12)
		start := max(0, m.sessionSelected-rows+1)
		for i := start; i < len(m.sessions) && i < start+rows; i++ {
			s := m.sessions[i]
			mark := "  "
			if i == m.sessionSelected {
				mark = "› "
			}
			label := safeText(s.DeviceLabel)
			if label == "" {
				label = safeText(s.UserAgent)
			}
			if label == "" {
				label = "Unknown device"
			}
			if s.Current {
				label += " (this device)"
			}
			body += mark + preview(label, max(12, m.innerWidth()-5)) + "\n"
			body += "    Last used " + s.LastUsedAt.Local().Format("Jan 02, 2006 15:04") + "\n"
		}
	case sessionConfirmStage:
		switch m.sessionAction {
		case revokeOne:
			body = "Revoke this session?"
			if m.sessionID == m.currentSessionID() {
				body += " This will sign you out here."
			}
		case revokeAll:
			body = "Revoke all sessions? Every device, including this one, will be signed out."
		case logout:
			body = "Log out of this device? The server session will be revoked."
		}
	case newProjectStage:
		body = "New project\n\nName\n" + m.inputs[projectNameInput].View()
	case moveStage:
		body = "Move note from " + m.listTitle() + "\n\n"
		for _, note := range m.notes {
			if note.ID == m.moveID {
				body += preview(note.Content, max(12, m.innerWidth()-5)) + "\n\n"
				break
			}
		}
		if len(m.moveTargets) == 0 && !m.busy && m.message == "" {
			body += "No other active locations. Create a project first.\n"
		}
		rows := max(3, m.height-13)
		start := max(0, m.moveSelected-rows+1)
		for i := start; i < len(m.moveTargets) && i < start+rows; i++ {
			mark := "  "
			if i == m.moveSelected {
				mark = "› "
			}
			body += mark + preview(m.moveTargets[i].name, max(12, m.innerWidth()-5)) + "\n"
		}
	case archiveStage:
		name := "this project"
		for _, project := range m.projects {
			if project.ID == m.archiveID {
				name = preview(project.Name, max(12, m.innerWidth()-5))
				break
			}
		}
		body = "Archive “" + name + "”?\nIts notes will stay available, but you cannot edit, move, create, or delete notes in an archived project until it is unarchived."
	}
	// Screens not yet moved to the status line keep their message in the
	// body; many are notices too long or important for a single footer line.
	if m.message != "" {
		body += "\n\n" + errStyle.Render(m.message)
	}
	content := titleStyle.Render("ShortLog") + "\n\n" + body
	height := m.bodyHeight()
	page := lipgloss.NewStyle().Width(m.contentWidth()).Padding(1, 2).Render(content)
	return lipgloss.NewStyle().Height(height).MaxHeight(height).Render(page) + "\n" + m.footer()
}
