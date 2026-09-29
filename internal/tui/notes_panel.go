package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"shortlog-cli/internal/api"
)

const (
	inboxTab = iota
	projectTab
)

// notesPanel is panel [3]. Its Inbox tab lists notes outside any project; its
// second tab lists the notes of the project selected in panel [2]. Each tab
// keeps its own cursor.
type notesPanel struct {
	inbox   []api.Note
	project []api.Note
	// current is the project the second tab shows; nil before any is chosen.
	current *api.Project
	lists   [2]scrollList
	tab     int
	rows    int // visible note rows, set by the workspace layout
}

func (n notesPanel) items() []api.Note {
	if n.tab == projectTab {
		return n.project
	}
	return n.inbox
}

func (n notesPanel) selected() (api.Note, bool) {
	items := n.items()
	if len(items) == 0 {
		return api.Note{}, false
	}
	return items[n.lists[n.tab].cursor], true
}

// showProject points the second tab at a project and switches to it. A
// different project starts at its newest note.
func (n *notesPanel) showProject(p api.Project, notes []api.Note) {
	if n.current == nil || n.current.ID != p.ID {
		n.lists[projectTab] = scrollList{}
	}
	project := p
	n.current, n.project, n.tab = &project, notes, projectTab
	n.lists[projectTab].clamp(len(notes), n.rows)
}

// setRows fits the lists to a new panel height.
func (n *notesPanel) setRows(rows int) {
	n.rows = max(rows, 1)
	n.lists[inboxTab].clamp(len(n.inbox), n.rows)
	n.lists[projectTab].clamp(len(n.project), n.rows)
}

// Update handles keys while the panel has focus and reports whether the
// selected note changed.
func (n notesPanel) Update(msg tea.KeyPressMsg, keys workspaceKeyMap) (notesPanel, bool) {
	before, _ := n.selected()
	list := &n.lists[n.tab]
	switch {
	case key.Matches(msg, keys.Up):
		list.move(-1, len(n.items()), n.rows)
	case key.Matches(msg, keys.Down):
		list.move(1, len(n.items()), n.rows)
	case key.Matches(msg, keys.PageUp):
		list.move(-n.rows, len(n.items()), n.rows)
	case key.Matches(msg, keys.PageDown):
		list.move(n.rows, len(n.items()), n.rows)
	case key.Matches(msg, keys.PrevTab), key.Matches(msg, keys.NextTab):
		if n.current != nil {
			n.tab = 1 - n.tab
		}
	}
	after, _ := n.selected()
	return n, after.ID != before.ID
}

// tabs names the two tabs; the second is the chosen project, marked when it
// is archived (and so read-only). The name gives way to fit the panel's top
// edge, so the marker is never the part that gets cut.
func (n notesPanel) tabs(width int) []string {
	if n.current == nil {
		return []string{"Inbox"}
	}
	marker := ""
	if n.current.ArchivedAt != nil {
		marker = " (archived)"
	}
	room := width - len("╭─[3] Inbox │ ") - len(marker) - 3
	return []string{"Inbox", ansi.Truncate(safeText(n.current.Name), max(room, 4), "…") + marker}
}

func (n notesPanel) View(width, height int, focused bool) string {
	items, list := n.items(), n.lists[n.tab]
	rows := max(height-2, 1)
	list.clamp(len(items), rows)
	var body []string
	if len(items) == 0 {
		empty := "No notes in the Inbox"
		if n.tab == projectTab {
			empty = "No notes in this project"
		}
		body = append(body, "   "+dimStyle.Render(empty))
	}
	start, end := list.window(len(items), rows)
	for i := start; i < end; i++ {
		note := items[i]
		row := dimStyle.Render(note.CreatedAt.Local().Format("Jan 02")) + "  " + noteTitle(note.Content)
		body = append(body, listRow(row, i == list.cursor, focused, width-2))
	}
	f := frame{number: 3, tabs: n.tabs(width), tab: n.tab, footer: list.counter(len(items), rows), focused: focused}
	return f.render(joinLines(body), width, height)
}

// noteTitle is a note's first non-blank line, which stands in for a title.
func noteTitle(content string) string {
	for _, line := range strings.Split(safeText(content), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "(empty note)"
}

func joinLines(lines []string) string { return strings.Join(lines, "\n") }
