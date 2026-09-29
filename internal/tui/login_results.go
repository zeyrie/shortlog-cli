package tui

import (
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"shortlog-cli/internal/api"
)

func (m Model) onLoadedSession(msg loadedSession) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.stage, m.busy = loginStage, false
		m.inputs[m.focus].Blur()
		m.message = "Credential store unavailable. Sign in; this session may not be saved."
		return m, nil
	}
	if msg.token == "" {
		m.stage, m.busy = loginStage, false
		m.inputs[m.focus].Blur()
		return m, nil
	}
	m.token = msg.token
	m.message = "Checking saved session…"
	return m, m.loadInbox(msg.token, false)
}

func (m Model) onTelegramStart(msg telegramStartResult) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		m.clearTelegram()
		m.stage = m.telegramBack
		var apiErr *api.Error
		if errors.As(msg.err, &apiErr) && apiErr.Code == "service_unavailable" {
			m.message = "Telegram sign-in unavailable. Configure Telegram on the server or use email."
		} else {
			m.message = friendlyError(msg.err, "Could not start Telegram sign-in.")
		}
		return m, nil
	}
	if !validTelegramURL(msg.start.AuthorizationURL) {
		m.clearTelegram()
		m.stage = m.telegramBack
		m.message = "Server returned an unsafe Telegram sign-in URL. Sign-in was cancelled."
		return m, nil
	}
	m.telegramLogin = true
	m.telegramAttempt = msg.start.AttemptID
	m.telegramSecret = msg.start.PollSecret
	m.telegramURL = msg.start.AuthorizationURL
	m.telegramExpires = time.Now().Add(10 * time.Minute)
	m.stage = telegramStage
	m.message = "Approve sign-in in your browser; this screen checks automatically."
	id, address, opener := m.telegramAttempt, m.telegramURL, m.openBrowser
	return m, tea.Batch(m.telegramTimer(), func() tea.Msg {
		return telegramBrowserResult{id, opener(address)}
	})
}

func (m Model) onTelegramBrowser(msg telegramBrowserResult) (tea.Model, tea.Cmd) {
	if m.stage == telegramStage && m.telegramAttempt == msg.attempt && msg.err != nil {
		m.message = "Could not open the browser. Press o to retry, or copy the URL below into your browser."
	}
	return m, nil
}

func (m Model) onTelegramTick(msg telegramTick) (tea.Model, tea.Cmd) {
	if m.stage != telegramStage || m.telegramAttempt != msg.attempt || m.telegramGeneration != msg.generation || m.busy {
		return m, nil
	}
	return m, m.pollTelegram("", "")
}

func (m Model) onTelegramPoll(msg telegramPollResult) (tea.Model, tea.Cmd) {
	if m.telegramAttempt != msg.attempt || !m.telegramLogin || (m.stage != telegramStage && m.stage != profileStage) {
		return m, nil
	}
	m.busy = false
	if msg.err != nil {
		var apiErr *api.Error
		if errors.As(msg.err, &apiErr) && apiErr.Code == "profile_required" {
			m.stage = profileStage
			m.focusInput(usernameInput)
			m.message = "New Telegram account: enter a name and IANA time zone."
			return m, nil
		}
		if errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request" {
			if m.stage == profileStage {
				m.message = "Server rejected the profile or attempt. Check your details; if expired, press Esc and start again."
			} else {
				m.message = "Telegram attempt expired or invalid. Press Esc then choose 2 to start again."
			}
		} else if errors.As(msg.err, &apiErr) && apiErr.Code == "service_unavailable" {
			m.message = "Telegram sign-in is temporarily unavailable. Press r to retry or Esc to cancel."
		} else {
			m.message = friendlyError(msg.err, "Could not check Telegram approval.") + " Press r to retry."
		}
		return m, nil
	}
	switch msg.result.Status {
	case "pending":
		if m.stage == profileStage {
			m.message = "Still waiting for Telegram approval. Press Enter to check again."
			return m, nil
		}
		m.message = "Waiting for Telegram approval… Press r to check now or Esc to cancel."
		return m, m.telegramTimer()
	case "restore_required":
		m.stage = restoreStage
		m.ticket = msg.result.RecoveryTicket
		m.message = ""
		return m, nil
	case "signed_in":
		m.clearTelegram()
		return m, m.signedIn(msg.result.Token)
	}
	return m, nil
}

func (m Model) onEmailStart(msg startResult) (tea.Model, tea.Cmd) {
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
}

func (m Model) onEmailVerify(msg verifyResult) (tea.Model, tea.Cmd) {
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
}

func (m Model) onRestore(msg restoreResult) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		var apiErr *api.Error
		if m.telegramLogin && errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request" {
			m.message = "Telegram restoration expired or was already used. Press Esc then choose 2 to start again."
		} else {
			m.message = friendlyError(msg.err, "Could not restore the account.")
		}
	} else {
		m.clearTelegram()
		return m, m.signedIn(msg.token)
	}
	return m, nil
}
