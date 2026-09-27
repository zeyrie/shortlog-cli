package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"shortlog-cli/internal/api"
)

type emailAPI interface {
	StartEmail(context.Context, string) (string, error)
	VerifyEmail(context.Context, string, string, string, string) (api.VerifyResult, error)
	RestoreEmail(context.Context, string) (string, error)
	Me(context.Context, string) (api.Account, error)
	Inbox(context.Context, string) (api.NotesPage, error)
	CreateInboxNote(context.Context, string, string) (api.Note, error)
}

type sessionStore interface {
	Load() (string, error)
	Save(string) error
	Delete() error
}

type stage int

const (
	startupStage stage = iota
	emailStage
	codeStage
	profileStage
	restoreStage
	inboxStage
	readingStage
	captureStage
)

type Model struct {
	api         emailAPI
	store       sessionStore
	stage       stage
	inputs      [4]textinput.Model
	focus       int
	challenge   string
	ticket      string
	token       string // Never rendered or logged.
	email       string
	username    string
	notes       []api.Note
	more        bool
	selected    int
	reader      viewport.Model
	draft       textarea.Model
	resumeDraft bool
	selectID    string
	busy        bool
	message     string
	width       int
	height      int
}

const (
	emailInput = iota
	codeInput
	usernameInput
	zoneInput
)

func New(client emailAPI, store sessionStore) Model {
	m := Model{api: client, store: store, stage: startupStage, width: 80, height: 24, busy: true, reader: viewport.New(68, 14)}
	m.draft = textarea.New()
	m.draft.Placeholder = "What's on your mind?"
	m.draft.CharLimit = 20000
	m.draft.ShowLineNumbers = false
	m.draft.SetWidth(68)
	m.draft.SetHeight(12)
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

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, func() tea.Msg {
		token, err := m.store.Load()
		return loadedSession{token, err}
	})
}

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
type loadedSession struct {
	token string
	err   error
}
type inboxResult struct {
	account api.Account
	page    api.NotesPage
	err     error
	saveErr error
}
type clearedSession struct{ err error }
type createdNote struct {
	note api.Note
	err  error
}

