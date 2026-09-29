package tui

import (
	"context"
	"errors"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"shortlog-cli/internal/api"
)

// workspaceWriteAPI is the part of the Shortlog API the workspace changes
// things with.
type workspaceWriteAPI interface {
	CreateInboxNote(context.Context, string, string) (api.Note, error)
	CreateProjectNote(context.Context, string, string, string) (api.Note, error)
	UpdateNote(context.Context, string, string, string) (api.Note, error)
	MoveNote(context.Context, string, string, string) (api.Note, error)
	DeleteNote(context.Context, string, string) error
	CreateProject(context.Context, string, string) (api.Project, error)
	ArchiveProject(context.Context, string, string) error
	UnarchiveProject(context.Context, string, string) error
}

// workspaceClient is everything the workspace needs from the API.
type workspaceClient interface {
	workspaceAPI
	workspaceWriteAPI
}

// Results of the workspace's changes. Like its loads, each carries the
// session token so a late result is ignored.
type (
	noteCreated struct {
		token, source string
		note          api.Note
		err           error
	}
	noteUpdated struct {
		token string
		note  api.Note
		err   error
	}
	noteDeleted struct {
		token, source, id string
		err               error
	}
	noteMoved struct {
		token, from string
		note        api.Note
		err         error
	}
	projectAdded struct {
		token   string
		project api.Project
		err     error
	}
	projectArchived struct {
		token, id string
		archived  bool
		err       error
	}
)

// unsentDraft is text a save could not deliver because the session ended.
// The root keeps it across signing in again, and the new workspace reopens
// it: a new note in the capture popup, an edit in the editor.
type unsentDraft struct {
	source string
	noteID string // empty for a new note
	text   string
}

func apiCode(err error) string {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return ""
}

// readOnly reports whether a source's notes cannot change: those of an
// archived project.
func (w workspaceModel) readOnly(source string) bool {
	if source == inboxSource {
		return false
	}
	if w.notes.current != nil && w.notes.current.ID == source {
		return w.notes.current.ArchivedAt != nil
	}
	return slices.ContainsFunc(w.projects.projects[archivedTab], func(p api.Project) bool { return p.ID == source })
}

// placeName names a note source for titles and messages.
func (w workspaceModel) placeName(source string) string {
	if source == inboxSource {
		return "Inbox"
	}
	for _, list := range w.projects.projects {
		for _, p := range list {
			if p.ID == source {
				return p.Name
			}
		}
	}
	return "project"
}

func (w *workspaceModel) warnReadOnly() {
	w.note = &statusNote{level: statusWarn, text: "Archived projects are read-only. Unarchive it to change its notes."}
}

// startNewNote opens the capture popup for the notes tab on show.
func (w *workspaceModel) startNewNote() tea.Cmd {
	source := w.visibleSource()
	if w.readOnly(source) {
		w.warnReadOnly()
		return nil
	}
	w.popup = newCapture("New note · "+w.placeName(source), "", pendingAction{kind: newNoteAction, source: source})
	w.popup.setSize(w.width, w.height)
	return nil
}

// startEdit opens the selected note in the main panel's editor.
func (w *workspaceModel) startEdit() {
	note, ok := w.notes.selected()
	source := w.visibleSource()
	switch {
	case !ok:
		return
	case w.readOnly(source):
		w.warnReadOnly()
		return
	}
	w.openEditor(notePlace{Note: note, place: w.placeName(source)}, source, note.Content)
}

func (w *workspaceModel) openEditor(note notePlace, source, text string) {
	editor := newNoteEditor(note, source, text)
	_, mainWidth := w.columnWidths()
	editor.setSize(mainWidth, w.height)
	w.editor = &editor
	w.setFocus(focusMain)
}

// startMove offers the places the selected note can move to: the Inbox and
// every other active project.
func (w *workspaceModel) startMove() {
	note, ok := w.notes.selected()
	source := w.visibleSource()
	switch {
	case !ok:
		return
	case w.readOnly(source):
		w.warnReadOnly()
		return
	}
	var options []menuOption
	if source != inboxSource {
		options = append(options, menuOption{id: inboxSource, label: "Inbox"})
	}
	for _, p := range w.projects.projects[activeTab] {
		if p.ID != source {
			options = append(options, menuOption{id: p.ID, label: p.Name})
		}
	}
	if len(options) == 0 {
		w.note = &statusNote{level: statusInfo, text: "There is nowhere else to move it. Create a project first."}
		return
	}
	w.popup = newMenu("Move “"+noteTitle(note.Content)+"” to", options, pendingAction{kind: moveNoteAction, source: source, noteID: note.ID})
}

