package tui

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"shortlog-cli/internal/api"
)

// focusArea is the part of the workspace that receives keys. The values match
// the panel numbers, so 0–3 focus them directly.
type focusArea int

const (
	focusMain focusArea = iota
	focusAccount
	focusProjects
	focusNotes
)

// narrowWidth is the width below which the workspace shows one column at a
// time: the panels, or the main panel once it has focus.
const narrowWidth = 70

// notePlace is a note together with where it lives, for the reader header.
type notePlace struct {
	api.Note
	place string
}

// workspaceModel is the signed-in screen, laid out like lazygit: stacked
// panels on the left ([1] account, [2] projects, [3] notes) and the main
// panel [0] on the right showing the selected note or the account page.
//
// It loads its own data through commands and reports to the root model the
// way the sign-in screen does, through state the root reads after each
// update: a pending status note, and whether the session has expired.
type workspaceModel struct {
	api           workspaceClient
	token         string // never rendered
	keys          workspaceKeyMap
	width, height int // the space above the footer
	focus         focusArea
	back          focusArea // where Esc returns from the main panel

	account  accountPanel
	projects projectsPanel
	notes    notesPanel
	reader   noteView

	accountState  loadState
	projectsState loadState
	sets          map[string]noteSet // loaded notes by source; see inboxSource

	// popup, when set, is drawn over the workspace and takes every key.
	popup *popupState
	// editor, when set, replaces the reader in the main panel.
	editor *noteEditor
	// pendingEdit is a restored edit waiting for its note to load.
	pendingEdit *unsentDraft

	note    *statusNote
	expired bool         // a request was refused with 401; the root signs out
	unsent  *unsentDraft // text the expired session could not save
	signOut *signOut     // the session ended by request; the root signs out
	// wantServer asks the root to open the server popup.
	wantServer bool
}

func newWorkspace(client workspaceClient, token, server string) workspaceModel {
	w := workspaceModel{api: client, token: token, keys: defaultWorkspaceKeys(), focus: focusNotes, back: focusNotes, reader: newNoteView(), sets: map[string]noteSet{}}
	w.account.server = server
	w.setSize(80, 22)
	return w
}

// start requests everything the workspace opens with: the account, the
// projects, and the Inbox. A project's notes load when it is first selected.
func (w *workspaceModel) start() tea.Cmd {
	cmds := tea.Batch(w.loadAccount(), w.loadProjects(), w.loadNotes(inboxSource, ""))
	w.syncNotes()
	return cmds
}

// userName is the signed-in user's name once the account has loaded.
func (w workspaceModel) userName() string { return w.account.account.Username }

// loading reports whether any request is in flight.
func (w workspaceModel) loading() bool {
	if w.accountState.loading || w.projectsState.loading {
		return true
	}
	for _, set := range w.sets {
		if set.loading {
			return true
		}
	}
	return false
}

func (w *workspaceModel) setSize(width, height int) {
	w.width, w.height = width, height
	_, _, notesHeight := w.columnHeights()
	w.notes.setRows(notesHeight - 2)
	_, mainWidth := w.columnWidths()
	w.reader.setSize(mainWidth, height)
	if w.editor != nil {
		w.editor.setSize(mainWidth, height)
	}
	if w.popup != nil {
		w.popup.setSize(width, height)
	}
}

// columnWidths splits the width between the panel column and the main panel.
// Narrow terminals give each the full width, one shown at a time.
func (w workspaceModel) columnWidths() (int, int) {
	if w.width < narrowWidth {
		return w.width, w.width
	}
	left := min(max(w.width*2/5, 30), 52)
	return left, w.width - left
}

// columnHeights divides the panel column: the account line, the projects
// list (up to projectRows), and the notes list with the rest. On short
// terminals the projects list gives way first, so notes stay usable.
func (w workspaceModel) columnHeights() (account, projects, notes int) {
	account = 3
	projects = w.projects.rows() + 2
	notes = w.height - account - projects
	if notes < 5 {
		projects = max(3, w.height-account-5)
		notes = max(w.height-account-projects, 3)
	}
	return account, projects, notes
}

