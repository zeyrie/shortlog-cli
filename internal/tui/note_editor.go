package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type editorOutcome int

const (
	editorOpen editorOutcome = iota
	editorSave
	editorCancel // leave; the workspace asks first if the text changed
)

// noteEditor edits an existing note in the main panel, in place of the
// reader. Ctrl+S saves; Esc leaves, asking first if the text changed. It
// stays open, busy, while the save runs, and keeps the text if it fails.
type noteEditor struct {
	editor textarea.Model
	noteID string
	source string
	title  string
	start  string
	busy   bool
	err    string
}

func newNoteEditor(note notePlace, source, text string) noteEditor {
	ed := textarea.New()
	ed.CharLimit = maxNoteLength
	ed.ShowLineNumbers = false
	ed.Prompt = ""
	styles := ed.Styles()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	ed.SetStyles(styles)
	ed.SetValue(text)
	ed.Focus()
	return noteEditor{editor: ed, noteID: note.ID, source: source, title: noteTitle(note.Content), start: note.Content}
}

func (e *noteEditor) setSize(width, height int) {
	e.editor.SetWidth(max(width-4, 1))
	e.editor.SetHeight(max(height-5, 1))
}

func (e noteEditor) changed() bool { return e.editor.Value() != e.start }

func (e noteEditor) Update(msg tea.Msg, keys workspaceKeyMap) (noteEditor, tea.Cmd, editorOutcome) {
	if e.busy {
		return e, nil, editorOpen
	}
	if press, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(press, keys.Save):
			if strings.TrimSpace(e.editor.Value()) == "" {
				e.err = "A note can't be empty. Delete it instead."
				return e, nil, editorOpen
			}
			return e, nil, editorSave
		case key.Matches(press, keys.Back):
			return e, nil, editorCancel
		}
	}
	var cmd tea.Cmd
	e.editor, cmd = e.editor.Update(msg)
	return e, cmd, editorOpen
}

func (e noteEditor) View(width, height int, spinner string) string {
	status := dimStyle.Render(fmt.Sprintf("%d / %d", utf8.RuneCountInString(e.editor.Value()), maxNoteLength))
	if e.changed() {
		status += dimStyle.Render(" · unsaved")
	}
	switch {
	case e.busy:
		status = spinner + " " + dimStyle.Render("Saving…")
	case e.err != "":
		status = errStyle.Render("✗ " + e.err)
	}
	body := lipgloss.NewStyle().PaddingLeft(1).Render(e.editor.View()) + "\n\n " + status
	return frame{number: 0, title: "Editing · " + e.title, focused: true}.render(body, width, height)
}

func (e noteEditor) ShortHelp() []key.Binding {
	if e.busy {
		return nil
	}
	return []key.Binding{bind("ctrl+s", "save", "ctrl+s"), bind("enter", "new line", "enter"), bind("esc", "cancel", "esc")}
}
