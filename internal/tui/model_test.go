package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"shortlog-cli/internal/api"
)

type fakeAPI struct {
	projectNoteLoads     []string
	telegramStart        api.TelegramStart
	telegramStartErr     error
	telegramPoll         []api.VerifyResult
	telegramPollErr      error
	telegramPollCalls    []string
	telegramRestoreErr   error
	telegramRestoreCalls int
	account              api.Account
	accountErr           error
	profileErr           error
	profileCalls         int
	deletionErr          error
	deletionCalls        int
	sessionList          []api.Session
	sessionErr           error
	actionErr            error
	logoutCalls          int
	revokeCalls          []string
	revokeAllCalls       int
	starts               int
	verifies             int
	restores             int
	name                 string
	zone                 string
	meErr                error
	inboxErr             error
	notes                []api.Note
	next                 *string
	pages                map[string]api.NotesPage
	pageErr              error
	requested            []string
	createErr            error
	created              []string
	updateErr            error
	updated              []string
	deleteErr            error
	deleted              []string
	moveErr              error
	moved                []string

	projectList      []api.Project
	archivedList     []api.Project
	projectsErr      error
	archivedErr      error
	archiveErr       error
	unarchiveErr     error
	archiveCalls     []string
	unarchiveCalls   []string
	createProjectErr error
	projectNotes     []api.Note
	projectNext      *string
	projectPages     map[string]api.NotesPage
	projectPageErr   error
	projectRequested []string
	projectCreated   []string
}

func (f *fakeAPI) StartTelegram(_ context.Context) (api.TelegramStart, error) {
	return f.telegramStart, f.telegramStartErr
}
func (f *fakeAPI) PollTelegram(_ context.Context, id, secret, name, zone string) (api.VerifyResult, error) {
	f.telegramPollCalls = append(f.telegramPollCalls, id+":"+secret+":"+name+":"+zone)
	if f.telegramPollErr != nil {
		return api.VerifyResult{}, f.telegramPollErr
	}
	if len(f.telegramPoll) == 0 {
		return api.VerifyResult{Status: "pending"}, nil
	}
	result := f.telegramPoll[0]
	f.telegramPoll = f.telegramPoll[1:]
	return result, nil
}
func (f *fakeAPI) RestoreTelegram(_ context.Context, _ string) (string, error) {
	f.telegramRestoreCalls++
	if f.telegramRestoreErr != nil {
		return "", f.telegramRestoreErr
	}
	return "telegram-token", nil
}

func (f *fakeAPI) UpdateProfile(_ context.Context, _, name, zone string) (api.Account, error) {
	f.profileCalls++
	if f.profileErr != nil {
		return api.Account{}, f.profileErr
	}
	f.account.Username, f.account.TimeZone = name, zone
	return f.account, nil
}
func (f *fakeAPI) RequestAccountDeletion(_ context.Context, _ string) (time.Time, error) {
	f.deletionCalls++
	if f.deletionErr != nil {
		return time.Time{}, f.deletionErr
	}
	return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC), nil
}

func (f *fakeAPI) Sessions(_ context.Context, _ string) ([]api.Session, error) {
	return f.sessionList, f.sessionErr
}
func (f *fakeAPI) Logout(_ context.Context, _ string) error { f.logoutCalls++; return f.actionErr }
func (f *fakeAPI) RevokeSession(_ context.Context, _, id string) error {
	f.revokeCalls = append(f.revokeCalls, id)
	return f.actionErr
}
func (f *fakeAPI) RevokeAllSessions(_ context.Context, _ string) error {
	f.revokeAllCalls++
	return f.actionErr
}

type fakeStore struct {
	token     string
	saveErr   error
	deletes   int
	deleteErr error
	saves     int
}

func (s *fakeStore) Load() (string, error) { return s.token, nil }
func (s *fakeStore) Save(token string) error {
	s.saves++
	if s.saveErr == nil {
		s.token = token
	}
	return s.saveErr
}
func (s *fakeStore) Delete() error {
	s.deletes++
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.token = ""
	return nil
}

