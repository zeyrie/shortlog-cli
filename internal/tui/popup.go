package tui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// overlay draws top centred over base, a little above the middle, using Lip
// Gloss's compositor so the workspace stays visible around it.
func overlay(base, top string, width, height int) string {
	x := max((width-lipgloss.Width(top))/2, 0)
	y := max((height-lipgloss.Height(top))/3, 0)
	canvas := lipgloss.NewCanvas(width, height)
	canvas.Compose(lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(top).X(x).Y(y).Z(1)))
	return canvas.Render()
}

type popupKind int

const (
	confirmPopup popupKind = iota + 1 // a yes/no question
	menuPopup                         // pick one of several options
	formPopup                         // one or more labelled one-line entries
	capturePopup                      // a new note
)

// actionKind is what a popup is for; the workspace acts on it when the popup
// is accepted.
type actionKind int

const (
	newNoteAction actionKind = iota + 1
	deleteNoteAction
	moveNoteAction
	newProjectAction
	archiveProjectAction
	editProfileAction
	revokeSessionAction
	revokeAllAction
	signOutAction
	deleteAccountAction
	serverAction
	discardEditAction
	quitAction
)

// pendingAction is a popup's purpose and what it applies to.
type pendingAction struct {
	kind    actionKind
	source  string // the note source a note action applies to
	noteID  string
	id      string // the project or session an action applies to
	name    string // for messages
	current bool   // a revoked session is this device's
}

type menuOption struct{ id, label string }

// popupField is one entry of a form popup, checked as the user leaves it and
// again, with the others, before the form is accepted.
type popupField struct {
	label    string
	input    textinput.Model
	validate func(string) error
}

func newField(label, placeholder, value string, limit int, validate func(string) error) popupField {
	in := textinput.New()
	in.Placeholder, in.CharLimit, in.Prompt = placeholder, limit, "> "
	in.SetValue(value)
	return popupField{label: label, input: in, validate: validate}
}

type popupOutcome int

const (
	popupOpen popupOutcome = iota
	popupAccepted
	popupCancelled
)

// popupState is the popup over the workspace. It takes every key while
// open. When it is accepted the workspace starts the action's request and
// the popup stays open, busy, until the result arrives: it closes on
// success, and shows the error and keeps the entry on failure.
type popupState struct {
	kind    popupKind
	title   string
	message string // a confirm popup's question
	accept  string // the confirm key's action, such as "delete"
	danger  bool   // a destructive confirm
	options []menuOption
	cursor  int
	window  scrollList // the menu's visible rows; long menus scroll
	fields  []popupField
	field   int // the focused form field
	editor  textarea.Model
	start   string // the capture's text when it opened, to tell if it changed
	// discarding is set while a changed capture asks whether to throw the
	// text away.
	discarding bool
	busy       bool
	err        string
	action     pendingAction
}

func newConfirm(title, message, accept string, danger bool, action pendingAction) *popupState {
	return &popupState{kind: confirmPopup, title: title, message: message, accept: accept, danger: danger, action: action}
}

func newMenu(title string, options []menuOption, action pendingAction) *popupState {
	return &popupState{kind: menuPopup, title: title, options: options, action: action}
}

// newForm opens a form. message, if set, explains what accepting does;
// accept names the Enter key's action on the last field.
func newForm(title, message, accept string, danger bool, fields []popupField, action pendingAction) *popupState {
	fields[0].input.Focus()
	return &popupState{kind: formPopup, title: title, message: message, accept: accept, danger: danger, fields: fields, action: action}
}

func newCapture(title, text string, action pendingAction) *popupState {
	ed := textarea.New()
	ed.Placeholder = "What's on your mind?"
	ed.CharLimit = maxNoteLength
	ed.ShowLineNumbers = false
	ed.Prompt = ""
	styles := ed.Styles()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	ed.SetStyles(styles)
	ed.SetValue(text)
	ed.Focus()
	return &popupState{kind: capturePopup, title: title, editor: ed, start: text, action: action}
}

// menuRows caps how many options a menu shows at once.
const menuRows = 10

// maxNoteLength is the server's limit on a note, in characters.
const maxNoteLength = 20000

// changed reports whether a capture holds text worth asking about before it
// is thrown away.
func (p popupState) changed() bool {
	return p.kind == capturePopup && p.editor.Value() != p.start
}

// value is what an accepted popup entered: the chosen option's ID, the
// input's text, or the capture's note.
func (p popupState) value() string {
	switch p.kind {
	case menuPopup:
		if len(p.options) > 0 {
			return p.options[p.cursor].id
		}
	case formPopup:
		return strings.TrimSpace(p.fields[0].input.Value())
	case capturePopup:
		return p.editor.Value()
	}
	return ""
}