func (m Model) loadInbox(token string, save bool) tea.Cmd {
	return func() tea.Msg {
		var saveErr error
		if save {
			saveErr = m.store.Save(token)
			if saveErr != nil {
				// Do not leave an older session saved under the same origin.
				_ = m.store.Delete()
			}
		}
		account, err := m.api.Me(context.Background(), token)
		if err != nil {
			return inboxResult{err: err, saveErr: saveErr}
		}
		page, err := m.api.Inbox(context.Background(), token)
		return inboxResult{account: account, page: page, err: err, saveErr: saveErr}
	}
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeReader()
		m.draft.SetWidth(m.contentWidth())
		m.draft.SetHeight(max(3, m.height-10))
		return m, nil
	case loadedSession:
		if msg.err != nil {
			m.stage, m.busy = emailStage, false
			m.message = "Credential store unavailable. Sign in; this session may not be saved."
			return m, nil
		}
		if msg.token == "" {
			m.stage, m.busy = emailStage, false
			return m, nil
		}
		m.token = msg.token
		m.message = "Checking saved session…"
		return m, m.loadInbox(msg.token, false)
	case clearedSession:
		m.busy = false
		if msg.err != nil {
			m.message = "Could not remove the old saved session; it may reopen on next launch."
		}
		return m, nil
	case inboxResult:
		m.busy = false
		if msg.err != nil {
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.stage = emailStage
				m.focusInput(emailInput)
				m.message = "Session expired. Sign in again."
				return m, nil
			}
			m.stage = inboxStage
			if m.selectID != "" {
				m.message = "Note saved, but Inbox refresh failed. Press r to retry."
			} else {
				m.message = friendlyError(msg.err, "Could not load your Inbox.") + " Press r to retry."
			}
			return m, nil
		}
		m.stage = inboxStage
		m.username = msg.account.Username
		m.notes = msg.page.Items
		m.more = msg.page.NextCursor != nil
		m.selected = 0
		m.message = ""
		if m.selectID != "" {
			for i, note := range m.notes {
				if note.ID == m.selectID {
					m.selected = i
					break
				}
			}
			m.selectID = ""
			m.message = "Note saved to Inbox."
		}
		if msg.saveErr != nil {
			m.message = "Could not save session to the OS credential store; you may need to sign in next time."
		}
		if m.resumeDraft {
			m.resumeDraft = false
			m.stage = captureStage
			m.message = "Sign-in complete. Your unsaved draft is ready."
			return m, m.draft.Focus()
		}
		return m, nil
	case createdNote:
		if msg.err != nil {
			m.busy = false
			var apiErr *api.Error
			if errors.As(msg.err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				_ = m.store.Delete()
				m.token = ""
				m.resumeDraft = true
				m.stage = emailStage
				m.focusInput(emailInput)
				m.message = "Session expired. Sign in again to resume your draft."
				return m, nil
			}
			if errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request" {
				m.message = "The server rejected this note. Check its content and try again."
			} else {
				m.message = "Save not confirmed. Check Inbox before retrying to avoid a duplicate. Your draft is still here."
			}
			return m, nil
		}
		m.stage = inboxStage
		m.selectID = msg.note.ID
		m.notes = append([]api.Note{msg.note}, m.notes...)
		m.selected = 0
		m.draft.SetValue("")
		m.draft.Blur()
		m.message = "Note saved. Refreshing Inbox…"
		return m, m.loadInbox(m.token, false)
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
			return m, m.signedIn(msg.result.Token)
		}
		return m, nil
	case restoreResult:
		m.busy = false
		if msg.err != nil {
			m.message = friendlyError(msg.err, "Could not restore the account.")
		} else {
			return m, m.signedIn(msg.token)
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
			case captureStage:
				m.stage = inboxStage
				m.draft.Blur()
				m.draft.SetValue("")
				m.message = "Draft discarded."
				return m, nil
			case startupStage:
				m.stage = emailStage
				m.token = ""
				m.focusInput(emailInput)
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
			case readingStage:
				m.stage = inboxStage
				return m, nil
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
		case startupStage:
			if msg.String() == "r" && m.token != "" {
				m.busy = true
				return m, m.loadInbox(m.token, false)
			}
		case inboxStage:
			switch msg.String() {
			case "n":
				m.stage = captureStage
				m.draft.SetValue("")
				m.message = ""
				return m, m.draft.Focus()
			case "q":
				return m, tea.Quit
			case "s":
				m.stage = emailStage
				m.token = ""
				m.notes = nil
				m.busy = true
				m.message = "Removing saved session…"
				m.focusInput(emailInput)
				return m, func() tea.Msg { return clearedSession{m.store.Delete()} }
			case "r":
				m.busy = true
				return m, m.loadInbox(m.token, false)
			case "j", "down":
				if m.selected+1 < len(m.notes) {
					m.selected++
				}
			case "k", "up":
				if m.selected > 0 {
					m.selected--
				}
			case "enter":
				if len(m.notes) > 0 {
					m.stage = readingStage
					m.resizeReader()
					m.reader.SetContent(lipgloss.NewStyle().Width(m.reader.Width).Render(safeText(m.notes[m.selected].Content)))
					m.reader.GotoTop()
				}
			}
			return m, nil
		case readingStage:
			if msg.String() == "q" {
				return m, tea.Quit
			}
			var cmd tea.Cmd
			m.reader, cmd = m.reader.Update(msg)
			return m, cmd
		case captureStage:
			if msg.String() == "ctrl+s" {
				content := m.draft.Value()
				if strings.TrimSpace(content) == "" {
					m.message = "Write something before saving."
					return m, nil
				}
				if utf8.RuneCountInString(content) > 20000 {
					m.message = "Notes must be at most 20,000 characters."
					return m, nil
				}
				m.busy = true
				m.message = "Saving note…"
				return m, func() tea.Msg {
					note, err := m.api.CreateInboxNote(context.Background(), m.token, content)
					return createdNote{note, err}
				}
			}
			var cmd tea.Cmd
			m.draft, cmd = m.draft.Update(msg)
			if m.message != "" {
				m.message = ""
			}
			return m, cmd
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

func (m *Model) signedIn(token string) tea.Cmd {
	m.token = token
	m.ticket = ""
	m.challenge = ""
	m.inputs[codeInput].SetValue("")
	m.stage = inboxStage
	m.busy = true
	m.message = "Loading Inbox…"
	return m.loadInbox(token, true)
}

func (m *Model) resizeReader() {
	width := m.contentWidth()
	m.reader.Width = width
	m.reader.Height = max(3, m.height-10)
	if m.stage == readingStage && len(m.notes) > m.selected {
		m.reader.SetContent(lipgloss.NewStyle().Width(width).Render(safeText(m.notes[m.selected].Content)))
	}
}

func (m Model) contentWidth() int {
	return max(20, min(68, m.width-4))
}

// Notes are untrusted terminal text; remove control sequences before rendering.
func safeText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

func preview(text string, limit int) string {
	first, _, _ := strings.Cut(safeText(text), "\n")
	first = strings.TrimSpace(first)
	runes := []rune(first)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return first
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
	case startupStage:
		body = "Checking saved session…\n\n" + dimStyle.Render("Ctrl+C quit")
	case emailStage:
		body = "Sign in with email\n\nEmail\n" + m.inputs[emailInput].View() + "\n\n" + dimStyle.Render("Enter send code · Ctrl+C quit")
	case codeStage:
		body = fmt.Sprintf("Code sent to %s\n\n8-digit code\n%s\n\n%s", m.email, m.inputs[codeInput].View(), dimStyle.Render("Enter verify · Esc change email · Ctrl+C quit"))
	case profileStage:
		body = "Finish creating your account\n\nName\n" + m.inputs[usernameInput].View() + "\n\nTime zone (IANA)\n" + m.inputs[zoneInput].View() + "\n\n" + dimStyle.Render("Tab switch field · Enter verify · Esc back")
	case restoreStage:
		body = "This account is scheduled for deletion.\nRestoring it keeps its projects and notes, but previously signed-in devices remain signed out.\n\nRestore this account? [y/N]"
	case inboxStage:
		body = "Inbox"
		if m.username != "" {
			body += " · " + safeText(m.username)
		}
		body += "\n\n"
		if len(m.notes) == 0 && !m.busy && m.message == "" {
			body += "No Inbox notes yet.\n"
		}
		rows := max(3, m.height-11)
		start := max(0, m.selected-rows+1)
		for i := start; i < len(m.notes) && i < start+rows; i++ {
			mark := "  "
			if i == m.selected {
				mark = "› "
			}
			body += fmt.Sprintf("%s%s  %s\n", mark, m.notes[i].CreatedAt.Local().Format("Jan 02"), preview(m.notes[i].Content, max(12, m.contentWidth()-16)))
		}
		if m.more {
			body += "\n" + dimStyle.Render("Showing newest 50 notes; older pages are not available yet.")
		}
		body += "\n" + dimStyle.Render("n new · ↑/↓ or j/k select · Enter read · r refresh · s sign in · q quit")
	case readingStage:
		body = "Inbox · " + m.notes[m.selected].CreatedAt.Local().Format("Jan 02, 2006 15:04") + "\n\n" + m.reader.View() + "\n\n" + dimStyle.Render("↑/↓ or j/k scroll · Esc back · q quit")
	case captureStage:
		body = "New Inbox note\n\n" + m.draft.View() + "\n\n" + dimStyle.Render("Enter new line · Ctrl+S save · Esc discard")
	}
	if m.busy {
		body += "\n\nWorking…"
	}
	if m.message != "" {
		body += "\n\n" + errStyle.Render(m.message)
	}
	content := titleStyle.Render("Shortlog") + "\n\n" + body
	width := m.contentWidth()
	return lipgloss.NewStyle().Width(width).Padding(1, 2).Render(content)
}
