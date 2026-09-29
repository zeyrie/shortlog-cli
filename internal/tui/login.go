package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func isLoginStage(s stage) bool {
	switch s {
	case loginStage, emailStage, codeStage, profileStage, restoreStage, telegramStage:
		return true
	}
	return false
}

func (m Model) updateLoginKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" && m.stage == telegramStage {
		m.clearTelegram()
		m.stage = m.telegramBack
		m.busy = false
		m.message = "Telegram sign-in cancelled."
		if m.stage == emailStage {
			m.focusInput(emailInput)
		}
		return m, nil
	}
	if m.busy {
		return m, nil
	}
	if msg.String() == "esc" {
		switch m.stage {
		case loginStage:
			return m, nil
		case emailStage:
			m.stage = loginStage
			m.inputs[emailInput].Blur()
		case codeStage:
			m.stage = emailStage
			m.challenge = ""
			m.focusInput(emailInput)
		case profileStage:
			if m.telegramLogin {
				m.clearTelegram()
				m.stage = loginStage
			} else {
				m.stage = codeStage
				m.focusInput(codeInput)
			}
		case restoreStage:
			m.clearTelegram()
			m.stage = loginStage
		}
		m.message = ""
		return m, nil
	}
	switch m.stage {
	case loginStage:
		return m.updateLoginMenu(msg)
	case emailStage:
		if msg.String() == "ctrl+t" {
			return m, m.startTelegram(emailStage)
		}
		if msg.String() == "enter" {
			m.clearTelegram()
			address := strings.TrimSpace(m.inputs[emailInput].Value())
			if address == "" || !strings.Contains(address, "@") {
				m.message = "Enter a valid email address."
				return m, nil
			}
			m.email, m.busy, m.message = address, true, ""
			return m, func() tea.Msg {
				id, err := m.api.StartEmail(context.Background(), address)
				return startResult{id, err}
			}
		}
	case codeStage:
		if msg.String() == "enter" {
			code := m.inputs[codeInput].Value()
			if !eightDigits(code) {
				m.message = "Enter the 8-digit code from your email."
				return m, nil
			}
			return m.verify(code, "", "")
		}
	case profileStage:
		if msg.String() == "tab" || msg.String() == "shift+tab" {
			if m.focus == usernameInput {
				m.focusInput(zoneInput)
			} else {
				m.focusInput(usernameInput)
			}
			return m, nil
		}
		if msg.String() == "enter" {
			name := strings.TrimSpace(m.inputs[usernameInput].Value())
			zone := strings.TrimSpace(m.inputs[zoneInput].Value())
			if name == "" || zone == "" {
				m.message = "Both name and IANA time zone are required."
				return m, nil
			}
			if zone == "Local" {
				m.message = "Choose an IANA time zone, not Local."
				return m, nil
			}
			if _, err := time.LoadLocation(zone); err != nil {
				m.message = "Enter a valid IANA time zone (for example, Europe/London)."
				return m, nil
			}
			if m.telegramLogin {
				return m, m.pollTelegram(name, zone)
			}
			return m.verify(m.inputs[codeInput].Value(), name, zone)
		}
	case restoreStage:
		switch strings.ToLower(msg.String()) {
		case "y":
			m.busy, m.message = true, ""
			ticket, telegram := m.ticket, m.telegramLogin
			return m, func() tea.Msg {
				var token string
				var err error
				if telegram {
					token, err = m.api.RestoreTelegram(context.Background(), ticket)
				} else {
					token, err = m.api.RestoreEmail(context.Background(), ticket)
				}
				return restoreResult{token, err}
			}
		case "n":
			m.clearTelegram()
			m.stage = loginStage
			m.message = "Restoration cancelled."
		}
		return m, nil
	case telegramStage:
		switch msg.String() {
		case "r":
			return m, m.pollTelegram("", "")
		case "o":
			id, address, opener := m.telegramAttempt, m.telegramURL, m.openBrowser
			return m, func() tea.Msg { return telegramBrowserResult{id, opener(address)} }
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m *Model) clearTelegram() {
	m.telegramLogin = false
	m.telegramAttempt = ""
	m.telegramSecret = ""
	m.telegramURL = ""
	m.telegramExpires = time.Time{}
	m.telegramGeneration++
	m.ticket = ""
}

func (m *Model) openEmail() {
	m.stage = emailStage
	m.message = ""
	m.focusInput(emailInput)
}

func (m *Model) startTelegram(back stage) tea.Cmd {
	m.clearTelegram()
	m.telegramBack = back
	m.busy = true
	m.message = "Starting Telegram sign-in…"
	return func() tea.Msg {
		start, err := m.api.StartTelegram(context.Background())
		return telegramStartResult{start, err}
	}
}

func (m Model) telegramTimer() tea.Cmd {
	attempt := m.telegramAttempt
	generation := m.telegramGeneration
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return telegramTick{attempt, generation} })
}

