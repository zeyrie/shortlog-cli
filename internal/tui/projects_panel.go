package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"shortlog-cli/internal/api"
)

// projectRows caps how many projects the panel shows at once; longer lists
// scroll inside it, with a position counter in the bottom edge.
const projectRows = 8

const (
	activeTab = iota
	archivedTab
)

// projectsPanel is panel [2]: the user's projects, split into Active and
// Archived tabs, each with its own cursor.
type projectsPanel struct {
	projects [2][]api.Project
	lists    [2]scrollList
	tab      int
}

func (p projectsPanel) items() []api.Project { return p.projects[p.tab] }

// selected is the highlighted project on the current tab.
func (p projectsPanel) selected() (api.Project, bool) {
	items := p.items()
	if len(items) == 0 {
		return api.Project{}, false
	}
	return items[p.lists[p.tab].cursor], true
}

// setProjects replaces the lists, keeping each tab's cursor where it can.
func (p *projectsPanel) setProjects(active, archived []api.Project) {
	p.projects = [2][]api.Project{active, archived}
	for tab := range p.lists {
		p.lists[tab].clamp(len(p.projects[tab]), projectRows)
	}
}

// rows is how many lines of projects the panel shows, before borders.
func (p projectsPanel) rows() int { return min(max(len(p.items()), 1), projectRows) }

// Update handles keys while the panel has focus. It reports whether the
// selected project changed, so the workspace can show its notes.
func (p projectsPanel) Update(msg tea.KeyPressMsg, keys workspaceKeyMap) (projectsPanel, bool) {
	before, _ := p.selected()
	list := &p.lists[p.tab]
	switch {
	case key.Matches(msg, keys.Up):
		list.move(-1, len(p.items()), p.rows())
	case key.Matches(msg, keys.Down):
		list.move(1, len(p.items()), p.rows())
	case key.Matches(msg, keys.PageUp):
		list.move(-projectRows, len(p.items()), p.rows())
	case key.Matches(msg, keys.PageDown):
		list.move(projectRows, len(p.items()), p.rows())
	case key.Matches(msg, keys.PrevTab), key.Matches(msg, keys.NextTab):
		p.tab = 1 - p.tab
	}
	after, _ := p.selected()
	return p, after.ID != before.ID
}

func (p projectsPanel) View(width, height int, focused bool, state loadState, spinner string) string {
	items, list := p.items(), p.lists[p.tab]
	rows := max(height-2, 1)
	list.clamp(len(items), rows) // a short terminal may show fewer rows than the cap
	body := stateRows(state, len(items) > 0, spinner, "projects")
	if len(items) == 0 && len(body) == 0 {
		empty := "No active projects"
		if p.tab == archivedTab {
			empty = "No archived projects"
		}
		body = append(body, "   "+dimStyle.Render(empty))
	}
	var rowsOut []string
	start, end := list.window(len(items), rows)
	for i := start; i < end; i++ {
		rowsOut = append(rowsOut, listRow(safeText(items[i].Name), i == list.cursor, focused, width-2))
	}
	body = append(rowsOut, body...)
	f := frame{number: 2, tabs: []string{"Active", "Archived"}, tab: p.tab, footer: list.counter(len(items), rows), focused: focused}
	return f.render(joinLines(body), width, height)
}
