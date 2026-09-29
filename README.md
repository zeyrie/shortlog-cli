# Shortlog CLI

Go terminal client for the [Shortlog server](../shortlog-server/README.md). Email and Telegram sign-in, Inbox note management, and project browsing are available.

## Run

Start the Shortlog server and its database, with email OTP delivery configured. Then run:

```sh
go run ./cmd/shortlog
```

The default API origin is `http://127.0.0.1:8080`. For another server, pass `-api-url https://your-host`. Use HTTPS for a remote server: email codes and session tokens must not travel over plaintext HTTP.

Every screen ends with a two-line footer: the first line lists the shortcuts for the current screen (trailing ones are dropped on narrow terminals), and the second is a status line for progress and messages, with the signed-in name and server on the right. Status messages fade after a few seconds; warnings and errors stay until you press a key. Prompts and important notices, such as a scheduled deletion date, stay in the main area.

Colours follow your terminal: the CLI asks the terminal for its background and uses a light or dark palette to match.

Sign-in opens a full-screen ShortLog welcome screen. Choose Email or Telegram with `j`/`k` or the arrow keys and `Enter`, or press `1` for Email or `2` for Telegram directly. The block-letter wordmark switches to a compact one on narrow terminals. The email path uses inline forms: type your address and press `Enter`, then the eight-digit code (pasting it as `1234 5678` or `1234-5678` works); the new-account step adds name and IANA time zone fields, with `Tab` and `Shift+Tab` moving between them. Invalid entries are flagged inline and a failed request keeps what you typed. Or approve Telegram sign-in in your browser. While you approve, the screen shows the time left on the attempt and checks approval automatically; press `r` to check immediately, `o` to reopen the browser, `c` to copy the link (for example, to approve on another device), or `Esc` to cancel. If the browser fails to open, the link is shown below the card; in terminals that support hyperlinks it is clickable. Telegram must be configured on the server; it signs in with a Telegram identity and does **not** merge accounts with matching email or name. If an account is pending deletion, restoring it requires an explicit `y` confirmation; `n` keeps the deletion. `Esc` returns to the previous sign-in step or provider selection, and `Ctrl+C` quits.

After sign-in, the CLI saves the session in your OS credential store (macOS Keychain, Windows Credential Manager, or a Linux Secret Service). It is scoped to the API origin and checked against `/v1/me` on launch. A 401 clears the expired saved session; a network failure does not. If the credential store is unavailable, the Inbox remains usable for this run, but you may need to sign in again next time. No token is written to a config file or printed in the terminal.

The Inbox starts with the newest 50 notes; press `m` to load each older page or `r` to refresh from the newest page. Use `j`/`k` or the arrow keys to select, `Enter` to read, `n` to create, `e` to edit, `v` to move, and `d` to permanently delete a note (confirmation required; there is no Trash). The multiline editor uses `Enter` for a new line and `Ctrl+S` to save; it shows a live character count and prompts before discarding unsaved changes with `Esc` or quitting with `Ctrl+C`. If a save cannot be confirmed, the draft stays open; check the note before retrying to avoid duplicates.

Press `p` to open the project list. In Active projects, use `a` to create, `x` to archive with confirmation, or `Enter` to open a project. Press `t` to switch between Active and Archived projects; in Archived, use `u` to unarchive or `Enter` to open a project **read-only**. Archived projects allow reading and pagination, but not creating, editing, moving, or deleting notes until unarchived. Press `i` or `Esc` from the project list to return to the Inbox.

Active projects have the same reading, editing, moving, deletion, pagination, and quick capture behavior as the Inbox; new notes saved there stay in the project. To move a note, press `v` from the list or reader, choose an active destination with `j`/`k` and `Enter`, or cancel with `Esc`. From a project, Inbox is the first destination. A failed move leaves the note visible in its current list; if the result is uncertain, refresh before retrying.

Press `s` from Inbox or Projects to manage sessions. The current device is marked. Use `j`/`k` to select a session, `x` to revoke the selected one, `a` to revoke all sessions, or `l` to log out of this device. Each action asks for confirmation. Revoking the current device, revoking all, or logging out clears the local saved credential and returns to sign-in. If a request cannot be confirmed, refresh the list with `r` before retrying.

Press `g` from Inbox or Projects for account settings. Use `e` to edit your name and IANA time zone (`Tab` switches fields, `Enter` saves, and `Esc` asks before discarding changes). Use `d` to request account deletion; the screen explains the consequences and requires typing `DELETE` exactly. When accepted, all sessions are revoked immediately and the account can be restored by signing in with the same identity before the displayed 30-day deadline. After that deadline, the server's scheduled purge permanently erases the account and its data. If deletion cannot be confirmed, check your account status before retrying.

Run tests with `go test ./...`.
