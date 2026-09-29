package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"shortlog-cli/internal/api"
)

// loginAPI is the part of the Shortlog API the sign-in screen uses.
type loginAPI interface {
	StartEmail(context.Context, string) (string, error)
	VerifyEmail(context.Context, string, string, string, string) (api.VerifyResult, error)
	RestoreEmail(context.Context, string) (string, error)
	StartTelegram(context.Context) (api.TelegramStart, error)
	PollTelegram(context.Context, string, string, string, string) (api.VerifyResult, error)
	RestoreTelegram(context.Context, string) (string, error)
}

// loginStep is the sign-in screen's current step.
type loginStep int

const (
	menuStep loginStep = iota
	emailStep
	codeStep
	profileStep
	telegramStep
	restoreStep
)

// statusNote is a status-line change the sign-in screen asks the root model to
// make. An empty text clears the line.
type statusNote struct {
	level statusLevel
	text  string
}

// loginModel is the sign-in screen: provider choice, the email code and
// profile forms, Telegram approval, and account restoration. It reports back
// to the root model through state the root reads after each update, the way
// a parent reads a Huh form's State: a pending status note, and the session
// token once sign-in succeeds.
type loginModel struct {
	api         loginAPI
	openBrowser func(string) error
	keys        loginKeyMap
	step        loginStep
	busy        bool
	width       int // the space above the footer
	height      int

	options list.Model
	form    *huh.Form
	values  *loginFormValues

	email     string
	challenge string
	ticket    string // recovery ticket; never rendered

	telegramLogin         bool
	telegramBrowserFailed bool
	telegramAttempt       string
	telegramSecret        string // never rendered
	telegramURL           string
	telegramExpires       time.Time
	telegramGeneration    uint64
	telegramBack          loginStep

	note  *statusNote
	token string // set when sign-in succeeds; the root takes it and resets the screen
}

func newLogin(client loginAPI, openBrowser func(string) error) loginModel {
	l := loginModel{api: client, openBrowser: openBrowser, keys: defaultLoginKeys(), values: &loginFormValues{}, options: newLoginOptions()}
	if zone := time.Now().Location().String(); zone != "Local" {
		l.values.zone = zone
	}
	return l
}

// reset returns to provider choice with no attempt in progress, keeping only
// the screen size and the highlighted provider.
func (l loginModel) reset() loginModel {
	fresh := newLogin(l.api, l.openBrowser)
	fresh.options.Select(l.options.Index())
	fresh.setSize(l.width, l.height)
	return fresh
}

func (l *loginModel) setStatus(level statusLevel, text string) {
	l.note = &statusNote{level: level, text: text}
}

func (l *loginModel) clearStatus() { l.note = &statusNote{} }

// setSize fits the screen to the space above the footer.
func (l *loginModel) setSize(width, height int) {
	l.width, l.height = width, height
	selected := l.options.Index()
	l.options.SetSize(min(34, max(1, width-2)), 2)
	if width < 32 {
		l.options.SetItems([]list.Item{loginOption{"1  Email"}, loginOption{"2  Telegram"}})
	} else {
		l.options.SetItems([]list.Item{loginOption{"1  Continue with email"}, loginOption{"2  Continue with Telegram"}})
	}
	l.options.Select(selected)
	l.resizeForm()
}

func (l loginModel) Update(msg tea.Msg) (loginModel, tea.Cmd) {
	switch msg := msg.(type) {
	case startResult:
		return l.onEmailStart(msg)
	case verifyResult:
		return l.onEmailVerify(msg)
	case restoreResult:
		return l.onRestore(msg)
	case telegramStartResult:
		return l.onTelegramStart(msg)
	case telegramBrowserResult:
		return l.onTelegramBrowser(msg)
	case telegramTick:
		return l.onTelegramTick(msg)
	case telegramPollResult:
		return l.onTelegramPoll(msg)
	case tea.KeyMsg:
		return l.updateKey(msg)
	}
	if !l.busy && l.hasForm() {
		return l.updateForm(msg)
	}
	return l, nil
}

func (l loginModel) hasForm() bool {
	return l.form != nil && (l.step == emailStep || l.step == codeStep || l.step == profileStep)
}

func (l loginModel) updateKey(msg tea.KeyMsg) (loginModel, tea.Cmd) {
	if l.step == telegramStep && key.Matches(msg, l.keys.Cancel) {
		l.clearTelegram()
		l.step, l.busy = l.telegramBack, false
		l.setStatus(statusInfo, "Telegram sign-in cancelled.")
		return l, nil
	}
	if l.busy {
		return l, nil
	}
	if key.Matches(msg, l.keys.Back) && l.step != menuStep {
		return l.back()
	}
	switch l.step {
	case menuStep:
		return l.updateMenu(msg)
	case emailStep:
		if key.Matches(msg, l.keys.UseTelegram) {
			return l, l.startTelegram(emailStep)
		}
	case restoreStep:
		switch {
		case key.Matches(msg, l.keys.Restore):
			return l, l.restore()
		case key.Matches(msg, l.keys.Decline):
			l.clearTelegram()
			l.step = menuStep
			l.setStatus(statusInfo, "Restoration cancelled.")
		}
		return l, nil
	case telegramStep:
		switch {
		case key.Matches(msg, l.keys.Check):
			return l, l.pollTelegram("", "")
		case key.Matches(msg, l.keys.Reopen):
			id, address, opener := l.telegramAttempt, l.telegramURL, l.openBrowser
			return l, func() tea.Msg { return telegramBrowserResult{id, opener(address)} }
		}
		return l, nil
	}
	if l.hasForm() {
		return l.updateForm(msg)
	}
	return l, nil
}

