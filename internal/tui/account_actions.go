package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"shortlog-cli/internal/api"
)

// accountAPI is the part of the Shortlog API the account page changes things
// with.
type accountAPI interface {
	UpdateProfile(context.Context, string, string, string) (api.Account, error)
	RequestAccountDeletion(context.Context, string) (time.Time, error)
	Logout(context.Context, string) error
	RevokeSession(context.Context, string, string) error
	RevokeAllSessions(context.Context, string) error
}

// Results of the account page's changes.
type (
	profileUpdated struct {
		token   string
		account api.Account
		err     error
	}
	sessionEnded struct {
		token   string
		action  actionKind // revokeSessionAction, revokeAllAction, or signOutAction
		id      string     // the revoked session, for revokeSessionAction
		current bool       // whether that ends this device's session
		err     error
	}
	accountDeletionRequested struct {
		token    string
		deadline time.Time
		err      error
	}
)

// signOut is how the workspace asks the root to end the session locally:
// clear the saved credential and show sign-in with notice.
type signOut struct{ notice string }

// onAccountPage reports whether account actions apply: the account panel,
// or the main panel while it shows the account page.
func (w workspaceModel) onAccountPage() bool {
	return w.accountState.loaded && (w.focus == focusAccount || (w.focus == focusMain && w.back == focusAccount))
}

func (w *workspaceModel) startEditProfile() {
	a := w.account.account
	w.popup = newForm("Edit profile", "", "save", false, []popupField{
		newField("Name", "Your name", a.Username, 80, validateName),
		newField("IANA time zone", "e.g. Europe/London", a.TimeZone, 64, validateZone),
	}, pendingAction{kind: editProfileAction})
	w.popup.setSize(w.width, w.height)
}

// startRevoke asks before revoking the selected session, warning when it is
// this device's.
func (w *workspaceModel) startRevoke() {
	s, ok := w.account.selectedSession()
	if !ok {
		return
	}
	message := "Revoke “" + sessionName(s) + "”? That device will be signed out."
	if s.Current {
		message = "Revoke this device's session? You will be signed out here."
	}
	w.popup = newConfirm("Revoke session", message, "revoke", true, pendingAction{kind: revokeSessionAction, id: s.ID, name: sessionName(s), current: s.Current})
}

func (w *workspaceModel) startRevokeAll() {
	w.popup = newConfirm("Revoke all sessions", "Revoke every session? All devices, including this one, will be signed out.", "revoke all", true, pendingAction{kind: revokeAllAction})
}

func (w *workspaceModel) startSignOut() {
	w.popup = newConfirm("Sign out", "Sign out of this device? Its session is revoked on the server.", "sign out", false, pendingAction{kind: signOutAction})
}

// startDeleteAccount explains what deletion does and asks for DELETE, typed
// exactly, before requesting it.
func (w *workspaceModel) startDeleteAccount() {
	confirm := newField("Confirm", "Type DELETE", "", 6, func(v string) error {
		if v != "DELETE" {
			return errors.New("type DELETE exactly to confirm")
		}
		return nil
	})
	message := "Delete your account? Every device is signed out at once, and your projects and notes become inaccessible. " +
		"You can restore the account by signing in with the same identity within 30 days; after that, everything is erased for good.\n\nType DELETE to confirm."
	w.popup = newForm("Delete account", message, "delete account", true, []popupField{confirm}, pendingAction{kind: deleteAccountAction})
	w.popup.setSize(w.width, w.height)
}

// acceptAccountAction starts an accepted account popup's request.
func (w *workspaceModel) acceptAccountAction(action pendingAction) tea.Cmd {
	client, token := w.api, w.token
	switch action.kind {
	case editProfileAction:
		values := w.popup.values()
		name, zone := strings.TrimSpace(values[0]), values[1]
		return func() tea.Msg {
			account, err := client.UpdateProfile(context.Background(), token, name, zone)
			return profileUpdated{token: token, account: account, err: err}
		}
	case revokeSessionAction:
		return func() tea.Msg {
			err := client.RevokeSession(context.Background(), token, action.id)
			return sessionEnded{token: token, action: action.kind, id: action.id, current: action.current, err: err}
		}
	case revokeAllAction:
		return func() tea.Msg {
			return sessionEnded{token: token, action: action.kind, current: true, err: client.RevokeAllSessions(context.Background(), token)}
		}
	case signOutAction:
		return func() tea.Msg {
			return sessionEnded{token: token, action: action.kind, current: true, err: client.Logout(context.Background(), token)}
		}
	case deleteAccountAction:
		return func() tea.Msg {
			deadline, err := client.RequestAccountDeletion(context.Background(), token)
			return accountDeletionRequested{token: token, deadline: deadline, err: err}
		}
	}
	return nil
}

func (w workspaceModel) onProfileUpdated(msg profileUpdated) workspaceModel {
	if w.popup == nil || w.popup.action.kind != editProfileAction {
		return w
	}
	p := w.popup
	p.busy = false
	if msg.err != nil {
		switch {
		case w.writeFailed(msg.err, nil):
		case apiCode(msg.err) == "invalid_request" || apiCode(msg.err) == "validation_failed":
			p.err = "The server rejected these details. Check the name and time zone."
		default:
			p.err = "Update not confirmed. Check your account with r before retrying."
		}
		return w
	}
	w.popup = nil
	w.account.account = msg.account
	w.note = &statusNote{level: statusInfo, text: "Profile updated."}
	return w
}

func (w workspaceModel) onSessionEnded(msg sessionEnded) workspaceModel {
	w.popup = nil
	if msg.err != nil {
		if !w.writeFailed(msg.err, nil) {
			w.note = &statusNote{level: statusError, text: "Action not confirmed. Refresh with r before retrying."}
		}
		return w
	}
	if msg.current {
		w.signOut = &signOut{notice: "Signed out."}
		return w
	}
	w.account.sessions = slices.DeleteFunc(slices.Clone(w.account.sessions), func(s api.Session) bool { return s.ID == msg.id })
	w.account.list.clamp(len(w.account.sessions), len(w.account.sessions))
	w.note = &statusNote{level: statusInfo, text: "Session revoked."}
	return w
}

func (w workspaceModel) onAccountDeletionRequested(msg accountDeletionRequested) workspaceModel {
	if w.popup == nil || w.popup.action.kind != deleteAccountAction {
		return w
	}
	if msg.err != nil {
		w.popup.busy = false
		if !w.writeFailed(msg.err, nil) {
			w.popup.err = "Deletion not confirmed. Check your account before trying again."
		}
		return w
	}
	w.popup = nil
	w.signOut = &signOut{notice: "Deletion scheduled for " + msg.deadline.Local().Format("Jan 02, 2006 15:04") +
		". All devices were signed out. Sign in with the same identity before then to restore your account."}
	return w
}

func sessionName(s api.Session) string {
	switch {
	case s.DeviceLabel != "":
		return safeText(s.DeviceLabel)
	case s.UserAgent != "":
		return safeText(s.UserAgent)
	}
	return "Unknown device"
}
