package tui

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// loginFormValues backs the Huh form fields for the email and Telegram paths.
// Fields bind to these values by pointer, so entries survive failed requests.
type loginFormValues struct {
	email string
	code  string
	name  string
	zone  string
}

func validateEmail(value string) error {
	address := strings.TrimSpace(value)
	if address == "" || !strings.Contains(address, "@") {
		return errors.New("enter a valid email address")
	}
	return nil
}

func validateCode(value string) error {
	if !eightDigits(value) {
		return errors.New("enter the 8-digit code from your email")
	}
	return nil
}

func validateName(value string) error {
	name := strings.TrimSpace(value)
	if name == "" || utf8.RuneCountInString(name) > 80 || strings.ContainsFunc(name, unicode.IsControl) {
		return errors.New("name must be 1–80 characters without control characters")
	}
	return nil
}

func validateZone(value string) error {
	if value == "" || value == "Local" || strings.TrimSpace(value) != value {
		return errors.New("enter an IANA time zone, not Local")
	}
	if _, err := time.LoadLocation(value); err != nil {
		return errors.New("enter a valid IANA time zone, for example Europe/London")
	}
	return nil
}

// loginFormWidth keeps the field close to the text it holds. A field much
// wider than its content would render as a left-heavy void inside the
// centered sign-in layout, so each step gets a width that fits its input.
func loginFormWidth(s stage, width int) int {
	field := 30 // room for a full email address plus the prompt
	switch s {
	case codeStage:
		field = 16 // eight digits plus the prompt
	case profileStage:
		field = 28 // name and "e.g. Europe/London" plus the prompt
	}
	return min(field, max(10, width-10))
}

// showLoginForm rebuilds the Huh form for a sign-in step.
func (m *Model) showLoginForm(s stage) tea.Cmd {
	m.stage = s
	m.inputs[m.focus].Blur()
	theme := huh.ThemeBase()
	theme.Focused.Title = titleStyle
	theme.Focused.ErrorMessage = errStyle
	theme.Focused.Base = theme.Focused.Base.BorderForeground(lipgloss.Color("6"))
	theme.Focused.TextInput.Prompt = menuRailStyle
	theme.Focused.TextInput.Cursor = menuRailStyle
	theme.Focused.TextInput.Placeholder = dimStyle
	var fields []huh.Field
	switch s {
	case emailStage:
		fields = []huh.Field{huh.NewInput().Key("email").Title("Email address").Placeholder("you@example.com").CharLimit(320).Value(&m.loginValues.email).Validate(validateEmail)}
	case codeStage:
		fields = []huh.Field{huh.NewInput().Key("code").Title("8-digit email code").Placeholder("Code from your email").CharLimit(8).EchoMode(huh.EchoModePassword).Value(&m.loginValues.code).Validate(validateCode)}
	case profileStage:
		fields = []huh.Field{
			huh.NewInput().Key("name").Title("Name").Placeholder("Your name").CharLimit(80).Value(&m.loginValues.name).Validate(validateName),
			huh.NewInput().Key("zone").Title("IANA time zone").Placeholder("e.g. Europe/London").CharLimit(64).Value(&m.loginValues.zone).Validate(validateZone),
		}
	default:
		return nil
	}
	m.loginForm = huh.NewForm(huh.NewGroup(fields...)).WithTheme(theme).WithShowHelp(false).WithWidth(loginFormWidth(s, m.width))
	return m.loginForm.Init()
}

func (m Model) loginFieldKeys() []string {
	switch m.stage {
	case emailStage:
		return []string{"email"}
	case codeStage:
		return []string{"code"}
	case profileStage:
		return []string{"name", "zone"}
	}
	return nil
}

// updateLoginForm forwards messages to the Huh form. Huh validates while the
// user types; Enter and Tab advance or submit, Shift+Tab returns a field.
func (m Model) updateLoginForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.loginForm == nil {
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter", "tab":
			form, _ := m.loginForm.Update(msg)
			m.loginForm = form.(*huh.Form)
			if m.loginForm.GetFocusedField().Error() != nil {
				return m, nil // Huh renders the focused field's error.
			}
			if keys := m.loginFieldKeys(); len(keys) > 0 && m.loginForm.GetFocusedField().GetKey() == keys[len(keys)-1] {
				return m.completeLoginForm()
			}
			m.loginForm.NextField()
			return m, nil
		case "shift+tab":
			m.loginForm.PrevField()
			return m, nil
		}
	}
	form, cmd := m.loginForm.Update(msg)
	m.loginForm = form.(*huh.Form)
	return m, cmd
}

// completeLoginForm finishes the form group and dispatches the API request
// for the completed sign-in step.
func (m Model) completeLoginForm() (tea.Model, tea.Cmd) {
	if m.loginForm.State == huh.StateNormal {
		m.loginForm.NextGroup()
	}
	if m.loginForm.State != huh.StateCompleted {
		return m, nil
	}
	return m.dispatchLogin()
}

func (m Model) dispatchLogin() (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	switch m.stage {
	case emailStage:
		m.clearTelegram()
		address := strings.TrimSpace(m.loginValues.email)
		m.email, m.busy = address, true
		m.setStatus(statusInfo, "Sending email code…")
		return m, func() tea.Msg {
			id, err := m.api.StartEmail(context.Background(), address)
			return startResult{id, err}
		}
	case codeStage:
		cmd := m.verify(m.loginValues.code, "", "")
		return m, cmd
	case profileStage:
		name := strings.TrimSpace(m.loginValues.name)
		if m.telegramLogin {
			m.setStatus(statusInfo, "Checking Telegram approval…")
			cmd := m.pollTelegram(name, m.loginValues.zone)
			return m, cmd
		}
		cmd := m.verify(m.loginValues.code, name, m.loginValues.zone)
		return m, cmd
	}
	return m, nil
}
