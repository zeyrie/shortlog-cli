package tui

import (
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"shortlog-cli/internal/api"
)

func (l loginModel) onTelegramStart(msg telegramStartResult) (loginModel, tea.Cmd) {
	l.busy = false
	if msg.err != nil {
		l.clearTelegram()
		l.step = l.telegramBack
		var apiErr *api.Error
		if errors.As(msg.err, &apiErr) && apiErr.Code == "service_unavailable" {
			l.setStatus(statusError, "Telegram sign-in unavailable on this server. Use email.")
		} else {
			l.setStatus(statusError, friendlyError(msg.err, "Could not start Telegram sign-in."))
		}
		return l, nil
	}
	if !validTelegramURL(msg.start.AuthorizationURL) {
		l.clearTelegram()
		l.step = l.telegramBack
		l.setStatus(statusError, "Server sent an unsafe Telegram URL; sign-in cancelled.")
		return l, nil
	}
	l.telegramLogin = true
	l.telegramAttempt = msg.start.AttemptID
	l.telegramSecret = msg.start.PollSecret
	l.telegramURL = msg.start.AuthorizationURL
	l.telegramExpires = time.Now().Add(10 * time.Minute)
	l.step, l.form = telegramStep, nil
	l.clearStatus() // the card shows the waiting state
	id, address, opener := l.telegramAttempt, l.telegramURL, l.openBrowser
	return l, tea.Batch(l.telegramTimer(), func() tea.Msg {
		return telegramBrowserResult{id, opener(address)}
	})
}

func (l loginModel) onTelegramBrowser(msg telegramBrowserResult) (loginModel, tea.Cmd) {
	if l.step == telegramStep && l.telegramAttempt == msg.attempt {
		l.telegramBrowserFailed = msg.err != nil
		if msg.err != nil {
			l.setStatus(statusError, "Could not open the browser. Open the link below, or press o.")
		}
	}
	return l, nil
}

func (l loginModel) onTelegramTick(msg telegramTick) (loginModel, tea.Cmd) {
	if l.step != telegramStep || l.telegramAttempt != msg.attempt || l.telegramGeneration != msg.generation || l.busy {
		return l, nil
	}
	return l, l.pollTelegram("", "")
}

func (l loginModel) onTelegramPoll(msg telegramPollResult) (loginModel, tea.Cmd) {
	if l.telegramAttempt != msg.attempt || !l.telegramLogin || (l.step != telegramStep && l.step != profileStep) {
		return l, nil
	}
	l.busy = false
	if msg.err != nil {
		var apiErr *api.Error
		switch {
		case errors.As(msg.err, &apiErr) && apiErr.Code == "profile_required":
			l.setStatus(statusInfo, "New Telegram account: add a name and time zone.")
			return l, l.showForm(profileStep)
		case errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request" && l.step == profileStep:
			l.setStatus(statusError, "Profile or attempt rejected. Check details, or Esc to restart.")
		case errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request":
			l.setStatus(statusError, "Telegram attempt expired. Esc, then 2 to start again.")
		case errors.As(msg.err, &apiErr) && apiErr.Code == "service_unavailable":
			l.setStatus(statusWarn, "Telegram is temporarily unavailable. Press r to retry.")
		default:
			l.setStatus(statusError, friendlyError(msg.err, "Could not check Telegram approval."))
		}
		return l, nil
	}
	switch msg.result.Status {
	case "pending":
		if l.step == profileStep {
			l.setStatus(statusInfo, "Still waiting for Telegram approval. Enter checks again.")
			return l, nil
		}
		return l, l.telegramTimer()
	case "restore_required":
		l.step, l.form = restoreStep, nil
		l.ticket = msg.result.RecoveryTicket
		l.clearStatus()
		return l, nil
	case "signed_in":
		l.signedIn(msg.result.Token)
	}
	return l, nil
}

func (l loginModel) onEmailStart(msg startResult) (loginModel, tea.Cmd) {
	l.busy = false
	if msg.err != nil {
		l.setStatus(statusError, friendlyError(msg.err, "Could not send a code."))
		return l, nil
	}
	l.challenge = msg.id
	l.values.code = ""
	l.setStatus(statusInfo, "Code sent. It expires in 10 minutes.")
	return l, l.showForm(codeStep)
}

func (l loginModel) onEmailVerify(msg verifyResult) (loginModel, tea.Cmd) {
	l.busy = false
	if msg.err != nil {
		var apiErr *api.Error
		if errors.As(msg.err, &apiErr) && apiErr.Code == "profile_required" {
			l.setStatus(statusInfo, "New account: add a name and time zone to finish.")
			return l, l.showForm(profileStep)
		}
		// The code or profile form stays open with its values for a retry.
		l.setStatus(statusError, friendlyError(msg.err, "Could not verify the code."))
		return l, nil
	}
	if msg.result.Status == "restore_required" {
		l.step, l.form = restoreStep, nil
		l.ticket = msg.result.RecoveryTicket
		l.values.code = ""
		l.clearStatus()
		return l, nil
	}
	l.signedIn(msg.result.Token)
	return l, nil
}

func (l loginModel) onRestore(msg restoreResult) (loginModel, tea.Cmd) {
	l.busy = false
	if msg.err != nil {
		var apiErr *api.Error
		if l.telegramLogin && errors.As(msg.err, &apiErr) && apiErr.Code == "invalid_request" {
			l.setStatus(statusError, "Telegram restore expired or was used. Esc, then 2 to restart.")
		} else {
			l.setStatus(statusError, friendlyError(msg.err, "Could not restore the account."))
		}
		return l, nil
	}
	l.signedIn(msg.token)
	return l, nil
}
