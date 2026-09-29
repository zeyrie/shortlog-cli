package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
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

// loginKeyMap is the sign-in screen's bindings. Update matches keys with
// these, and ShortHelp picks the ones that apply to the current step.
type loginKeyMap struct {
	ChooseEmail, ChooseTelegram, Continue, Select, Quit key.Binding
	Next, PrevField, UseTelegram, Back                  key.Binding
	Check, Reopen, Copy, Cancel                         key.Binding
	Restore, Decline                                    key.Binding
}

func defaultLoginKeys() loginKeyMap {
	return loginKeyMap{
		ChooseEmail:    bind("1", "email", "1"),
		ChooseTelegram: bind("2", "telegram", "2"),
		Continue:       bind("enter", "continue", "enter"),
		Select:         keySelect,
		Quit:           keyQuit,
		Next:           bind("enter", "next", "enter", "tab"),
		PrevField:      bind("shift+tab", "previous field", "shift+tab"),
		UseTelegram:    bind("ctrl+t", "telegram", "ctrl+t"),
		Back:           keyBack,
		Check:          bind("r", "check now", "r"),
		Reopen:         bind("o", "reopen browser", "o"),
		Copy:           bind("c", "copy link", "c"),
		Cancel:         keyCancel,
		Restore:        bind("y", "restore", "y", "Y"),
		Decline:        bind("n", "cancel", "n", "N"),
	}
}

// ShortHelp lists the footer hints for the current step, most important first.
func (l loginModel) ShortHelp() []key.Binding {
	k := l.keys
	switch l.step {
	case menuStep:
		return []key.Binding{k.Continue, bind("1/2", "choose", "1", "2"), k.Select, k.Quit}
	case emailStep:
		return []key.Binding{bind("enter", "send code", "enter"), k.UseTelegram, bind("esc", "sign-in options", "esc"), keyForceQuit}
	case codeStep:
		return []key.Binding{bind("enter", "verify", "enter"), bind("esc", "change email", "esc"), keyForceQuit}
	case profileStep:
		return []key.Binding{bind("enter", "next/save", "enter"), keyTabSwitch, k.Back}
	case telegramStep:
		return []key.Binding{k.Check, k.Reopen, k.Copy, k.Cancel}
	case restoreStep:
		return []key.Binding{k.Restore, bind("n", "keep deletion", "n"), bind("esc", "sign-in options", "esc")}
	}
	return nil
}

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
		return m.login.ShortHelp()
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
