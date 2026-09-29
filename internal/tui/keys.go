package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// bind makes a key binding: the keys it matches, and the hint the footer's
// help line shows for it. Screens match keys with key.Matches against their
// key maps, and list the same bindings in their ShortHelp.
func bind(label, action string, keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(label, action))
}

var (
	keyQuit      = bind("q", "quit", "q")
	keyForceQuit = bind("ctrl+c", "quit", "ctrl+c")
	keySelect    = bind("↑/↓", "select", "up", "down", "k", "j")
	keyBack      = bind("esc", "back", "esc")
	keyCancel    = bind("esc", "cancel", "esc")
	keyTabSwitch = bind("tab", "switch field", "tab", "shift+tab")
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
	case loginStage:
		return m.login.ShortHelp()
	case workspaceStage:
		return m.workspace.ShortHelp()
	}
	return []key.Binding{keyForceQuit}
}