func (w workspaceModel) Update(msg tea.Msg) (workspaceModel, tea.Cmd) {
	switch msg := msg.(type) {
	case accountLoaded:
		if msg.token != w.token {
			return w, nil
		}
		return w.onAccountLoaded(msg), nil
	case projectsLoaded:
		if msg.token != w.token {
			return w, nil
		}
		return w.onProjectsLoaded(msg)
	case notesLoaded:
		if msg.token != w.token {
			return w, nil
		}
		w = w.onNotesLoaded(msg)
		w.reopenPendingEdit(msg.source)
		return w, w.loadMoreIfNear()
	case noteCreated:
		if msg.token != w.token {
			return w, nil
		}
		return w.onNoteCreated(msg), nil
	case noteUpdated:
		if msg.token != w.token {
			return w, nil
		}
		return w.onNoteUpdated(msg), nil
	case noteDeleted:
		if msg.token != w.token {
			return w, nil
		}
		return w.onNoteDeleted(msg), nil
	case noteMoved:
		if msg.token != w.token {
			return w, nil
		}
		return w.onNoteMoved(msg), nil
	case projectAdded:
		if msg.token != w.token {
			return w, nil
		}
		return w.onProjectAdded(msg)
	case projectArchived:
		if msg.token != w.token {
			return w, nil
		}
		return w.onProjectArchived(msg)
	case profileUpdated:
		if msg.token != w.token {
			return w, nil
		}
		return w.onProfileUpdated(msg), nil
	case sessionEnded:
		if msg.token != w.token {
			return w, nil
		}
		return w.onSessionEnded(msg), nil
	case accountDeletionRequested:
		if msg.token != w.token {
			return w, nil
		}
		return w.onAccountDeletionRequested(msg), nil
	}
	if w.popup != nil {
		return w.updatePopup(msg)
	}
	if w.editor != nil {
		return w.updateEditor(msg)
	}
	msg2, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if w.focus == focusMain {
			var cmd tea.Cmd
			w.reader, cmd = w.reader.Update(msg)
			return w, cmd
		}
		return w, nil
	}
	k := w.keys
	switch {
	case key.Matches(msg2, k.Quit):
		return w, tea.Quit
	case key.Matches(msg2, k.FocusMain):
		w.setFocus(focusMain)
	case key.Matches(msg2, k.FocusAccount):
		w.setFocus(focusAccount)
	case key.Matches(msg2, k.FocusProjects):
		w.setFocus(focusProjects)
	case key.Matches(msg2, k.FocusNotes):
		w.setFocus(focusNotes)
	case key.Matches(msg2, k.NextPanel):
		w.setFocus([]focusArea{focusAccount, focusProjects, focusNotes, focusMain}[w.focus])
	case key.Matches(msg2, k.PrevPanel):
		w.setFocus([]focusArea{focusNotes, focusMain, focusAccount, focusProjects}[w.focus])
	case key.Matches(msg2, k.Refresh):
		return w, w.refresh()
	case key.Matches(msg2, k.EditProfile) && w.onAccountPage():
		w.startEditProfile()
	case key.Matches(msg2, k.RevokeSession) && w.onAccountPage():
		w.startRevoke()
	case key.Matches(msg2, k.RevokeAll) && w.onAccountPage():
		w.startRevokeAll()
	case key.Matches(msg2, k.SignOut) && w.onAccountPage():
		w.startSignOut()
	case key.Matches(msg2, k.DeleteAccount) && w.onAccountPage():
		w.startDeleteAccount()
	case key.Matches(msg2, k.Server) && w.onAccountPage():
		w.wantServer = true
	case (key.Matches(msg2, k.Up) || key.Matches(msg2, k.Down)) && w.onAccountPage():
		delta := 1
		if key.Matches(msg2, k.Up) {
			delta = -1
		}
		w.account.list.move(delta, len(w.account.sessions), len(w.account.sessions))
	case key.Matches(msg2, k.New) && w.onNotes():
		return w, w.startNewNote()
	case key.Matches(msg2, k.Edit) && w.onNotes():
		w.startEdit()
	case key.Matches(msg2, k.Move) && w.onNotes():
		w.startMove()
	case key.Matches(msg2, k.Delete) && w.onNotes():
		w.startDelete()
	case key.Matches(msg2, k.NewProject) && w.focus == focusProjects:
		w.startNewProject()
	case key.Matches(msg2, k.Archive) && w.focus == focusProjects:
		return w, w.startArchiveToggle(true)
	case key.Matches(msg2, k.Unarchive) && w.focus == focusProjects:
		return w, w.startArchiveToggle(false)
	case key.Matches(msg2, k.Open) && w.focus != focusMain:
		// Enter drills in: a project to its notes, a note or the account
		// line to the main panel.
		if w.focus == focusProjects {
			cmd := w.showSelectedProject()
			w.setFocus(focusNotes)
			return w, cmd
		}
		w.setFocus(focusMain)
	case key.Matches(msg2, k.Back) && w.focus == focusMain:
		w.setFocus(w.back)
	default:
		return w.updateFocused(msg2)
	}
	return w, nil
}

