package tui

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// noteView is the main panel [0] when a note is selected: the whole note,
// word-wrapped to the panel and scrollable once the panel has focus.
type noteView struct {
	vp    viewport.Model
	note  string // ID of the note shown, to keep the scroll position
	title string
	text  string // the unwrapped content, re-wrapped when the panel resizes
	has   bool
}

func newNoteView() noteView { return noteView{vp: viewport.New()} }

// setSize fits the viewport inside the panel's border, with a column of
// padding on each side.
func (n *noteView) setSize(width, height int) {
	n.vp.SetWidth(max(width-4, 1))
	n.vp.SetHeight(max(height-2, 1))
	n.wrap()
}

// wrap lays the note out at the viewport width. Lip Gloss wraps at word
// boundaries; the viewport's own soft wrap would split words.
func (n *noteView) wrap() {
	offset := n.vp.YOffset()
	n.vp.SetContent(lipgloss.NewStyle().Width(n.vp.Width()).Render(n.text))
	n.vp.SetYOffset(offset)
}

// show displays a note with where it lives; showing the same note again
// keeps the reader's scroll position.
func (n *noteView) show(note notePlace, ok bool) {
	if !ok {
		n.has, n.note, n.title, n.text = false, "", "", ""
		n.vp.SetContent("")
		return
	}
	if n.has && n.note == note.ID {
		return
	}
	n.has, n.note, n.title = true, note.ID, noteTitle(note.Content)
	header := dimStyle.Render(note.CreatedAt.Local().Format("Mon, Jan 02 2006 · 15:04") + " · " + safeText(note.place))
	n.text = header + "\n\n" + safeText(note.Content)
	n.wrap()
	n.vp.GotoTop()
}

func (n noteView) Update(msg tea.Msg) (noteView, tea.Cmd) {
	var cmd tea.Cmd
	n.vp, cmd = n.vp.Update(msg)
	return n, cmd
}

func (n noteView) View(width, height int, focused bool) string {
	f := frame{number: 0, title: "Note", focused: focused}
	if !n.has {
		return f.render(" "+dimStyle.Render("Select a note to read it here."), width, height)
	}
	f.title = n.title
	if n.vp.TotalLineCount() > n.vp.Height() {
		f.footer = fmt.Sprintf(" %d%% ", int(n.vp.ScrollPercent()*100))
	}
	return f.render(lipgloss.NewStyle().PaddingLeft(1).Render(n.vp.View()), width, height)
}
