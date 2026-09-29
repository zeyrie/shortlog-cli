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

// workspaceData is everything the workspace shows. It is static sample data
// for now; the next step loads it from the API.
type workspaceData struct {
	account  api.Account
	sessions []api.Session
	server   string
	active   []api.Project
	archived []api.Project
	inbox    []api.Note
	notes    map[string][]api.Note // by project ID
}

// notePlace is a note together with where it lives, for the reader header.
type notePlace struct {
	api.Note
	place string
}

// workspaceModel is the signed-in screen, laid out like lazygit: stacked
// panels on the left ([1] account, [2] projects, [3] notes) and the main
// panel [0] on the right showing the selected note or the account page.
type workspaceModel struct {
	keys          workspaceKeyMap
	width, height int // the space above the footer
	focus         focusArea
	back          focusArea // where Esc returns from the main panel

	account  accountPanel
	projects projectsPanel
	notes    notesPanel
	reader   noteView
	data     workspaceData
}

func newWorkspace(data workspaceData) workspaceModel {
	w := workspaceModel{keys: defaultWorkspaceKeys(), focus: focusNotes, back: focusNotes, reader: newNoteView(), data: data}
	w.account = accountPanel{account: data.account, sessions: data.sessions, server: data.server}
	w.projects.setProjects(data.active, data.archived)
	w.notes.inbox = data.inbox
	if p, ok := w.projects.selected(); ok {
		// The project tab is ready from the start, but the Inbox shows first.
		w.notes.showProject(p, data.notes[p.ID])
		w.notes.tab = inboxTab
	}
	w.setSize(80, 22)
	w.syncReader()
	return w
}

func (w *workspaceModel) setSize(width, height int) {
	w.width, w.height = width, height
	_, _, notesHeight := w.columnHeights()
	w.notes.setRows(notesHeight - 2)
	_, mainWidth := w.columnWidths()
	w.reader.setSize(mainWidth, height)
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
	case key.Matches(msg2, k.Open) && w.focus != focusMain:
		// Enter drills in: a project to its notes, a note or the account
		// line to the main panel.
		if w.focus == focusProjects {
			w.showSelectedProject()
			w.setFocus(focusNotes)
		} else {
			w.setFocus(focusMain)
		}
	case key.Matches(msg2, k.Back) && w.focus == focusMain:
		w.setFocus(w.back)
	default:
		return w.updateFocused(msg2)
	}
	return w, nil
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
			w.showSelectedProject()
		}
	case focusNotes:
		w.notes, _ = w.notes.Update(msg, w.keys)
		w.syncReader()
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
// highlighted in panel [2].
func (w *workspaceModel) showSelectedProject() {
	if p, ok := w.projects.selected(); ok {
		w.notes.showProject(p, w.data.notes[p.ID])
	}
	w.syncReader()
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

func (w workspaceModel) View() string {
	leftWidth, mainWidth := w.columnWidths()
	accountHeight, projectsHeight, notesHeight := w.columnHeights()
	left := lipgloss.JoinVertical(lipgloss.Left,
		w.account.View(leftWidth, accountHeight, w.focus == focusAccount),
		w.projects.View(leftWidth, projectsHeight, w.focus == focusProjects),
		w.notes.View(leftWidth, notesHeight, w.focus == focusNotes),
	)
	main := w.mainView(mainWidth)
	if w.width < narrowWidth {
		if w.focus == focusMain {
			return main
		}
		return left
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, main)
}

// mainView is the account page while the account is in view, and otherwise
// the note selected in panel [3].
func (w workspaceModel) mainView(width int) string {
	focused := w.focus == focusMain
	if w.focus == focusAccount || (focused && w.back == focusAccount) {
		return w.account.page(width, w.height, focused, time.Now())
	}
	return w.reader.View(width, w.height, focused)
}

// workspaceKeyMap is the workspace's bindings; panels match keys against it.
type workspaceKeyMap struct {
	Up, Down, PageUp, PageDown                         key.Binding
	PrevTab, NextTab, NextPanel, PrevPanel             key.Binding
	FocusMain, FocusAccount, FocusProjects, FocusNotes key.Binding
	Open, Back, Quit                                   key.Binding
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
		Quit:          bind("q", "quit", "q"),
	}
}

// ShortHelp lists the footer hints for the focused area, most useful first.
func (w workspaceModel) ShortHelp() []key.Binding {
	k := w.keys
	panels := bind("0-3", "panels", "0", "1", "2", "3")
	switch w.focus {
	case focusProjects:
		return []key.Binding{keySelect, bind("[/]", "active/archived", "[", "]"), bind("enter", "notes", "enter"), panels, k.Quit}
	case focusNotes:
		tabs := bind("[/]", "inbox/project", "[", "]")
		return []key.Binding{keySelect, tabs, bind("enter", "read", "enter"), panels, k.Quit}
	case focusMain:
		return []key.Binding{bind("↑/↓", "scroll", "up", "down"), k.Back, panels, k.Quit}
	}
	return []key.Binding{bind("enter", "account page", "enter"), panels, k.NextPanel, k.Quit}
}