func (f *fakeAPI) StartEmail(_ context.Context, _ string) (string, error) {
	f.starts++
	return "challenge", nil
}

func (f *fakeAPI) VerifyEmail(_ context.Context, _, _, name, zone string) (api.VerifyResult, error) {
	f.verifies++
	f.name, f.zone = name, zone
	if name == "" {
		return api.VerifyResult{}, &api.Error{Status: 422, Code: "profile_required"}
	}
	return api.VerifyResult{Status: "signed_in", Token: "secret-token"}, nil
}

func (f *fakeAPI) RestoreEmail(_ context.Context, _ string) (string, error) {
	f.restores++
	return "restored-token", nil
}

func (f *fakeAPI) Me(_ context.Context, _ string) (api.Account, error) {
	if f.account.ID != "" || f.accountErr != nil {
		return f.account, f.accountErr
	}
	return api.Account{ID: "account", Username: "Ari"}, f.meErr
}
func (f *fakeAPI) Inbox(_ context.Context, _ string) (api.NotesPage, error) {
	return api.NotesPage{Items: f.notes, NextCursor: f.next}, f.inboxErr
}
func (f *fakeAPI) InboxPage(_ context.Context, _, cursor string) (api.NotesPage, error) {
	f.requested = append(f.requested, cursor)
	return f.pages[cursor], f.pageErr
}
func (f *fakeAPI) CreateInboxNote(_ context.Context, _, content string) (api.Note, error) {
	f.created = append(f.created, content)
	if f.createErr != nil {
		return api.Note{}, f.createErr
	}
	note := api.Note{ID: "created", Content: content, CreatedAt: time.Now()}
	f.notes = append([]api.Note{note}, f.notes...)
	return note, nil
}
func (f *fakeAPI) UpdateNote(_ context.Context, _, id, content string) (api.Note, error) {
	f.updated = append(f.updated, content)
	if f.updateErr != nil {
		return api.Note{}, f.updateErr
	}
	for i := range f.notes {
		if f.notes[i].ID == id {
			f.notes[i].Content = content
			return f.notes[i], nil
		}
	}
	return api.Note{ID: id, Content: content}, nil
}
func (f *fakeAPI) DeleteNote(_ context.Context, _, id string) error {
	f.deleted = append(f.deleted, id)
	return f.deleteErr
}
func (f *fakeAPI) MoveNote(_ context.Context, _, id, projectID string) (api.Note, error) {
	f.moved = append(f.moved, id+":"+projectID)
	if f.moveErr != nil {
		return api.Note{}, f.moveErr
	}
	for _, source := range []*[]api.Note{&f.notes, &f.projectNotes} {
		for i, note := range *source {
			if note.ID != id {
				continue
			}
			*source = append(append([]api.Note(nil), (*source)[:i]...), (*source)[i+1:]...)
			note.ProjectID = nil
			if projectID != "" {
				target := projectID
				note.ProjectID = &target
				f.projectNotes = append([]api.Note{note}, f.projectNotes...)
			} else {
				f.notes = append([]api.Note{note}, f.notes...)
			}
			return note, nil
		}
	}
	return api.Note{ID: id}, nil
}
func (f *fakeAPI) Projects(_ context.Context, _ string) ([]api.Project, error) {
	return f.projectList, f.projectsErr
}
func (f *fakeAPI) ArchivedProjects(_ context.Context, _ string) ([]api.Project, error) {
	return f.archivedList, f.archivedErr
}
func (f *fakeAPI) ArchiveProject(_ context.Context, _, id string) error {
	f.archiveCalls = append(f.archiveCalls, id)
	if f.archiveErr != nil {
		return f.archiveErr
	}
	for i, project := range f.projectList {
		if project.ID == id {
			f.projectList = append(append([]api.Project(nil), f.projectList[:i]...), f.projectList[i+1:]...)
			now := time.Now()
			project.ArchivedAt = &now
			f.archivedList = append([]api.Project{project}, f.archivedList...)
			break
		}
	}
	return nil
}
func (f *fakeAPI) UnarchiveProject(_ context.Context, _, id string) error {
	f.unarchiveCalls = append(f.unarchiveCalls, id)
	if f.unarchiveErr != nil {
		return f.unarchiveErr
	}
	for i, project := range f.archivedList {
		if project.ID == id {
			f.archivedList = append(append([]api.Project(nil), f.archivedList[:i]...), f.archivedList[i+1:]...)
			project.ArchivedAt = nil
			f.projectList = append([]api.Project{project}, f.projectList...)
			break
		}
	}
	return nil
}
func (f *fakeAPI) CreateProject(_ context.Context, _, name string) (api.Project, error) {
	if f.createProjectErr != nil {
		return api.Project{}, f.createProjectErr
	}
	project := api.Project{ID: "project-" + name, Name: name}
	f.projectList = append([]api.Project{project}, f.projectList...)
	return project, nil
}
func (f *fakeAPI) ProjectNotes(_ context.Context, _, id string) (api.NotesPage, error) {
	f.projectNoteLoads = append(f.projectNoteLoads, id)
	return api.NotesPage{Items: f.projectNotes, NextCursor: f.projectNext}, f.inboxErr
}
func (f *fakeAPI) ProjectNotesPage(_ context.Context, _, _, cursor string) (api.NotesPage, error) {
	f.projectRequested = append(f.projectRequested, cursor)
	return f.projectPages[cursor], f.projectPageErr
}
func (f *fakeAPI) CreateProjectNote(_ context.Context, _, _, content string) (api.Note, error) {
	f.projectCreated = append(f.projectCreated, content)
	if f.createErr != nil {
		return api.Note{}, f.createErr
	}
	note := api.Note{ID: "project-created", Content: content, CreatedAt: time.Now()}
	f.projectNotes = append([]api.Note{note}, f.projectNotes...)
	return note, nil
}