// back steps to the previous sign-in step, or to provider choice.
func (l loginModel) back() (loginModel, tea.Cmd) {
	l.clearStatus()
	switch l.step {
	case codeStep:
		l.challenge = ""
		return l, l.showForm(emailStep)
	case profileStep:
		if !l.telegramLogin {
			return l, l.showForm(codeStep)
		}
		l.clearTelegram()
	case restoreStep:
		l.clearTelegram()
	}
	l.step, l.form = menuStep, nil
	return l, nil
}

func (l loginModel) updateMenu(msg tea.KeyMsg) (loginModel, tea.Cmd) {
	switch {
	case key.Matches(msg, l.keys.ChooseEmail):
		l.options.Select(0)
		return l, l.openEmail()
	case key.Matches(msg, l.keys.ChooseTelegram):
		l.options.Select(1)
		return l, l.startTelegram(menuStep)
	case key.Matches(msg, l.keys.Continue):
		if l.options.Index() == 0 {
			return l, l.openEmail()
		}
		return l, l.startTelegram(menuStep)
	case key.Matches(msg, l.keys.Quit):
		return l, tea.Quit
	}
	var cmd tea.Cmd
	l.options, cmd = l.options.Update(msg)
	return l, cmd
}

func (l *loginModel) openEmail() tea.Cmd {
	l.clearStatus()
	return l.showForm(emailStep)
}

func (l *loginModel) restore() tea.Cmd {
	l.busy = true
	l.setStatus(statusInfo, "Restoring account…")
	ticket, telegram, client := l.ticket, l.telegramLogin, l.api
	return func() tea.Msg {
		var token string
		var err error
		if telegram {
			token, err = client.RestoreTelegram(context.Background(), ticket)
		} else {
			token, err = client.RestoreEmail(context.Background(), ticket)
		}
		return restoreResult{token, err}
	}
}

// verify dispatches an email code check, with the profile for a new account.
func (l *loginModel) verify(code, name, zone string) tea.Cmd {
	l.busy = true
	l.setStatus(statusInfo, "Verifying sign-in…")
	challenge, client := l.challenge, l.api
	return func() tea.Msg {
		result, err := client.VerifyEmail(context.Background(), challenge, code, name, zone)
		return verifyResult{result, err}
	}
}

// signedIn hands the token to the root model, which leaves the screen.
func (l *loginModel) signedIn(token string) {
	l.clearTelegram()
	l.token = token
	l.challenge = ""
	l.values.code = ""
	l.form = nil
	l.busy = false
}

func (l *loginModel) clearTelegram() {
	l.telegramLogin = false
	l.telegramAttempt = ""
	l.telegramSecret = ""
	l.telegramURL = ""
	l.telegramExpires = time.Time{}
	l.telegramBrowserFailed = false
	l.telegramGeneration++
	l.ticket = ""
}

func (l *loginModel) startTelegram(back loginStep) tea.Cmd {
	l.clearTelegram()
	l.telegramBack = back
	l.busy = true
	l.setStatus(statusInfo, "Starting Telegram sign-in…")
	client := l.api
	return func() tea.Msg {
		start, err := client.StartTelegram(context.Background())
		return telegramStartResult{start, err}
	}
}

func (l loginModel) telegramTimer() tea.Cmd {
	attempt, generation := l.telegramAttempt, l.telegramGeneration
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return telegramTick{attempt, generation} })
}

func (l *loginModel) pollTelegram(name, zone string) tea.Cmd {
	if !time.Now().Before(l.telegramExpires) {
		l.setStatus(statusError, "Telegram attempt expired. Esc, then 2 to start again.")
		return nil
	}
	l.busy = true
	l.telegramGeneration++
	id, secret, client := l.telegramAttempt, l.telegramSecret, l.api
	return func() tea.Msg {
		result, err := client.PollTelegram(context.Background(), id, secret, name, zone)
		return telegramPollResult{id, result, err}
	}
}

// cardLoading reports whether the step's card shows its own progress, so the
// footer need not repeat it. Telegram approval always shows its waiting line.
func (l loginModel) cardLoading() bool {
	switch l.step {
	case telegramStep:
		return true
	case emailStep, codeStep, profileStep:
		return l.busy && l.form != nil
	case restoreStep:
		return l.busy
	}
	return false
}

type loginOption struct{ label string }

func (o loginOption) Title() string       { return o.label }
func (o loginOption) Description() string { return "" }
func (o loginOption) FilterValue() string { return o.label }

func newLoginOptions() list.Model {
	// The list tracks the selection and its keys; loginModel.menu draws it.
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
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