func (w *workspaceModel) startDelete() {
	note, ok := w.notes.selected()
	source := w.visibleSource()
	switch {
	case !ok:
		return
	case w.readOnly(source):
		w.warnReadOnly()
		return
	}
	title := noteTitle(note.Content)
	w.popup = newConfirm("Delete note", "Permanently delete “"+title+"”? There is no Trash, so this cannot be undone.", "delete", true,
		pendingAction{kind: deleteNoteAction, source: source, noteID: note.ID, name: title})
}

func (w *workspaceModel) startNewProject() {
	w.popup = newInput("New project", "Project name", 120, pendingAction{kind: newProjectAction})
	w.popup.setSize(w.width, w.height)
}

// startArchiveToggle archives the selected active project, after asking, or
// unarchives the selected archived one straight away.
func (w *workspaceModel) startArchiveToggle(archive bool) tea.Cmd {
	p, ok := w.projects.selected()
	if !ok || (archive != (w.projects.tab == activeTab)) {
		return nil
	}
	if archive {
		w.popup = newConfirm("Archive project", "Archive “"+p.Name+"”? Its notes stay readable, but can't be changed until you unarchive it.", "archive", false,
			pendingAction{kind: archiveProjectAction, id: p.ID, name: p.Name})
		return nil
	}
	return w.setArchived(p.ID, false)
}

// confirmQuit asks before quitting would throw away unsaved text.
func (w *workspaceModel) confirmQuit() {
	w.popup = newConfirm("Quit", "You have unsaved text. Quit and discard it?", "quit", true, pendingAction{kind: quitAction})
}

// unsaved reports whether the capture popup or the editor holds changes.
func (w workspaceModel) unsaved() bool {
	return (w.popup != nil && w.popup.changed()) || (w.editor != nil && w.editor.changed())
}

// acceptPopup starts what an accepted popup asked for.
func (w *workspaceModel) acceptPopup() tea.Cmd {
	p := w.popup
	client, token, action, value := w.api, w.token, p.action, p.value()
	switch action.kind {
	case discardEditAction:
		w.popup, w.editor = nil, nil
		w.setFocus(w.back)
		return nil
	case quitAction:
		return tea.Quit
	}
	p.busy, p.err = true, ""
	switch action.kind {
	case newNoteAction:
		return func() tea.Msg {
			var note api.Note
			var err error
			if action.source == inboxSource {
				note, err = client.CreateInboxNote(context.Background(), token, value)
			} else {
				note, err = client.CreateProjectNote(context.Background(), token, action.source, value)
			}
			return noteCreated{token: token, source: action.source, note: note, err: err}
		}
	case deleteNoteAction:
		return func() tea.Msg {
			return noteDeleted{token: token, source: action.source, id: action.noteID, err: client.DeleteNote(context.Background(), token, action.noteID)}
		}
	case moveNoteAction:
		return func() tea.Msg {
			note, err := client.MoveNote(context.Background(), token, action.noteID, value)
			return noteMoved{token: token, from: action.source, note: note, err: err}
		}
	case newProjectAction:
		return func() tea.Msg {
			project, err := client.CreateProject(context.Background(), token, value)
			return projectAdded{token: token, project: project, err: err}
		}
	case archiveProjectAction:
		return w.setArchived(action.id, true)
	}
	return nil
}

func (w *workspaceModel) setArchived(id string, archived bool) tea.Cmd {
	client, token := w.api, w.token
	return func() tea.Msg {
		var err error
		if archived {
			err = client.ArchiveProject(context.Background(), token, id)
		} else {
			err = client.UnarchiveProject(context.Background(), token, id)
		}
		return projectArchived{token: token, id: id, archived: archived, err: err}
	}
}

// saveEdit sends the editor's text.
func (w *workspaceModel) saveEdit() tea.Cmd {
	e := w.editor
	e.busy, e.err = true, ""
	client, token, id, text := w.api, w.token, e.noteID, e.editor.Value()
	return func() tea.Msg {
		note, err := client.UpdateNote(context.Background(), token, id, text)
		return noteUpdated{token: token, note: note, err: err}
	}
}