func press(m Model, key tea.KeyPressMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

func TestEmailProfileSignIn(t *testing.T) {
	f := &fakeAPI{}
	s := &fakeStore{}
	m := New(f, s)
	m.stage, m.busy = loginStage, false
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.login.step != emailStep || m.login.form == nil {
		t.Fatalf("email form did not open: stage=%d", m.stage)
	}
	m = typeText(m, "a@example.com")
	m, cmd = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.busy || cmd == nil {
		t.Fatal("expected async email start")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.login.step != codeStep || f.starts != 1 {
		t.Fatalf("start: stage=%d, requests=%d", m.stage, f.starts)
	}
	m = typeText(m, "12345678")
	m.login.values.zone = "" // start the profile form empty whatever the machine's TZ
	m, cmd = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.login.step != profileStep || m.login.values.code != "12345678" {
		t.Fatal("expected profile form with code retained")
	}
	m = typeText(m, "Ari")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = typeText(m, "Europe/London")
	m, cmd = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	next, cmd = m.Update(cmd())
	m = next.(Model)
	if m.stage != workspaceStage || m.token != "secret-token" || f.name != "Ari" || f.zone != "Europe/London" {
		t.Fatalf("sign-in: stage=%d, name=%q, zone=%q", m.stage, f.name, f.zone)
	}
	m = feed(m, cmd)
	if s.token != "secret-token" || m.busy || m.workspace.loading() || m.workspace.userName() != "Ari" {
		t.Fatal("session not saved or workspace not loaded")
	}
	if strings.Contains(m.View().Content, "secret-token") || strings.Contains(m.View().Content, "12345678") {
		t.Fatal("secret appeared in screen")
	}
}

// feed runs a command, including any it batches, and applies each result.
// Commands those results return are not run, so a chain stops after one step.
func feed(m Model, cmd tea.Cmd) Model {
	for _, msg := range runCmd(cmd) {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// runeKey is a key press that types r.
func runeKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// typeText feeds rune keys into the focused Huh form field.
func typeText(m Model, text string) Model {
	for _, r := range text {
		next, _ := m.Update(runeKey(r))
		m = next.(Model)
	}
	return m
}

func TestRestoreNeedsExplicitConsent(t *testing.T) {
	f := &fakeAPI{}
	m := New(f, &fakeStore{})
	m.stage, m.busy = loginStage, false
	next, _ := m.Update(verifyResult{result: api.VerifyResult{Status: "restore_required", RecoveryTicket: "secret-ticket"}})
	m = next.(Model)
	if m.login.step != restoreStep || strings.Contains(m.View().Content, "secret-ticket") {
		t.Fatal("expected restoration confirmation without ticket exposure")
	}
	m, _ = press(m, runeKey('n'))
	if f.restores != 0 || m.login.ticket != "" || m.login.step != menuStep {
		t.Fatal("decline must not restore")
	}
	next, _ = m.Update(verifyResult{result: api.VerifyResult{Status: "restore_required", RecoveryTicket: "secret-ticket"}})
	m = next.(Model)
	m, cmd := press(m, runeKey('y'))
	if cmd == nil || f.restores != 0 {
		t.Fatal("restore should be asynchronous after consent")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if f.restores != 1 || m.stage != workspaceStage || m.token != "restored-token" || m.login.ticket != "" {
		t.Fatal("restore did not finish securely")
	}
}

func TestInvalidCodeDoesNotCallAPI(t *testing.T) {
	f := &fakeAPI{}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.login.step = loginStage, false, codeStep
	m.login.challenge = "challenge"
	m.login.showForm(codeStep)
	m = typeText(m, "123")
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || f.verifies != 0 || m.busy {
		t.Fatal("invalid code sent to API")
	}
}

func TestSavedSessionAndReadInbox(t *testing.T) {
	f := &fakeAPI{notes: []api.Note{{ID: "one", Content: "first\nline", CreatedAt: time.Now()}, {ID: "two", Content: "second", CreatedAt: time.Now()}}}
	s := &fakeStore{token: "saved-token"}
	m := New(f, s)
	next, cmd := m.Update(loadedSession{token: s.token})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected validation request")
	}
	m = feed(m, cmd)
	if m.stage != workspaceStage || len(m.workspace.notes.inbox) != 2 || m.token != "saved-token" || s.saves != 0 {
		t.Fatal("saved session did not open the workspace with the Inbox, or was saved again")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.workspace.focus != focusMain || !strings.Contains(m.View().Content, "second") {
		t.Fatal("did not open the second note")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.workspace.focus != focusNotes {
		t.Fatal("did not return to the notes")
	}
}

func TestExpiredSessionRemoved(t *testing.T) {
	f := &fakeAPI{meErr: &api.Error{Status: 401, Code: "unauthorized"}}
	s := &fakeStore{token: "expired-token"}
	m := New(f, s)
	next, cmd := m.Update(loadedSession{token: s.token})
	m = feed(next.(Model), cmd)
	if m.stage != loginStage || m.token != "" || s.token != "" || s.deletes != 1 || !strings.Contains(m.View().Content, "Session expired") {
		t.Fatal("expired session not cleared")
	}
}

func TestNetworkErrorKeepsSavedSession(t *testing.T) {
	f := &fakeAPI{meErr: errors.New("offline")}
	s := &fakeStore{token: "saved-token"}
	m := New(f, s)
	next, cmd := m.Update(loadedSession{token: s.token})
	m = feed(next.(Model), cmd)
	if m.stage != workspaceStage || s.token != "saved-token" || s.deletes != 0 || !strings.Contains(m.View().Content, "retry") {
		t.Fatal("network error lost session")
	}
}

func TestSanitizeNoteContent(t *testing.T) {
	if got := safeText("ok\x1b[31m\nnext"); got != "ok[31m\nnext" {
		t.Fatalf("unsafe text: %q", got)
	}
}

func TestTelegramSignInAndNewProfile(t *testing.T) {
	start := api.TelegramStart{AttemptID: "attempt", PollSecret: "private-secret", AuthorizationURL: "https://oauth.telegram.org/auth?state=abc"}
	f := &fakeAPI{telegramStart: start, telegramPoll: []api.VerifyResult{{Status: "pending"}, {Status: "signed_in", Token: "telegram-token"}}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.login.step = loginStage, false, emailStep
	m.login.openBrowser = func(string) error { return errors.New("no browser") }
	m, cmd := press(m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	if cmd == nil || !m.busy {
		t.Fatal("Telegram start not dispatched")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.login.step != telegramStep || strings.Contains(m.View().Content, "private-secret") {
		t.Fatal("attempt not started securely")
	}
	next, _ = m.Update(telegramBrowserResult{attempt: "attempt", err: errors.New("no browser")})
	m = next.(Model)
	if !strings.Contains(m.View().Content, start.AuthorizationURL) {
		t.Fatal("no manual browser fallback")
	}
	m, poll := press(m, runeKey('r'))
	next, _ = m.Update(poll())
	m = next.(Model)
	if m.login.step != telegramStep || m.busy || strings.Contains(m.View().Content, "private-secret") {
		t.Fatal("pending result failed")
	}
	f.telegramPollErr = &api.Error{Status: 422, Code: "profile_required"}
	m.login.values.zone = "" // start the profile form empty whatever the machine's TZ
	m, poll = press(m, runeKey('r'))
	next, _ = m.Update(poll())
	m = next.(Model)
	if m.login.step != profileStep || !m.login.telegramLogin {
		t.Fatal("new account profile not prompted")
	}
	f.telegramPollErr = nil
	m = typeText(m, "New User")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = typeText(m, "UTC")
	m, poll = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	next, _ = m.Update(poll())
	m = next.(Model)
	if m.stage != workspaceStage || m.token != "telegram-token" || m.login.telegramSecret != "" || len(f.telegramPollCalls) != 3 || !strings.Contains(f.telegramPollCalls[2], "New User:UTC") {
		t.Fatal("Telegram profile completion failed")
	}
}

func TestTelegramRestoreRequiresConsent(t *testing.T) {
	start := api.TelegramStart{AttemptID: "attempt", PollSecret: "private-secret", AuthorizationURL: "https://oauth.telegram.org/auth"}
	f := &fakeAPI{telegramStart: start, telegramPoll: []api.VerifyResult{{Status: "restore_required", RecoveryTicket: "private-ticket"}}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.login.step = loginStage, false, emailStep
	m.login.openBrowser = func(string) error { return nil }
	m, cmd := press(m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	next, _ := m.Update(cmd())
	m = next.(Model)
	m, poll := press(m, runeKey('r'))
	next, _ = m.Update(poll())
	m = next.(Model)
	if m.login.step != restoreStep || strings.Contains(m.View().Content, "private-ticket") || f.telegramRestoreCalls != 0 {
		t.Fatal("restore consent was skipped or secret leaked")
	}
	m, cmd = press(m, runeKey('y'))
	next, _ = m.Update(cmd())
	m = next.(Model)
	if f.telegramRestoreCalls != 1 || m.stage != workspaceStage || m.token != "telegram-token" {
		t.Fatal("Telegram restore failed")
	}
}

func TestTelegramCancelAndUnsafeURL(t *testing.T) {
	f := &fakeAPI{telegramStart: api.TelegramStart{AttemptID: "attempt", PollSecret: "private", AuthorizationURL: "javascript:alert(1)"}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.login.step = loginStage, false, emailStep
	m.login.openBrowser = func(string) error { t.Fatal("unsafe browser URL opened"); return nil }
	m, cmd := press(m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.login.step != emailStep || !strings.Contains(m.View().Content, "unsafe") || m.login.telegramSecret != "" {
		t.Fatal("unsafe URL accepted")
	}
	f.telegramStart.AuthorizationURL = "https://oauth.telegram.org/auth"
	m, cmd = press(m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	next, _ = m.Update(cmd())
	m = next.(Model)
	m, poll := press(m, runeKey('r'))
	if !m.busy {
		t.Fatal("poll not running")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.login.step != emailStep || m.busy || m.login.telegramSecret != "" {
		t.Fatal("could not cancel in-flight poll")
	}
	next, _ = m.Update(poll())
	m = next.(Model)
	if m.login.step != emailStep || m.token != "" {
		t.Fatal("late poll result signed in after cancellation")
	}
}

func TestTelegramAutomaticPollAndStaleTick(t *testing.T) {
	f := &fakeAPI{telegramStart: api.TelegramStart{AttemptID: "attempt", PollSecret: "secret", AuthorizationURL: "https://oauth.telegram.org/auth"}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.login.step = loginStage, false, emailStep
	m.login.openBrowser = func(string) error { return nil }
	m, cmd := press(m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	next, _ := m.Update(cmd())
	m = next.(Model)
	tick := telegramTick{attempt: m.login.telegramAttempt, generation: m.login.telegramGeneration}
	next, poll := m.Update(tick)
	m = next.(Model)
	if !m.busy || poll == nil {
		t.Fatal("automatic poll did not start")
	}
	next, timer := m.Update(poll())
	m = next.(Model)
	if m.busy || timer == nil || len(f.telegramPollCalls) != 1 {
		t.Fatal("pending poll did not schedule another check")
	}
	next, stale := m.Update(tick)
	m = next.(Model)
	if stale != nil || len(f.telegramPollCalls) != 1 {
		t.Fatal("stale tick caused duplicate poll")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	next, ignored := m.Update(telegramTick{attempt: "attempt", generation: m.login.telegramGeneration})
	m = next.(Model)
	if ignored != nil || m.login.step != emailStep {
		t.Fatal("cancelled attempt kept polling")
	}
}

func TestTelegramRestoreDeclineAndStartFailure(t *testing.T) {
	f := &fakeAPI{telegramStartErr: &api.Error{Status: 503, Code: "service_unavailable"}}
	m := New(f, &fakeStore{})
	m.stage, m.busy, m.login.step = loginStage, false, emailStep
	m, cmd := press(m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.login.step != emailStep || m.busy || !strings.Contains(m.View().Content, "unavailable") {
		t.Fatal("unconfigured Telegram server failure not shown")
	}
	m.login.telegramLogin = true
	m.login.telegramAttempt = "attempt"
	m.login.ticket = "private-ticket"
	m.stage, m.login.step = loginStage, restoreStep
	m, _ = press(m, runeKey('n'))
	if m.login.step != menuStep || f.telegramRestoreCalls != 0 || m.login.ticket != "" || m.login.telegramAttempt != "" {
		t.Fatal("declining restore did not clear secrets")
	}
}

func TestEditorsHaveNoCursorLineBackground(t *testing.T) {
	capture := newCapture("New note", "", pendingAction{kind: newNoteAction})
	editor := newNoteEditor(notePlace{Note: api.Note{ID: "n", Content: "x"}}, inboxSource, "x")
	for name, bg := range map[string]any{
		"capture": capture.editor.Styles().Focused.CursorLine.GetBackground(),
		"editor":  editor.editor.Styles().Focused.CursorLine.GetBackground(),
	} {
		// The textarea's default focused-line background is black in dark
		// terminals.
		if _, ok := bg.(lipgloss.NoColor); !ok {
			t.Errorf("%s: focused line has a background colour", name)
		}
	}
}
