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
		m.setStatus(statusWarn, "Credential store unavailable; this session may not be saved.")
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
			m.setStatus(statusError, "Telegram sign-in unavailable on this server. Use email.")
		} else {
			m.setStatus(statusError, friendlyError(msg.err, "Could not start Telegram sign-in."))
		}
		return m, nil
	}
	if !validTelegramURL(msg.start.AuthorizationURL) {
		m.clearTelegram()
		m.stage = m.telegramBack
		m.setStatus(statusError, "Server sent an unsafe Telegram URL; sign-in cancelled.")
		return m, nil
	}
	m.telegramLogin = true
	m.telegramAttempt = msg.start.AttemptID
	m.telegramSecret = msg.start.PollSecret
	m.telegramURL = msg.start.AuthorizationURL
	m.telegramExpires = time.Now().Add(10 * time.Minute)
	m.stage = telegramStage
	m.setStatus(statusInfo, "Waiting for Telegram approval…")
	id, address, opener := m.telegramAttempt, m.telegramURL, m.openBrowser
	return m, tea.Batch(m.telegramTimer(), func() tea.Msg {
		return telegramBrowserResult{id, opener(address)}
	})
}

func (m Model) onTelegramBrowser(msg telegramBrowserResult) (tea.Model, tea.Cmd) {
	if m.stage == telegramStage && m.telegramAttempt == msg.attempt {
		m.telegramBrowserFailed = msg.err != nil
		if msg.err != nil {
			m.setStatus(statusError, "Could not open the browser. Copy the URL above, or press o.")
		}
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
			m.setStatus(statusInfo, "New Telegram account: add a name and time zone.")
			return m, m.showLoginForm(profileStage)
		}
		if errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request" {
			if m.stage == profileStage {
				m.setStatus(statusError, "Profile or attempt rejected. Check details, or Esc to restart.")
				return m, nil
			} else {
				m.setStatus(statusError, "Telegram attempt expired. Esc, then 2 to start again.")
			}
		} else if errors.As(msg.err, &apiErr) && apiErr.Code == "service_unavailable" {
			m.setStatus(statusWarn, "Telegram is temporarily unavailable. Press r to retry.")
		} else {
			m.setStatus(statusError, friendlyError(msg.err, "Could not check Telegram approval."))
		}
		return m, nil
	}
	switch msg.result.Status {
	case "pending":
		if m.stage == profileStage {
			m.setStatus(statusInfo, "Still waiting for Telegram approval. Enter checks again.")
			return m, nil
		}
		m.setStatus(statusInfo, "Waiting for Telegram approval…")
		return m, m.telegramTimer()
	case "restore_required":
		m.stage = restoreStage
		m.ticket = msg.result.RecoveryTicket
		m.clearStatus()
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
		m.setStatus(statusError, friendlyError(msg.err, "Could not send a code."))
		return m, nil
	}
	m.challenge = msg.id
	m.loginValues.code = ""
	m.setStatus(statusInfo, "Code sent. It expires in 10 minutes.")
	return m, m.showLoginForm(codeStage)
}

func (m Model) onEmailVerify(msg verifyResult) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		var apiErr *api.Error
		if errors.As(msg.err, &apiErr) && apiErr.Code == "profile_required" {
			m.setStatus(statusInfo, "New account: add a name and time zone to finish.")
			return m, m.showLoginForm(profileStage)
		}
		// The code or profile form stays open with its values for a retry.
		m.setStatus(statusError, friendlyError(msg.err, "Could not verify the code."))
		return m, nil
	}
	if msg.result.Status == "restore_required" {
		m.stage = restoreStage
		m.ticket = msg.result.RecoveryTicket
		m.loginValues.code = ""
		m.clearStatus()
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
			m.setStatus(statusError, "Telegram restore expired or was used. Esc, then 2 to restart.")
		} else {
			m.setStatus(statusError, friendlyError(msg.err, "Could not restore the account."))
		}
	} else {
		m.clearTelegram()
		return m, m.signedIn(msg.token)
	}
	return m, nil
}