// onNotes reports whether note actions apply: the notes panel, or the main
// panel while it shows a note.
func (w workspaceModel) onNotes() bool {
	return w.focus == focusNotes || (w.focus == focusMain && w.back != focusAccount)
}

// updatePopup passes a message to the open popup and acts on its outcome.
func (w workspaceModel) updatePopup(msg tea.Msg) (workspaceModel, tea.Cmd) {
	p, cmd, outcome := w.popup.Update(msg, w.keys)
	w.popup = &p
	switch outcome {
	case popupAccepted:
		return w, tea.Batch(cmd, w.acceptPopup())
	case popupCancelled:
		w.popup = nil
	}
	return w, cmd
}

// updateEditor passes a message to the note editor and acts on its outcome.
func (w workspaceModel) updateEditor(msg tea.Msg) (workspaceModel, tea.Cmd) {
	e, cmd, outcome := w.editor.Update(msg, w.keys)
	w.editor = &e
	switch outcome {
	case editorSave:
		return w, w.saveEdit()
	case editorCancel:
		if e.changed() {
			w.popup = newConfirm("Discard changes", "Discard your changes to “"+e.title+"”?", "discard", true, pendingAction{kind: discardEditAction})
			return w, nil
		}
		w.editor = nil
		w.setFocus(w.back)
	}
	return w, cmd
}

// updateFocused passes a key to the focused panel and keeps the panels to
// its right in step with the new selection.
func (w workspaceModel) updateFocused(msg tea.KeyPressMsg) (workspaceModel, tea.Cmd) {
	switch w.focus {
	case focusProjects:
		var changed bool
		before := w.projects.tab
		w.projects, changed = w.projects.Update(msg, w.keys)
		if changed || before != w.projects.tab {
			return w, w.showSelectedProject()
		}
	case focusNotes:
		w.notes, _ = w.notes.Update(msg, w.keys)
		w.syncReader()
		return w, w.loadMoreIfNear()
	case focusMain:
		var cmd tea.Cmd
		w.reader, cmd = w.reader.Update(msg)
		return w, cmd
	}
	return w, nil
}

// setFocus moves focus, remembering which panel the main panel was entered
// from so Esc can return to it.
func (w *workspaceModel) setFocus(f focusArea) {
	if f == focusMain && w.focus != focusMain {
		w.back = w.focus
	}
	w.focus = f
}

