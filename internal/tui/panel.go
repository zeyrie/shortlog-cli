package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// frame draws a lazygit-style panel: a rounded border with the panel's
// number and title, or its tabs, set into the top edge, and an optional note
// such as a position counter set into the bottom edge. The focused panel's
// border takes the accent colour.
type frame struct {
	number  int // shown as [n]; negative for none
	title   string
	tabs    []string // when set, drawn instead of the title; tab is the active one
	tab     int
	footer  string
	focused bool
}

// render fits body into a width×height panel, border included. Lines are cut
// or padded to the inner width and the body to the inner height, so every
// panel keeps exactly the size the layout gave it.
func (f frame) render(body string, width, height int) string {
	width, height = max(width, 4), max(height, 2)
	inner, rows := width-2, height-2
	border := dimStyle
	if f.focused {
		border = lipgloss.NewStyle().Foreground(accentColor)
	}
	top := border.Render("╭─") + f.label(inner-3)
	if lipgloss.Width(top) < width-2 {
		top += " " // breathing room between the title and the border line
	}
	top += border.Render(strings.Repeat("─", max(0, width-1-lipgloss.Width(top))) + "╮")

	bottom := border.Render("╰")
	if note := ansi.Truncate(f.footer, max(0, inner-2), "…"); note != "" {
		bottom += border.Render(strings.Repeat("─", max(0, inner-1-lipgloss.Width(note)))) + dimStyle.Render(note) + border.Render("─╯")
	} else {
		bottom += border.Render(strings.Repeat("─", inner) + "╯")
	}

	lines := strings.Split(body, "\n")
	out := make([]string, 0, height)
	out = append(out, top)
	for i := 0; i < rows; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], inner, "…")
		}
		out = append(out, border.Render("│")+line+strings.Repeat(" ", max(0, inner-lipgloss.Width(line)))+border.Render("│"))
	}
	return strings.Join(append(out, bottom), "\n")
}

// label is the text set into the top border, cut to fit.
func (f frame) label(width int) string {
	var text string
	if f.number >= 0 {
		text = fmt.Sprintf("[%d] ", f.number)
	}
	style := titleStyle
	if !f.focused {
		style = lipgloss.NewStyle().Bold(true)
	}
	if len(f.tabs) == 0 {
		return ansi.Truncate(style.Render(text+f.title), width, "…")
	}
	parts := make([]string, len(f.tabs))
	for i, tab := range f.tabs {
		if i == f.tab {
			parts[i] = style.Render(tab)
		} else {
			parts[i] = dimStyle.Render(tab)
		}
	}
	return ansi.Truncate(style.Render(text)+strings.Join(parts, dimStyle.Render(" │ ")), width, "…")
}

// scrollList is the cursor and scroll window of a list panel. The window
// follows the cursor, so the selection is always visible.
type scrollList struct {
	cursor int
	offset int
}

// move shifts the cursor by delta within count items, keeping it inside a
// window of rows lines.
func (s *scrollList) move(delta, count, rows int) {
	if count == 0 {
		s.cursor, s.offset = 0, 0
		return
	}
	s.cursor = min(max(s.cursor+delta, 0), count-1)
	s.clamp(count, rows)
}

// clamp keeps the window around the cursor after the list or its height
// changed.
func (s *scrollList) clamp(count, rows int) {
	rows = max(rows, 1)
	s.cursor = min(max(s.cursor, 0), max(count-1, 0))
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+rows {
		s.offset = s.cursor - rows + 1
	}
	s.offset = min(max(s.offset, 0), max(count-rows, 0))
}

// window returns the visible index range [start, end).
func (s scrollList) window(count, rows int) (int, int) {
	return s.offset, min(s.offset+max(rows, 1), count)
}

// counter is the position note for a panel's bottom edge, shown only when the
// list does not fit.
func (s scrollList) counter(count, rows int) string {
	if count <= rows || count == 0 {
		return ""
	}
	return fmt.Sprintf(" %d of %d ", s.cursor+1, count)
}

// listRow renders one row of a list panel, one column in from the border: an
// accent rail marks the selected row, brighter when the panel has focus.
func listRow(text string, selected, focused bool, width int) string {
	text = ansi.Truncate(text, max(width-3, 1), "…")
	switch {
	case selected && focused:
		return " " + menuRailStyle.Render("▌") + " " + titleStyle.Render(text)
	case selected:
		return " " + dimStyle.Render("▌") + " " + lipgloss.NewStyle().Bold(true).Render(text)
	}
	return "   " + text
}

// stateRows is what a list panel shows instead of, or after, its items while
// loading or after a failure. With no items it fills the panel; with items it
// is one trailing row, so what already loaded stays readable.
func stateRows(state loadState, hasItems bool, spinner, what string) []string {
	switch {
	case state.loading && !hasItems:
		return []string{" " + spinner + " " + dimStyle.Render("Loading "+what+"…")}
	case state.err != "" && !hasItems:
		return []string{" " + errStyle.Render("✗ Could not load "+what), "   " + dimStyle.Render("r to retry")}
	case state.loading:
		return []string{" " + spinner + " " + dimStyle.Render("Loading more…")}
	case state.err != "":
		return []string{" " + errStyle.Render("✗ ") + dimStyle.Render("Could not load more · r to retry")}
	}
	return nil
}
