package tui

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
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

// boxed reports whether sign-in steps sit in a rounded card. The card costs
// two columns and two rows, so tight terminals fall back to Huh's left bar.
func (l loginModel) boxed() bool { return l.width >= 28 && l.height >= 12 }

// loginFieldWidth is the outer width of a sign-in card: fixed, so a field
// neither grows nor shifts while the user types, and the same on every step,
// so the card stays put between steps. Long entries scroll inside it.
func loginFieldWidth(termWidth int, boxed bool) int {
	width := min(42, max(12, termWidth-4))
	if !boxed {
		width = min(40, max(10, termWidth-2))
	}
	return width
}

// loginFormWidth converts the outer card width into the width of what goes
// inside it, which excludes the card's border and padding.
func loginFormWidth(termWidth int, boxed bool) int {
	if boxed {
		return loginFieldWidth(termWidth, boxed) - 4
	}
	return loginFieldWidth(termWidth, boxed)
}

// loginTheme styles the sign-in fields. Huh calls it with whether the
// terminal background is dark, once the form learns it.
func loginTheme(boxed bool) huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		theme := huh.ThemeBase(isDark)
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
			// The card is drawn around the whole form by the view. Huh sizes
			// its group viewport for unframed fields, so a border on each
			// field would be clipped; the fields themselves stay plain.
			theme.Focused.Base = lipgloss.NewStyle()
			theme.Blurred.Base = lipgloss.NewStyle()
		} else {
			theme.Focused.Base = theme.Focused.Base.BorderForeground(accentColor)
		}
		return theme
	})
}

// resizeForm refits the open form after a resize, switching between the
// boxed and compact styles when the size crosses the threshold.
func (l *loginModel) resizeForm() {
	if l.form != nil {
		boxed := l.boxed()
		l.form.WithTheme(loginTheme(boxed)).WithWidth(loginFormWidth(l.width, boxed))
	}
}

// showForm moves to a form step and builds its Huh form. Fields bind to
// l.values, so entries survive going back a step or a failed request.
func (l *loginModel) showForm(step loginStep) tea.Cmd {
	l.step = step
	boxed := l.boxed()
	var fields []huh.Field
	switch step {
	case emailStep:
		fields = []huh.Field{huh.NewInput().Key("email").Title("Email address").Placeholder("you@example.com").CharLimit(320).Value(&l.values.email).Validate(validateEmail)}
	case codeStep:
		fields = []huh.Field{huh.NewInput().Key("code").Title("8-digit email code").Placeholder("12345678").CharLimit(8).Value(&l.values.code).Validate(validateCode)}
	case profileStep:
		fields = []huh.Field{
			huh.NewInput().Key("name").Title("Name").Placeholder("Your name").CharLimit(80).Value(&l.values.name).Validate(validateName),
			huh.NewInput().Key("zone").Title("IANA time zone").Placeholder("e.g. Europe/London").CharLimit(64).Value(&l.values.zone).Validate(validateZone),
		}
	default:
		l.form = nil
		return nil
	}
	l.form = huh.NewForm(huh.NewGroup(fields...)).WithTheme(loginTheme(boxed)).WithShowHelp(false).WithWidth(loginFormWidth(l.width, boxed))
	init := l.form.Init()
	if l.background != nil {
		l.form.Update(*l.background) // a new form has not seen the terminal's reply
	}
	return init
}

func (l loginModel) fieldKeys() []string {
	switch l.step {
	case emailStep:
		return []string{"email"}
	case codeStep:
		return []string{"code"}
	case profileStep:
		return []string{"name", "zone"}
	}
	return nil
}

// updateForm forwards messages to the Huh form. Huh validates while the user
// types; Enter and Tab advance or submit, Shift+Tab returns a field.
func (l loginModel) updateForm(msg tea.Msg) (loginModel, tea.Cmd) {
	if l.form == nil {
		return l, nil
	}
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, l.keys.Next):
			// Huh answers Enter with its own advance command; drop it and
			// advance here, so validation and submission stay in one place.
			form, _ := l.form.Update(msg)
			l.form = form.(*huh.Form)
			if l.form.GetFocusedField().Error() != nil {
				return l, nil // Huh renders the focused field's error.
			}
			if keys := l.fieldKeys(); len(keys) > 0 && l.form.GetFocusedField().GetKey() == keys[len(keys)-1] {
				return l.submit()
			}
			return l, l.form.NextField() // focus command: starts the cursor blink
		case key.Matches(msg, l.keys.PrevField):
			return l, l.form.PrevField()
		}
	}
	form, cmd := l.form.Update(msg)
	l.form = form.(*huh.Form)
	return l, cmd
}

// submit checks every value of the step and dispatches its request. The Huh
// form is deliberately never completed: a completed form renders nothing and
// ignores input, which would blank the card while the request runs and leave
// the entry uneditable if it fails.
func (l loginModel) submit() (loginModel, tea.Cmd) {
	if l.busy {
		return l, nil
	}
	var err error
	switch l.step {
	case emailStep:
		err = validateEmail(l.values.email)
	case codeStep:
		err = validateCode(l.values.code)
	case profileStep:
		err = errors.Join(validateName(l.values.name), validateZone(l.values.zone))
	}
	if err != nil {
		// Only reachable when an earlier field was edited after leaving it;
		// Huh shows the focused field's own error inline.
		l.setStatus(statusError, "Check the form: "+err.Error()+".")
		return l, nil
	}
	switch l.step {
	case emailStep:
		l.clearTelegram()
		address := strings.TrimSpace(l.values.email)
		l.email, l.busy = address, true
		l.setStatus(statusInfo, "Sending email code…")
		client := l.api
		return l, func() tea.Msg {
			id, err := client.StartEmail(context.Background(), address)
			return startResult{id, err}
		}
	case codeStep:
		return l, l.verify(l.values.code, "", "")
	case profileStep:
		name := strings.TrimSpace(l.values.name)
		if l.telegramLogin {
			return l, l.pollTelegram(name, l.values.zone)
		}
		return l, l.verify(l.values.code, name, l.values.zone)
	}
	return l, nil
}