// setSize fits a capture's editor to the popup.
func (p *popupState) setSize(width, height int) {
	if p.kind == capturePopup {
		w, h := captureSize(width, height)
		p.editor.SetWidth(w - 4)
		p.editor.SetHeight(max(h-5, 2))
	}
	for i := range p.fields {
		p.fields[i].input.SetWidth(popupWidth(width) - 8)
	}
}

// values are a form's entries, as typed.
func (p popupState) values() []string {
	values := make([]string, len(p.fields))
	for i, f := range p.fields {
		values[i] = f.input.Value()
	}
	return values
}

// focusField moves a form's focus, blurring the field it leaves.
func (p *popupState) focusField(i int) tea.Cmd {
	p.fields[p.field].input.Blur()
	p.field = (i + len(p.fields)) % len(p.fields)
	return p.fields[p.field].input.Focus()
}

func popupWidth(width int) int { return min(58, max(width-6, 20)) }

func captureSize(width, height int) (int, int) {
	return min(72, max(width-6, 24)), min(16, max(height-4, 8))
}

func (p popupState) Update(msg tea.Msg, keys workspaceKeyMap) (popupState, tea.Cmd, popupOutcome) {
	if p.busy {
		return p, nil, popupOpen // the request decides what happens next
	}
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p.updateField(msg)
	}
	switch p.kind {
	case confirmPopup:
		switch {
		case key.Matches(press, keys.Yes):
			return p, nil, popupAccepted
		case key.Matches(press, keys.No):
			return p, nil, popupCancelled
		}
		return p, nil, popupOpen
	case menuPopup:
		switch {
		case key.Matches(press, keys.Up):
			p.window.move(-1, len(p.options), menuRows)
			p.cursor = p.window.cursor
		case key.Matches(press, keys.Down):
			p.window.move(1, len(p.options), menuRows)
			p.cursor = p.window.cursor
		case key.Matches(press, keys.Open) && len(p.options) > 0:
			return p, nil, popupAccepted
		case key.Matches(press, keys.Back):
			return p, nil, popupCancelled
		}
		return p, nil, popupOpen
	case formPopup:
		switch {
		case key.Matches(press, keys.Open):
			if err := p.fields[p.field].validate(p.fields[p.field].input.Value()); err != nil {
				p.err = err.Error()
				return p, nil, popupOpen
			}
			if p.field < len(p.fields)-1 {
				p.err = ""
				return p, p.focusField(p.field + 1), popupOpen
			}
			for i, f := range p.fields {
				if err := f.validate(f.input.Value()); err != nil {
					p.err = err.Error()
					return p, p.focusField(i), popupOpen
				}
			}
			return p, nil, popupAccepted
		case key.Matches(press, keys.NextPanel) && len(p.fields) > 1:
			return p, p.focusField(p.field + 1), popupOpen
		case key.Matches(press, keys.PrevPanel) && len(p.fields) > 1:
			return p, p.focusField(p.field - 1), popupOpen
		case key.Matches(press, keys.Back):
			return p, nil, popupCancelled
		}
	case capturePopup:
		if p.discarding {
			switch {
			case key.Matches(press, keys.Yes):
				return p, nil, popupCancelled
			case key.Matches(press, keys.No):
				p.discarding = false
			}
			return p, nil, popupOpen
		}
		switch {
		case key.Matches(press, keys.Save):
			if strings.TrimSpace(p.editor.Value()) == "" {
				p.err = "Write something first."
				return p, nil, popupOpen
			}
			return p, nil, popupAccepted
		case key.Matches(press, keys.Back):
			if p.changed() {
				p.discarding = true
				return p, nil, popupOpen
			}
			return p, nil, popupCancelled
		}
	}
	return p.updateField(msg)
}

// updateField passes a message to the popup's text field, if it has one.
func (p popupState) updateField(msg tea.Msg) (popupState, tea.Cmd, popupOutcome) {
	var cmd tea.Cmd
	switch p.kind {
	case formPopup:
		p.fields[p.field].input, cmd = p.fields[p.field].input.Update(msg)
		p.err = ""
	case capturePopup:
		p.editor, cmd = p.editor.Update(msg)
	}
	return p, cmd, popupOpen
}

