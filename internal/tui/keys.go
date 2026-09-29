package tui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

// Footer shortcuts. Each binding pairs the keys with the hint the footer's
// help line shows. Update still matches key strings; as screens become
// components they switch to key.Matches on these same bindings.
func bind(label, action string, keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(label, action))
}

// noKey is the "n or Esc" answer shared by every confirmation prompt.
func noKey(action string) key.Binding { return bind("n/esc", action, "n", "esc") }

var (
	keyQuit       = bind("q", "quit", "q")
	keyForceQuit  = bind("ctrl+c", "quit", "ctrl+c")
	keySelect     = bind("↑/↓", "select", "up", "down", "k", "j")
	keyScroll     = bind("↑/↓", "scroll", "up", "down", "k", "j")
	keyRefresh    = bind("r", "refresh", "r")
	keyBack       = bind("esc", "back", "esc")
	keyCancel     = bind("esc", "cancel", "esc")
	keySessions   = bind("s", "sessions", "s")
	keyAccount    = bind("g", "account", "g")
	keyProjects   = bind("p", "projects", "p")
	keyEditNote   = bind("e", "edit", "e")
	keyMoveNote   = bind("v", "move", "v")
	keyDeleteNote = bind("d", "delete", "d")
	keyOlder      = bind("m", "older", "m")
	keyTabSwitch  = bind("tab", "switch field", "tab", "shift+tab")
)

func newHelp() help.Model {
	h := help.New()
	h.Styles.ShortKey = helpKeyStyle
	h.Styles.ShortDesc = dimStyle
	h.Styles.ShortSeparator = dimStyle
	h.Styles.Ellipsis = dimStyle
	return h
}

// shortcuts lists the footer hints for the current screen, most important
// first: the help line drops trailing hints when the terminal is narrow.
func (m Model) shortcuts() []key.Binding {
	switch m.stage {
	case startupStage:
		return []key.Binding{keyForceQuit}
	case loginStage:
		return []key.Binding{bind("enter", "continue", "enter"), bind("1/2", "choose", "1", "2"), keySelect, keyQuit}
	case emailStage:
		return []key.Binding{bind("enter", "send code", "enter"), bind("ctrl+t", "telegram", "ctrl+t"), bind("esc", "sign-in options", "esc"), keyForceQuit}
	case codeStage:
		return []key.Binding{bind("enter", "verify", "enter"), bind("esc", "change email", "esc"), keyForceQuit}
	case profileStage:
		return []key.Binding{bind("enter", "next/save", "enter"), keyTabSwitch, keyBack}
	case telegramStage:
		return []key.Binding{bind("r", "check now", "r"), bind("o", "reopen browser", "o"), keyCancel}
	case restoreStage:
		return []key.Binding{bind("y", "restore", "y"), noKey("cancel")}
	case inboxStage:
		if m.archivedProject() {
			return []key.Binding{bind("enter", "read", "enter"), keySelect, keyOlder, keyRefresh, keyProjects, keySessions, keyAccount, keyBack, keyQuit}
		}
		return []key.Binding{bind("enter", "read", "enter"), bind("n", "new", "n"), keyEditNote, keyMoveNote, keyDeleteNote, keySelect, keyProjects, keyRefresh, keyOlder, keySessions, keyAccount, keyQuit}
	case readingStage:
		if m.archivedProject() {
			return []key.Binding{keyScroll, keyBack, keyQuit}
		}
		return []key.Binding{keyEditNote, keyMoveNote, keyDeleteNote, keyScroll, keyBack, keyQuit}
	case captureStage:
		return []key.Binding{bind("ctrl+s", "save", "ctrl+s"), bind("enter", "new line", "enter"), keyBack}
	case discardStage, accountDiscardStage:
		return []key.Binding{bind("y", "discard", "y"), noKey("keep editing")}
	case deleteStage:
		return []key.Binding{bind("y", "delete permanently", "y"), noKey("cancel")}
	case projectsStage:
		if m.showArchived {
			return []key.Binding{bind("enter", "read-only", "enter"), bind("u", "unarchive", "u"), bind("t", "active", "t"), keySelect, bind("i", "inbox", "i"), keySessions, keyAccount, keyRefresh, bind("esc", "inbox", "esc"), keyQuit}
		}
		return []key.Binding{bind("enter", "open", "enter"), bind("a", "new", "a"), bind("x", "archive", "x"), bind("t", "archived", "t"), keySelect, bind("i", "inbox", "i"), keySessions, keyAccount, keyRefresh, bind("esc", "inbox", "esc"), keyQuit}
	case newProjectStage:
		return []key.Binding{bind("enter", "create", "enter"), keyCancel}
	case moveStage:
		return []key.Binding{bind("enter", "move", "enter"), keySelect, keyRefresh, keyCancel}
	case archiveStage:
		return []key.Binding{bind("y", "archive", "y"), noKey("cancel")}
	case accountStage:
		return []key.Binding{bind("e", "edit profile", "e"), bind("d", "request deletion", "d"), keyRefresh, keyBack}
	case accountEditStage:
		return []key.Binding{bind("enter", "save", "enter"), keyTabSwitch, keyBack}
	case accountDeleteStage:
		return []key.Binding{bind("enter", "request deletion", "enter"), keyCancel}
	case sessionsStage:
		return []key.Binding{bind("x", "revoke", "x"), bind("a", "revoke all", "a"), bind("l", "log out here", "l"), keySelect, keyRefresh, keyBack}
	case sessionConfirmStage:
		return []key.Binding{bind("y", "confirm", "y"), noKey("cancel")}
	}
	return nil
}