// writeFailed handles a 401 on a change: the session ends, keeping any
// unsent text for after signing in again.
func (w *workspaceModel) writeFailed(err error, draft *unsentDraft) bool {
	if !unauthorized(err) {
		return false
	}
	w.expired, w.unsent = true, draft
	return true
}

func (w workspaceModel) onNoteCreated(msg noteCreated) workspaceModel {
	if w.popup == nil || w.popup.action.kind != newNoteAction {
		return w
	}
	p := w.popup
	p.busy = false
	if msg.err != nil {
		switch {
		case w.writeFailed(msg.err, &unsentDraft{source: msg.source, text: p.value()}):
		case apiCode(msg.err) == "invalid_request":
			p.err = "The server rejected this note. Check its content and try again."
		default:
			p.err = "Save not confirmed. Check the notes before retrying to avoid a duplicate. Your text is still here."
		}
		return w
	}
	w.popup = nil
	set := w.sets[msg.source]
	set.notes = insertNote(set.notes, msg.note)
	w.sets[msg.source] = set
	if w.visibleSource() == msg.source {
		w.notes.lists[w.notes.tab] = scrollList{}
	}
	w.note = &statusNote{level: statusInfo, text: "Note saved to " + w.placeName(msg.source) + "."}
	w.syncNotes()
	return w
}

func (w workspaceModel) onNoteUpdated(msg noteUpdated) workspaceModel {
	if w.editor == nil {
		return w
	}
	e := w.editor
	e.busy = false
	if msg.err != nil {
		switch {
		case w.writeFailed(msg.err, &unsentDraft{source: e.source, noteID: e.noteID, text: e.editor.Value()}):
		case apiCode(msg.err) == "invalid_request":
			e.err = "The server rejected these changes. Check the note and try again."
		case apiCode(msg.err) == "not_found":
			e.err = "This note no longer exists. Copy your text before leaving."
		default:
			e.err = "Update not confirmed. Check the note before retrying; your text is still here."
		}
		return w
	}
	for source, set := range w.sets {
		for i := range set.notes {
			if set.notes[i].ID == msg.note.ID {
				set.notes = slices.Clone(set.notes)
				set.notes[i] = msg.note
				w.sets[source] = set
			}
		}
	}
	w.editor = nil
	w.reader.note = "" // show the saved text, not the cached layout
	w.note = &statusNote{level: statusInfo, text: "Note updated."}
	w.syncNotes()
	return w
}

func (w workspaceModel) onNoteDeleted(msg noteDeleted) workspaceModel {
	w.popup = nil
	if msg.err != nil {
		if !w.writeFailed(msg.err, nil) {
			w.note = &statusNote{level: statusError, text: "Delete not confirmed. Refresh with r before trying again."}
		}
		return w
	}
	w.removeNote(msg.source, msg.id)
	w.note = &statusNote{level: statusInfo, text: "Note deleted."}
	w.syncNotes()
	return w
}

func (w workspaceModel) onNoteMoved(msg noteMoved) workspaceModel {
	w.popup = nil
	if msg.err != nil {
		switch {
		case w.writeFailed(msg.err, nil):
		case apiCode(msg.err) == "conflict":
			w.note = &statusNote{level: statusError, text: "That project may have been archived. Refresh projects with r and try again."}
		case apiCode(msg.err) == "not_found":
			w.note = &statusNote{level: statusError, text: "The note or project no longer exists. Refresh with r and try again."}
		default:
			w.note = &statusNote{level: statusError, text: "Move not confirmed. Refresh with r before retrying."}
		}
		return w
	}
	w.removeNote(msg.from, msg.note.ID)
	destination := inboxSource
	if msg.note.ProjectID != nil {
		destination = *msg.note.ProjectID
	}
	if set, ok := w.sets[destination]; ok && set.loaded {
		set.notes = insertNote(set.notes, msg.note)
		w.sets[destination] = set
	}
	w.note = &statusNote{level: statusInfo, text: "Moved to " + w.placeName(destination) + "."}
	w.syncNotes()
	return w
}