// showSelectedProject points the notes panel's project tab at the project
// highlighted in panel [2], loading its notes the first time.
func (w *workspaceModel) showSelectedProject() tea.Cmd {
	p, ok := w.projects.selected()
	if !ok {
		return nil
	}
	w.notes.showProject(p, w.sets[p.ID].notes)
	var cmd tea.Cmd
	if set := w.sets[p.ID]; !set.loaded && !set.loading {
		cmd = w.loadNotes(p.ID, "")
	}
	w.syncNotes()
	return cmd
}

// refresh reloads what the focused area shows.
func (w *workspaceModel) refresh() tea.Cmd {
	switch {
	case w.focus == focusAccount, w.focus == focusMain && w.back == focusAccount:
		return w.loadAccount()
	case w.focus == focusProjects:
		return w.loadProjects()
	}
	cmd := w.loadNotes(w.visibleSource(), "")
	w.syncNotes()
	return cmd
}

// syncReader shows the note selected in panel [3].
func (w *workspaceModel) syncReader() {
	note, ok := w.notes.selected()
	place := "Inbox"
	if w.notes.tab == projectTab && w.notes.current != nil {
		place = w.notes.current.Name
	}
	w.reader.show(notePlace{Note: note, place: place}, ok)
}

// View draws the workspace; spinner is the status bar's current frame, used
// by panels that are loading.
func (w workspaceModel) View(spinner string) string {
	leftWidth, mainWidth := w.columnWidths()
	accountHeight, projectsHeight, notesHeight := w.columnHeights()
	left := lipgloss.JoinVertical(lipgloss.Left,
		w.account.View(leftWidth, accountHeight, w.focus == focusAccount, w.accountState, spinner),
		w.projects.View(leftWidth, projectsHeight, w.focus == focusProjects, w.projectsState, spinner),
		w.notes.View(leftWidth, notesHeight, w.focus == focusNotes, spinner),
	)
	main := w.mainView(mainWidth, spinner)
	view := lipgloss.JoinHorizontal(lipgloss.Top, left, main)
	if w.width < narrowWidth {
		view = left
		if w.focus == focusMain {
			view = main
		}
	}
	if w.popup != nil {
		view = overlay(view, w.popup.View(w.width, w.height, spinner), w.width, w.height)
	}
	return view
}

// mainView is the account page while the account is in view, and otherwise
// the note selected in panel [3].
func (w workspaceModel) mainView(width int, spinner string) string {
	focused := w.focus == focusMain
	if w.editor != nil {
		return w.editor.View(width, w.height, spinner)
	}
	if w.focus == focusAccount || (focused && w.back == focusAccount) {
		return w.account.page(width, w.height, focused, w.accountState, spinner, time.Now())
	}
	return w.reader.View(width, w.height, focused)
}

// workspaceKeyMap is the workspace's bindings; panels match keys against it.
type workspaceKeyMap struct {
	Up, Down, PageUp, PageDown                         key.Binding
	PrevTab, NextTab, NextPanel, PrevPanel             key.Binding
	FocusMain, FocusAccount, FocusProjects, FocusNotes key.Binding
	Open, Back, Refresh, Quit                          key.Binding
	New, Edit, Move, Delete, NewProject, Archive       key.Binding
	Unarchive, Save, Yes, No                           key.Binding
	EditProfile, RevokeSession, RevokeAll, SignOut     key.Binding
	DeleteAccount, Server                              key.Binding
}

