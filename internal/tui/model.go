package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"shortlog-cli/internal/api"
)

type emailAPI interface {
	StartEmail(context.Context, string) (string, error)
	VerifyEmail(context.Context, string, string, string, string) (api.VerifyResult, error)
	RestoreEmail(context.Context, string) (string, error)
}

type stage int

const (
	emailStage stage = iota
	codeStage
	profileStage
	restoreStage
	signedInStage
)

type Model struct {
	api       emailAPI
	stage     stage
	inputs    [4]textinput.Model
	focus     int
	challenge string
	ticket    string
	token     string // Memory only; never rendered or logged.
	email     string
	busy      bool
	message   string
	width     int
}

const (
	emailInput = iota
	codeInput
	usernameInput
	zoneInput
)

func New(client emailAPI) Model {
	m := Model{api: client, width: 80}
	for i := range m.inputs {
		m.inputs[i] = textinput.New()
		m.inputs[i].CharLimit = 320
		m.inputs[i].Width = 42
	}
	m.inputs[emailInput].Placeholder = "you@example.com"
	m.inputs[emailInput].CharLimit = 320
	m.inputs[codeInput].Placeholder = "8-digit code"
	m.inputs[codeInput].CharLimit = 8
	m.inputs[codeInput].EchoMode = textinput.EchoPassword
	m.inputs[codeInput].EchoCharacter = '•'
	m.inputs[usernameInput].Placeholder = "Your name"
	m.inputs[usernameInput].CharLimit = 80
	m.inputs[zoneInput].Placeholder = "e.g. Europe/London"
	m.inputs[zoneInput].CharLimit = 64
	zone := time.Now().Location().String()
	if zone != "Local" {
		m.inputs[zoneInput].SetValue(zone)
	}
	m.focusInput(emailInput)
	return m
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

type startResult struct {
	id  string
	err error
}
type verifyResult struct {
	result api.VerifyResult
	err    error
}
type restoreResult struct {
	token string
	err   error
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case startResult:
		m.busy = false
		if msg.err != nil {
			m.message = friendlyError(msg.err, "Could not send a code.")
		} else {
			m.challenge = msg.id
			m.stage = codeStage
			m.inputs[codeInput].SetValue("")
			m.focusInput(codeInput)
			m.message = "Check your email. The code expires in 10 minutes."
		}
		return m, nil
	case verifyResult:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Code == "profile_required" {
				m.stage = profileStage
				m.focusInput(usernameInput)
				m.message = "New account: enter a name and IANA time zone, then verify again."
			} else {
				m.message = friendlyError(msg.err, "Could not verify the code.")
			}
			return m, nil
		}
		if msg.result.Status == "restore_required" {
			m.stage = restoreStage
			m.ticket = msg.result.RecoveryTicket
			m.inputs[codeInput].SetValue("")
			m.message = ""
		} else {
			m.signedIn(msg.result.Token)
		}
		return m, nil
	case restoreResult:
		m.busy = false
		if msg.err != nil {
			m.message = friendlyError(msg.err, "Could not restore the account.")
		} else {
			m.signedIn(msg.token)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.busy {
			return m, nil
		}
		if msg.String() == "esc" {
			switch m.stage {
			case codeStage:
				m.stage = emailStage
				m.challenge = ""
				m.focusInput(emailInput)
			case profileStage:
				m.stage = codeStage
				m.focusInput(codeInput)
			case restoreStage:
				m.ticket = ""
				m.stage = emailStage
				m.focusInput(emailInput)
			case signedInStage:
				return m, tea.Quit
			}
			m.message = ""
			return m, nil
		}
		switch m.stage {
		case emailStage:
			if msg.String() == "enter" {
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
				if _, err := time.LoadLocation(zone); err != nil {
					m.message = "Enter a valid IANA time zone (for example, Europe/London)."
					return m, nil
				}
				return m.verify(m.inputs[codeInput].Value(), name, zone)
			}
		case restoreStage:
			switch strings.ToLower(msg.String()) {
			case "y":
				m.busy, m.message = true, ""
				ticket := m.ticket
				return m, func() tea.Msg {
					token, err := m.api.RestoreEmail(context.Background(), ticket)
					return restoreResult{token, err}
				}
			case "n":
				m.ticket = ""
				m.stage = emailStage
				m.focusInput(emailInput)
				m.message = "Restoration cancelled."
			}
			return m, nil
		case signedInStage:
			if msg.String() == "q" || msg.String() == "enter" {
				return m, tea.Quit
			}
			return m, nil
		}
	}
	if m.stage <= profileStage && !m.busy {
		var cmd tea.Cmd
		m.inputs[m.focus], cmd = m.inputs[m.focus].Update(message)
		return m, cmd
	}
	return m, nil
}

func (m *Model) focusInput(index int) {
	m.inputs[m.focus].Blur()
	m.focus = index
	m.inputs[index].Focus()
}

func (m Model) verify(code, name, zone string) (tea.Model, tea.Cmd) {
	m.busy, m.message = true, ""
	return m, func() tea.Msg {
		result, err := m.api.VerifyEmail(context.Background(), m.challenge, code, name, zone)
		return verifyResult{result, err}
	}
}

func (m *Model) signedIn(token string) {
	m.token = token
	m.ticket = ""
	m.challenge = ""
	m.inputs[codeInput].SetValue("")
	m.stage = signedInStage
	m.message = "Signed in. Notes and projects are coming next; this session is not saved yet."
}

func eightDigits(code string) bool {
	if len(code) != 8 {
		return false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func friendlyError(err error, fallback string) string {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "rate_limited":
			return "Too many attempts. Wait before trying again."
		case "service_unavailable":
			return "Email sign-in is temporarily unavailable."
		case "invalid_request":
			return "Invalid or expired code or details. Check them and try again."
		}
		if apiErr.Status == http.StatusUnprocessableEntity {
			return "Check your account details and try again."
		}
	}
	return fallback + " Check your connection and try again."
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func (m Model) View() string {
	var body string
	switch m.stage {
	case emailStage:
		body = "Sign in with email\n\nEmail\n" + m.inputs[emailInput].View() + "\n\n" + dimStyle.Render("Enter send code · Ctrl+C quit")
	case codeStage:
		body = fmt.Sprintf("Code sent to %s\n\n8-digit code\n%s\n\n%s", m.email, m.inputs[codeInput].View(), dimStyle.Render("Enter verify · Esc change email · Ctrl+C quit"))
	case profileStage:
		body = "Finish creating your account\n\nName\n" + m.inputs[usernameInput].View() + "\n\nTime zone (IANA)\n" + m.inputs[zoneInput].View() + "\n\n" + dimStyle.Render("Tab switch field · Enter verify · Esc back")
	case restoreStage:
		body = "This account is scheduled for deletion.\nRestoring it keeps its projects and notes, but previously signed-in devices remain signed out.\n\nRestore this account? [y/N]"
	case signedInStage:
		body = "Email sign-in complete.\n\n" + dimStyle.Render("Enter or q to quit")
	}
	if m.busy {
		body += "\n\nWorking…"
	}
	if m.message != "" {
		body += "\n\n" + errStyle.Render(m.message)
	}
	content := titleStyle.Render("Shortlog") + "\n\n" + body
	width := m.width - 4
	if width < 20 {
		width = 20
	}
	if width > 68 {
		width = 68
	}
	return lipgloss.NewStyle().Width(width).Padding(1, 2).Render(content)
}
