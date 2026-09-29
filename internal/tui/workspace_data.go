package tui

import (
	"context"
	"errors"
	"net/http"

	tea "charm.land/bubbletea/v2"
	"shortlog-cli/internal/api"
)

// workspaceAPI is the part of the Shortlog API the workspace reads.
type workspaceAPI interface {
	Me(context.Context, string) (api.Account, error)
	Sessions(context.Context, string) ([]api.Session, error)
	Projects(context.Context, string) ([]api.Project, error)
	ArchivedProjects(context.Context, string) ([]api.Project, error)
	Inbox(context.Context, string) (api.NotesPage, error)
	InboxPage(context.Context, string, string) (api.NotesPage, error)
	ProjectNotes(context.Context, string, string) (api.NotesPage, error)
	ProjectNotesPage(context.Context, string, string, string) (api.NotesPage, error)
}

// inboxSource names the Inbox among note sources; any other source is a
// project ID.
const inboxSource = ""

// loadState is where a panel's data stands: still loading, failed, or ready.
type loadState struct {
	loading bool
	loaded  bool   // the data arrived at least once
	err     string // the last failure, shown in the panel until a retry
}

// noteSet is the notes loaded so far for one source, newest first, with the
// cursor of the next older page.
type noteSet struct {
	loadState
	notes []api.Note
	next  string // empty once every page is loaded
}

// Results of the workspace's requests. Each carries the session token it was
// made with, so a result that arrives after signing out or back in is ignored.
type accountLoaded struct {
	token       string
	account     api.Account
	sessions    []api.Session
	err         error
	sessionsErr error
}

type projectsLoaded struct {
	token            string
	active, archived []api.Project
	err              error
}

type notesLoaded struct {
	token, source string
	cursor        string // the page asked for; empty for the newest page
	page          api.NotesPage
	err           error
}

// loadMoreMargin is how close to the end of a note list the cursor gets
// before the next older page is requested.
const loadMoreMargin = 5

func unauthorized(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized
}

func (w *workspaceModel) loadAccount() tea.Cmd {
	w.accountState.loading, w.accountState.err = true, ""
	client, token := w.api, w.token
	return func() tea.Msg {
		account, err := client.Me(context.Background(), token)
		if err != nil {
			return accountLoaded{token: token, err: err}
		}
		sessions, sessionsErr := client.Sessions(context.Background(), token)
		return accountLoaded{token: token, account: account, sessions: sessions, sessionsErr: sessionsErr}
	}
}

func (w *workspaceModel) loadProjects() tea.Cmd {
	w.projectsState.loading, w.projectsState.err = true, ""
	client, token := w.api, w.token
	return func() tea.Msg {
		active, err := client.Projects(context.Background(), token)
		if err != nil {
			return projectsLoaded{token: token, err: err}
		}
		archived, err := client.ArchivedProjects(context.Background(), token)
		return projectsLoaded{token: token, active: active, archived: archived, err: err}
	}
}

// loadNotes asks for a page of a source's notes: the newest page when cursor
// is empty (a first load or a refresh), otherwise the next older page.
func (w *workspaceModel) loadNotes(source, cursor string) tea.Cmd {
	set := w.sets[source]
	set.loading, set.err = true, ""
	w.sets[source] = set
	client, token := w.api, w.token
	return func() tea.Msg {
		var page api.NotesPage
		var err error
		switch {
		case source == inboxSource && cursor == "":
			page, err = client.Inbox(context.Background(), token)
		case source == inboxSource:
			page, err = client.InboxPage(context.Background(), token, cursor)
		case cursor == "":
			page, err = client.ProjectNotes(context.Background(), token, source)
		default:
			page, err = client.ProjectNotesPage(context.Background(), token, source, cursor)
		}
		return notesLoaded{token: token, source: source, cursor: cursor, page: page, err: err}
	}
}

// failed records a request failure: a 401 ends the session; anything else is
// shown where it happened and on the status line.
func (w *workspaceModel) failed(state *loadState, err error, what string) {
	state.loading = false
	if unauthorized(err) {
		w.expired = true
		return
	}
	state.err = friendlyError(err, "Could not load "+what+".")
	w.note = &statusNote{level: statusError, text: state.err}
}

func (w workspaceModel) onAccountLoaded(msg accountLoaded) workspaceModel {
	if msg.err != nil {
		w.failed(&w.accountState, msg.err, "your account")
		return w
	}
	w.accountState = loadState{loaded: true}
	w.account.account, w.account.sessions = msg.account, msg.sessions
	w.account.sessionsErr = ""
	if msg.sessionsErr != nil {
		if unauthorized(msg.sessionsErr) {
			w.expired = true
			return w
		}
		w.account.sessionsErr = friendlyError(msg.sessionsErr, "Could not load sessions.")
	}
	return w
}

func (w workspaceModel) onProjectsLoaded(msg projectsLoaded) (workspaceModel, tea.Cmd) {
	if msg.err != nil {
		w.failed(&w.projectsState, msg.err, "projects")
		return w, nil
	}
	w.projectsState = loadState{loaded: true}
	w.projects.setProjects(msg.active, msg.archived)
	w.setSize(w.width, w.height) // the projects panel height follows its length
	if w.notes.current == nil {
		// The project tab is ready from the start, but the Inbox shows first.
		cmd := w.showSelectedProject()
		w.notes.tab = inboxTab
		w.syncReader()
		return w, cmd
	}
	return w, nil
}

func (w workspaceModel) onNotesLoaded(msg notesLoaded) workspaceModel {
	set := w.sets[msg.source]
	if msg.err != nil {
		w.failed(&set.loadState, msg.err, "notes")
		w.sets[msg.source] = set
		w.syncNotes()
		return w
	}
	next := cursorValue(msg.page.NextCursor)
	if msg.cursor == "" {
		set.notes = msg.page.Items
	} else {
		set.notes = append(append([]api.Note(nil), set.notes...), msg.page.Items...)
	}
	if next != "" && next == msg.cursor {
		// A server that hands back the cursor it was given would page forever.
		next = ""
		w.note = &statusNote{level: statusWarn, text: "Server repeated a page cursor; stopped loading older notes."}
	}
	set.loadState, set.next = loadState{loaded: true}, next
	w.sets[msg.source] = set
	w.syncNotes()
	return w
}

// loadMoreIfNear requests the next older page once the cursor nears the end
// of the visible note list.
func (w *workspaceModel) loadMoreIfNear() tea.Cmd {
	source := w.visibleSource()
	set := w.sets[source]
	if set.next == "" || set.loading || set.err != "" || w.notes.lists[w.notes.tab].cursor < len(set.notes)-loadMoreMargin {
		return nil
	}
	return w.loadNotes(source, set.next)
}

// visibleSource is the source of the notes tab on show.
func (w workspaceModel) visibleSource() string {
	if w.notes.tab == projectTab && w.notes.current != nil {
		return w.notes.current.ID
	}
	return inboxSource
}

// syncNotes hands the loaded notes and their load states to the notes panel.
func (w *workspaceModel) syncNotes() {
	inbox := w.sets[inboxSource]
	w.notes.inbox, w.notes.states[inboxTab] = inbox.notes, inbox.loadState
	w.notes.more[inboxTab] = inbox.next != ""
	if w.notes.current != nil {
		project := w.sets[w.notes.current.ID]
		w.notes.project, w.notes.states[projectTab] = project.notes, project.loadState
		w.notes.more[projectTab] = project.next != ""
	}
	w.notes.setRows(w.notes.rows)
	w.syncReader()
}
