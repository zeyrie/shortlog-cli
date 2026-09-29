# Shortlog CLI

Go terminal client for the [Shortlog server](../shortlog-server/README.md). Email sign-in, Inbox note management, and project browsing are available.

## Run

Start the Shortlog server and its database, with email OTP delivery configured. Then run:

```sh
go run ./cmd/shortlog
```

The default API origin is `http://127.0.0.1:8080`. For another server, pass `-api-url https://your-host`. Use HTTPS for a remote server: email codes and session tokens must not travel over plaintext HTTP.

Enter your email, then the eight-digit code. New accounts are prompted for a name and IANA time zone. If an account is pending deletion, restoring it requires an explicit `y` confirmation. `Esc` goes back and `Ctrl+C` quits.

After sign-in, the CLI saves the session in your OS credential store (macOS Keychain, Windows Credential Manager, or a Linux Secret Service). It is scoped to the API origin and checked against `/v1/me` on launch. A 401 clears the expired saved session; a network failure does not. If the credential store is unavailable, the Inbox remains usable for this run, but you may need to sign in again next time. No token is written to a config file or printed in the terminal.

The Inbox starts with the newest 50 notes; press `m` to load each older page or `r` to refresh from the newest page. Use `j`/`k` or the arrow keys to select, `Enter` to read, `n` to create, `e` to edit, `v` to move, and `d` to permanently delete a note (confirmation required; there is no Trash). The multiline editor uses `Enter` for a new line and `Ctrl+S` to save; it shows a live character count and prompts before discarding unsaved changes with `Esc` or quitting with `Ctrl+C`. If a save cannot be confirmed, the draft stays open; check the note before retrying to avoid duplicates.

Press `p` to open the project list. In Active projects, use `a` to create, `x` to archive with confirmation, or `Enter` to open a project. Press `t` to switch between Active and Archived projects; in Archived, use `u` to unarchive or `Enter` to open a project **read-only**. Archived projects allow reading and pagination, but not creating, editing, moving, or deleting notes until unarchived. Press `i` or `Esc` from the project list to return to the Inbox.

Active projects have the same reading, editing, moving, deletion, pagination, and quick capture behavior as the Inbox; new notes saved there stay in the project. To move a note, press `v` from the list or reader, choose an active destination with `j`/`k` and `Enter`, or cancel with `Esc`. From a project, Inbox is the first destination. A failed move leaves the note visible in its current list; if the result is uncertain, refresh before retrying.

Press `s` from Inbox or Projects to manage sessions. The current device is marked. Use `j`/`k` to select a session, `x` to revoke the selected one, `a` to revoke all sessions, or `l` to log out of this device. Each action asks for confirmation. Revoking the current device, revoking all, or logging out clears the local saved credential and returns to sign-in. If a request cannot be confirmed, refresh the list with `r` before retrying.

Run tests with `go test ./...`.