func defaultWorkspaceKeys() workspaceKeyMap {
	return workspaceKeyMap{
		Up:            bind("↑/k", "up", "up", "k"),
		Down:          bind("↓/j", "down", "down", "j"),
		PageUp:        bind("pgup", "page up", "pgup"),
		PageDown:      bind("pgdn", "page down", "pgdown"),
		PrevTab:       bind("[", "previous tab", "["),
		NextTab:       bind("]", "next tab", "]"),
		NextPanel:     bind("tab", "next panel", "tab"),
		PrevPanel:     bind("shift+tab", "previous panel", "shift+tab"),
		FocusMain:     bind("0", "main panel", "0"),
		FocusAccount:  bind("1", "account", "1"),
		FocusProjects: bind("2", "projects", "2"),
		FocusNotes:    bind("3", "notes", "3"),
		Open:          bind("enter", "open", "enter"),
		Back:          bind("esc", "back", "esc"),
		Refresh:       bind("r", "refresh", "r"),
		New:           bind("n", "new", "n"),
		Edit:          bind("e", "edit", "e"),
		Move:          bind("v", "move", "v"),
		Delete:        bind("d", "delete", "d"),
		NewProject:    bind("a", "new project", "a"),
		Archive:       bind("x", "archive", "x"),
		Unarchive:     bind("u", "unarchive", "u"),
		Save:          bind("ctrl+s", "save", "ctrl+s"),
		Yes:           bind("y", "yes", "y", "Y"),
		EditProfile:   bind("e", "edit profile", "e"),
		RevokeSession: bind("x", "revoke", "x"),
		RevokeAll:     bind("a", "revoke all", "a"),
		SignOut:       bind("l", "sign out", "l"),
		DeleteAccount: bind("D", "delete account", "D"),
		Server:        bind("s", "server", "s"),
		No:            bind("n/esc", "no", "n", "N", "esc"),
		Quit:          bind("q", "quit", "q"),
	}
}

// ShortHelp lists the footer hints for what has the keys, most useful first:
// an open popup, the editor, or the focused panel.
func (w workspaceModel) ShortHelp() []key.Binding {
	k := w.keys
	if w.popup != nil {
		return w.popup.ShortHelp(k)
	}
	if w.editor != nil {
		return w.editor.ShortHelp()
	}
	panels := bind("0-3", "panels", "0", "1", "2", "3")
	writable := !w.readOnly(w.visibleSource())
	switch w.focus {
	case focusProjects:
		if w.projects.tab == archivedTab {
			return []key.Binding{k.Unarchive, keySelect, bind("[/]", "active/archived", "[", "]"), bind("enter", "notes", "enter"), k.Refresh, panels, k.Quit}
		}
		return []key.Binding{k.NewProject, k.Archive, keySelect, bind("[/]", "active/archived", "[", "]"), bind("enter", "notes", "enter"), k.Refresh, panels, k.Quit}
	case focusNotes:
		tabs := bind("[/]", "inbox/project", "[", "]")
		if !writable {
			return []key.Binding{keySelect, tabs, bind("enter", "read", "enter"), k.Refresh, panels, k.Quit}
		}
		return []key.Binding{k.New, k.Edit, k.Move, k.Delete, keySelect, tabs, bind("enter", "read", "enter"), k.Refresh, panels, k.Quit}
	case focusMain:
		if w.back == focusAccount {
			return w.accountHelp(k.Back)
		}
		if !writable {
			return []key.Binding{bind("↑/↓", "scroll", "up", "down"), k.Back, panels, k.Quit}
		}
		return []key.Binding{k.Edit, k.New, bind("↑/↓", "scroll", "up", "down"), k.Back, panels, k.Quit}
	}
	return w.accountHelp(bind("enter", "open page", "enter"))
}

// accountHelp lists the account page's keys, once the account has loaded.
func (w workspaceModel) accountHelp(nav key.Binding) []key.Binding {
	k := w.keys
	panels := bind("0-3", "panels", "0", "1", "2", "3")
	if !w.accountState.loaded {
		return []key.Binding{k.Refresh, panels, k.Quit}
	}
	return []key.Binding{k.EditProfile, bind("↑/↓", "session", "up", "down"), k.RevokeSession, k.SignOut, k.Server, k.RevokeAll, k.DeleteAccount, nav, k.Refresh, panels, k.Quit}
}
