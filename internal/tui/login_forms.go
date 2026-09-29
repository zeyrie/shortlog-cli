package tui

import (
	"context"
	"errors"
	"net/mail"
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

// validateEmail catches obvious typos before a round trip; the server has the
// final say. A bare address is required, so "Name <a@b>" is rejected too.
func validateEmail(value string) error {
	address := strings.TrimSpace(value)
	parsed, err := mail.ParseAddress(address)
	if address == "" || err != nil || parsed.Address != address {
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
		return errors.New("use 1–80 characters, no control codes")
	}
	return nil
}

func validateZone(value string) error {
	if value == "" || value == "Local" || strings.TrimSpace(value) != value {
		return errors.New("use an IANA zone, not Local")
	}
	if _, err := time.LoadLocation(value); err != nil {
		return errors.New("unknown zone; try Europe/London")
	}
	return nil
}

// loginBoxed reports whether the sign-in form sits in a rounded card. The card
// costs two columns and two rows, so tight terminals fall back to Huh's bar.
func (m Model) loginBoxed() bool { return m.width >= 28 && m.bodyHeight() >= 12 }

// loginFieldWidth is the outer width of a sign-in field: fixed, so the field
// neither grows nor shifts while the user types, and the same on every step,
// so the box stays put between steps. Long entries scroll inside it.
func loginFieldWidth(termWidth int, boxed bool) int {
	width := min(42, max(12, termWidth-4))
	if !boxed {
		width = min(40, max(10, termWidth-2))
	}
	return width
}

// loginFormWidth converts the outer field width into Huh's width, which
// excludes the card's border and padding.
func loginFormWidth(termWidth int, boxed bool) int {
	if boxed {
		return loginFieldWidth(termWidth, boxed) - 4
	}
	return loginFieldWidth(termWidth, boxed)
}

func loginTheme(boxed bool) *huh.Theme {
	theme := huh.ThemeBase()
	theme.Focused.Title = titleStyle
	theme.Focused.ErrorMessage = errStyle
	theme.Focused.ErrorIndicator = errStyle
	theme.Focused.TextInput.Prompt = menuRailStyle
	theme.Focused.TextInput.Cursor = menuRailStyle
	theme.Focused.TextInput.Placeholder = dimStyle
	theme.Blurred.TextInput.Placeholder = dimStyle
	theme.Blurred.Title = dimStyle
	theme.Blurred.TextInput.Prompt = dimStyle
	if boxed {
		// The card is drawn around the whole form by formBlock. Huh sizes its
		// group viewport for unframed fields, so a border on each field would
		// be clipped; the fields themselves stay plain.
		theme.Focused.Base = lipgloss.NewStyle()
		theme.Blurred.Base = lipgloss.NewStyle()
	} else {
		theme.Focused.Base = theme.Focused.Base.BorderForeground(lipgloss.Color("6"))
	}
	return theme
}

// resizeLoginForm refits the open form after a terminal resize, switching
// between the boxed and compact styles when the size crosses the threshold.
func (m *Model) resizeLoginForm() {
	if m.loginForm != nil {
		boxed := m.loginBoxed()
		m.loginForm.WithTheme(loginTheme(boxed)).WithWidth(loginFormWidth(m.width, boxed))
	}
}

// showLoginForm rebuilds the Huh form for a sign-in step.
func (m *Model) showLoginForm(s stage) tea.Cmd {
	m.stage = s
	m.inputs[m.focus].Blur()
	boxed := m.loginBoxed()
	var fields []huh.Field
	switch s {
	case emailStage:
		fields = []huh.Field{huh.NewInput().Key("email").Title("Email address").Placeholder("you@example.com").CharLimit(320).Value(&m.loginValues.email).Validate(validateEmail)}
	case codeStage:
		fields = []huh.Field{huh.NewInput().Key("code").Title("8-digit email code").Placeholder("12345678").CharLimit(8).Value(&m.loginValues.code).Validate(validateCode)}
	case profileStage:
		fields = []huh.Field{
			huh.NewInput().Key("name").Title("Name").Placeholder("Your name").CharLimit(80).Value(&m.loginValues.name).Validate(validateName),
			huh.NewInput().Key("zone").Title("IANA time zone").Placeholder("e.g. Europe/London").CharLimit(64).Value(&m.loginValues.zone).Validate(validateZone),
		}
	default:
		return nil
	}
	m.loginForm = huh.NewForm(huh.NewGroup(fields...)).WithTheme(loginTheme(boxed)).WithShowHelp(false).WithWidth(loginFormWidth(m.width, boxed))
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
			// Huh answers Enter with its own advance command; drop it and
			// advance here, so validation and submission stay in one place.
			form, _ := m.loginForm.Update(msg)
			m.loginForm = form.(*huh.Form)
			if m.loginForm.GetFocusedField().Error() != nil {
				return m, nil // Huh renders the focused field's error.
			}
			if keys := m.loginFieldKeys(); len(keys) > 0 && m.loginForm.GetFocusedField().GetKey() == keys[len(keys)-1] {
				return m.submitLoginForm()
			}
			return m, m.loginForm.NextField() // focus command: starts the cursor blink
		case "shift+tab":
			return m, m.loginForm.PrevField()
		}
	}
	form, cmd := m.loginForm.Update(msg)
	m.loginForm = form.(*huh.Form)
	return m, cmd
}

// submitLoginForm checks every value of the step and dispatches its request.
// The Huh form is deliberately never completed: a completed form renders
// nothing and ignores input, which would blank the card while the request
// runs and leave the entry uneditable if it fails.
func (m Model) submitLoginForm() (tea.Model, tea.Cmd) {
	var err error
	switch m.stage {
	case emailStage:
		err = validateEmail(m.loginValues.email)
	case codeStage:
		err = validateCode(m.loginValues.code)
	case profileStage:
		err = errors.Join(validateName(m.loginValues.name), validateZone(m.loginValues.zone))
	}
	if err != nil {
		// Only reachable when an earlier field was edited after leaving it;
		// Huh shows the focused field's own error inline.
		m.setStatus(statusError, "Check the form: "+err.Error()+".")
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