func (p popupState) View(width, height int, spinner string) string {
	w := popupWidth(width)
	if p.kind == capturePopup {
		w, _ = captureSize(width, height)
	}
	inner := w - 4
	var lines []string
	footer := ""
	switch p.kind {
	case confirmPopup:
		lines = append(lines, strings.Split(lipgloss.NewStyle().Width(inner).Render(safeText(p.message)), "\n")...)
		lines = append(lines, "")
		yes := helpKeyStyle.Render("y") + dimStyle.Render(" "+p.accept)
		if p.danger {
			yes = errStyle.Render("y") + errStyle.Render(" "+p.accept)
		}
		lines = append(lines, yes+dimStyle.Render("  ·  ")+helpKeyStyle.Render("n")+dimStyle.Render(" cancel"))
	case menuPopup:
		rows := min(menuRows, max(height-6, 3))
		list := p.window
		list.clamp(len(p.options), rows)
		start, end := list.window(len(p.options), rows)
		for i := start; i < end; i++ {
			lines = append(lines, listRow(safeText(p.options[i].label), i == p.cursor, true, inner))
		}
		footer = list.counter(len(p.options), rows)
	case formPopup:
		if p.message != "" {
			lines = append(lines, strings.Split(lipgloss.NewStyle().Width(inner).Render(safeText(p.message)), "\n")...)
			lines = append(lines, "")
		}
		for i, f := range p.fields {
			if len(p.fields) > 1 {
				label := dimStyle
				if i == p.field {
					label = titleStyle
				}
				lines = append(lines, label.Render(f.label))
			}
			lines = append(lines, f.input.View())
			if i < len(p.fields)-1 {
				lines = append(lines, "")
			}
		}
	case capturePopup:
		lines = append(lines, strings.Split(p.editor.View(), "\n")...)
		count := fmt.Sprintf("%d / %d", utf8.RuneCountInString(p.editor.Value()), maxNoteLength)
		lines = append(lines, "", dimStyle.Render(count))
		if p.discarding {
			lines[len(lines)-1] = errStyle.Render("Discard this note?") + dimStyle.Render("  y discard · n keep writing")
		}
	}
	switch {
	case p.busy:
		lines = append(lines, "", spinner+" "+dimStyle.Render(p.busyText()))
	case p.err != "":
		for _, line := range strings.Split(lipgloss.NewStyle().Width(inner).Render("✗ "+p.err), "\n") {
			lines = append(lines, errStyle.Render(line))
		}
	}
	for i, line := range lines {
		lines[i] = " " + ansi.Truncate(line, inner, "…")
	}
	return frame{number: -1, title: p.title, footer: footer, focused: true}.render(strings.Join(lines, "\n"), w, len(lines)+2)
}

func (p popupState) busyText() string {
	switch p.action.kind {
	case newNoteAction:
		return "Saving…"
	case deleteNoteAction:
		return "Deleting…"
	case moveNoteAction:
		return "Moving…"
	case newProjectAction:
		return "Creating…"
	case archiveProjectAction:
		return "Archiving…"
	case revokeSessionAction, revokeAllAction:
		return "Revoking…"
	case signOutAction:
		return "Signing out…"
	case deleteAccountAction:
		return "Requesting deletion…"
	case serverAction:
		return "Checking the server…"
	}
	return "Working…"
}

// ShortHelp lists the popup's keys for the footer.
func (p popupState) ShortHelp(keys workspaceKeyMap) []key.Binding {
	switch {
	case p.busy:
		return nil
	case p.kind == confirmPopup, p.discarding:
		return []key.Binding{bind("y", p.acceptLabel(), "y"), bind("n/esc", "cancel", "n", "esc")}
	case p.kind == menuPopup:
		return []key.Binding{keySelect, bind("enter", "choose", "enter"), keyCancel}
	case p.kind == formPopup && p.field < len(p.fields)-1:
		return []key.Binding{bind("enter", "next", "enter"), bind("tab", "switch field", "tab"), keyCancel}
	case p.kind == formPopup && len(p.fields) > 1:
		return []key.Binding{bind("enter", p.accept, "enter"), bind("tab", "switch field", "tab"), keyCancel}
	case p.kind == formPopup:
		return []key.Binding{bind("enter", p.accept, "enter"), keyCancel}
	}
	return []key.Binding{bind("ctrl+s", "save", "ctrl+s"), bind("enter", "new line", "enter"), keyCancel}
}

func (p popupState) acceptLabel() string {
	if p.discarding {
		return "discard"
	}
	return p.accept
}

// validateProjectName mirrors the server's rule: 1–120 characters, none of
// them control characters.
func validateProjectName(name string) error {
	if name == "" || utf8.RuneCountInString(name) > 120 || strings.ContainsFunc(name, unicode.IsControl) {
		return errProjectName
	}
	return nil
}

var errProjectName = fmt.Errorf("a project name is 1–120 characters")