func (w workspaceModel) onProjectAdded(msg projectAdded) (workspaceModel, tea.Cmd) {
	if w.popup == nil || w.popup.action.kind != newProjectAction {
		return w, nil
	}
	p := w.popup
	p.busy = false
	if msg.err != nil {
		switch {
		case w.writeFailed(msg.err, nil):
		case apiCode(msg.err) == "invalid_request":
			p.err = "The server rejected that name. A project name is 1–120 characters."
		default:
			p.err = "Creation not confirmed. Check the projects before retrying."
		}
		return w, nil
	}
	w.popup = nil
	active := append([]api.Project{msg.project}, w.projects.projects[activeTab]...)
	w.projects.setProjects(active, w.projects.projects[archivedTab])
	w.projects.tab, w.projects.lists[activeTab] = activeTab, scrollList{}
	w.sets[msg.project.ID] = noteSet{loadState: loadState{loaded: true}}
	w.setSize(w.width, w.height)
	w.note = &statusNote{level: statusInfo, text: "Created " + msg.project.Name + "."}
	return w, w.showSelectedProject()
}

func (w workspaceModel) onProjectArchived(msg projectArchived) (workspaceModel, tea.Cmd) {
	w.popup = nil
	if msg.err != nil {
		if !w.writeFailed(msg.err, nil) {
			w.note = &statusNote{level: statusError, text: "Project change not confirmed. Refresh with r before retrying."}
		}
		return w, nil
	}
	from, to := activeTab, archivedTab
	if !msg.archived {
		from, to = archivedTab, activeTab
	}
	var moved api.Project
	var lists [2][]api.Project
	lists[from] = slices.DeleteFunc(slices.Clone(w.projects.projects[from]), func(p api.Project) bool {
		if p.ID == msg.id {
			moved = p
			return true
		}
		return false
	})
	if moved.ID == "" {
		return w, nil
	}
	if msg.archived {
		now := time.Now()
		moved.ArchivedAt = &now
		w.note = &statusNote{level: statusInfo, text: "Archived " + moved.Name + ". Its notes are read-only now."}
	} else {
		moved.ArchivedAt = nil
		w.note = &statusNote{level: statusInfo, text: "Unarchived " + moved.Name + "."}
	}
	lists[to] = append([]api.Project{moved}, w.projects.projects[to]...)
	w.projects.setProjects(lists[activeTab], lists[archivedTab])
	w.setSize(w.width, w.height)
	if w.notes.current != nil && w.notes.current.ID == moved.ID {
		w.notes.current = &moved // the tab's archived mark follows
	}
	return w, w.showSelectedProject()
}

// removeNote drops a note from a source's loaded notes.
func (w *workspaceModel) removeNote(source, id string) {
	set := w.sets[source]
	set.notes = slices.DeleteFunc(slices.Clone(set.notes), func(n api.Note) bool { return n.ID == id })
	w.sets[source] = set
}

// insertNote adds a note to a newest-first list in its place by creation
// time, so a moved note lands among its contemporaries.
func insertNote(notes []api.Note, note api.Note) []api.Note {
	i, _ := slices.BinarySearchFunc(notes, note, func(a, b api.Note) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return slices.Insert(slices.Clone(notes), i, note)
}

// resumeDraft reopens text a previous session could not save. A new note
// opens in the capture popup at once; an edit waits for its note to load.
func (w *workspaceModel) resumeDraft(d *unsentDraft) tea.Cmd {
	w.note = &statusNote{level: statusInfo, text: "Restored your unsaved text."}
	if d.noteID == "" {
		w.popup = newCapture("New note · restored", d.text, pendingAction{kind: newNoteAction, source: d.source})
		w.popup.start = "" // restored text counts as a change worth keeping
		w.popup.setSize(w.width, w.height)
		return nil
	}
	w.pendingEdit = d
	if d.source != inboxSource {
		return w.loadNotes(d.source, "")
	}
	return nil
}

// reopenPendingEdit opens a restored edit once its source has loaded. If the
// note is gone, the text opens as a new note rather than being lost.
func (w *workspaceModel) reopenPendingEdit(source string) {
	d := w.pendingEdit
	if d == nil || d.source != source {
		return
	}
	w.pendingEdit = nil
	for _, note := range w.sets[source].notes {
		if note.ID == d.noteID {
			w.openEditor(notePlace{Note: note, place: w.placeName(source)}, source, d.text)
			return
		}
	}
	w.popup = newCapture("New note · restored", d.text, pendingAction{kind: newNoteAction, source: inboxSource})
	w.popup.start = ""
	w.popup.setSize(w.width, w.height)
	w.note = &statusNote{level: statusWarn, text: "The note you were editing is gone; your text is here as a new Inbox note."}
}