func (m *Model) pollTelegram(name, zone string) tea.Cmd {
	if !time.Now().Before(m.telegramExpires) {
		m.message = "Telegram attempt expired. Press Esc then choose 2 to start again."
		return nil
	}
	m.busy = true
	m.telegramGeneration++
	m.message = "Checking Telegram approval…"
	id, secret := m.telegramAttempt, m.telegramSecret
	return func() tea.Msg {
		result, err := m.api.PollTelegram(context.Background(), id, secret, name, zone)
		return telegramPollResult{id, result, err}
	}
}

func (m Model) verify(code, name, zone string) (tea.Model, tea.Cmd) {
	m.busy, m.message = true, ""
	return m, func() tea.Msg {
		result, err := m.api.VerifyEmail(context.Background(), m.challenge, code, name, zone)
		return verifyResult{result, err}
	}
}

func (m *Model) signedIn(token string) tea.Cmd {
	m.token = token
	m.ticket = ""
	m.challenge = ""
	m.inputs[codeInput].SetValue("")
	m.activeProject = nil
	m.projects = nil
	m.projectSelected = 0
	m.showArchived = false
	m.inboxList = noteList{}
	m.inboxStashed = false
	m.inboxNeedsRefresh = false
	m.stage = inboxStage
	m.busy = true
	m.message = "Loading Inbox…"
	return m.loadInbox(token, true)
}

type loginOption struct{ label string }

func (o loginOption) Title() string       { return o.label }
func (o loginOption) Description() string { return "" }
func (o loginOption) FilterValue() string { return o.label }

func newLoginOptions() list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = lipgloss.NewStyle().PaddingLeft(2)
	delegate.Styles.SelectedTitle = titleStyle.Copy().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("6")).PaddingLeft(1)
	items := []list.Item{loginOption{"1  Continue with email"}, loginOption{"2  Continue with Telegram"}}
	options := list.New(items, delegate, 34, 2)
	options.SetFilteringEnabled(false)
	options.SetShowTitle(false)
	options.SetShowHelp(false)
	options.SetShowStatusBar(false)
	options.SetShowPagination(false)
	options.DisableQuitKeybindings()
	return options
}

func (m *Model) resizeLoginOptions() {
	selected := m.loginOptions.Index()
	m.loginOptions.SetSize(min(34, max(1, m.width-2)), 2)
	if m.width < 32 {
		m.loginOptions.SetItems([]list.Item{loginOption{"1  Email"}, loginOption{"2  Telegram"}})
	} else {
		m.loginOptions.SetItems([]list.Item{loginOption{"1  Continue with email"}, loginOption{"2  Continue with Telegram"}})
	}
	m.loginOptions.Select(selected)
}

func (m Model) updateLoginMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "1":
		m.loginOptions.Select(0)
		m.openEmail()
	case "2":
		m.loginOptions.Select(1)
		return m, m.startTelegram(loginStage)
	case "enter":
		if m.loginOptions.Index() == 0 {
			m.openEmail()
		} else {
			return m, m.startTelegram(loginStage)
		}
	case "q":
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		m.loginOptions, cmd = m.loginOptions.Update(msg)
		return m, cmd
	}
	return m, nil
}
