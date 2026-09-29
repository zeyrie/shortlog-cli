# Shortlog CLI

Go terminal client for the [Shortlog server](../shortlog-server/README.md): email and Telegram sign-in, and a lazygit-style workspace for your projects and notes.

## Run

Start the Shortlog server and its database, with email OTP delivery configured. Then run:

```sh
go run ./cmd/shortlog
```

The CLI connects to `http://127.0.0.1:8080` until you choose another server. Press `s` on the sign-in screen, or on your account page, to change it: enter the address (for example `https://notes.example.com`), and the CLI checks that the server answers before switching; if it doesn't, pressing `Enter` again connects anyway. The choice is saved in `config.json` in your configuration directory (`~/Library/Application Support/shortlog` on macOS, `~/.config/shortlog` on Linux) and used next time. Each server keeps its own saved session, so switching back signs you straight in again. To use another server for one run without changing the saved one, pass `-api-url https://your-host`. Use HTTPS for a remote server: email codes and session tokens must not travel over plaintext HTTP, and the CLI warns when you connect to one that way.

Every screen ends with a two-line footer: the first line lists the shortcuts for the current screen (trailing ones are dropped on narrow terminals), and the second is a status line for progress and messages, with the signed-in name and server on the right. Status messages fade after a few seconds; warnings and errors stay until you press a key. Prompts and important notices, such as a scheduled deletion date, stay in the main area.

Colours follow your terminal: the CLI asks the terminal for its background and uses a light or dark palette to match.

Sign-in opens a full-screen ShortLog welcome screen. Choose Email or Telegram with `j`/`k` or the arrow keys and `Enter`, or press `1` for Email or `2` for Telegram directly. The block-letter wordmark switches to a compact one on narrow terminals. The email path uses inline forms: type your address and press `Enter`, then the eight-digit code (pasting it as `1234 5678` or `1234-5678` works); the new-account step adds name and IANA time zone fields, with `Tab` and `Shift+Tab` moving between them. Invalid entries are flagged inline and a failed request keeps what you typed. Or approve Telegram sign-in in your browser. While you approve, the screen shows the time left on the attempt and checks approval automatically; press `r` to check immediately, `o` to reopen the browser, `c` to copy the link (for example, to approve on another device), or `Esc` to cancel. If the browser fails to open, the link is shown below the card; in terminals that support hyperlinks it is clickable. Telegram must be configured on the server; it signs in with a Telegram identity and does **not** merge accounts with matching email or name. If an account is pending deletion, restoring it requires an explicit `y` confirmation; `n` keeps the deletion. `Esc` returns to the previous sign-in step or provider selection, and `Ctrl+C` quits.

After sign-in, the CLI saves the session in your OS credential store (macOS Keychain, Windows Credential Manager, or a Linux Secret Service). It is scoped to the API origin and checked against `/v1/me` on launch. A 401 clears the expired saved session; a network failure does not. If the credential store is unavailable, the workspace remains usable for this run, but you may need to sign in again next time. No token is written to a config file or printed in the terminal.

## Workspace

After sign-in the workspace opens, laid out like lazygit: on the left, `[1]` Account, `[2]` Projects (with Active and Archived tabs), and `[3]` Notes (with the Inbox and the selected project as tabs); on the right, `[0]` shows the selected note. Move between panels with `0`–`3` or `Tab`, within a panel with `j`/`k` or the arrow keys (`PgUp`/`PgDn` page), and switch a panel's tabs with `[` and `]`. `Enter` opens: a project's notes, a note in the reader, or your account page from `[1]`; `Esc` goes back. Selecting a project loads its notes, and older notes load automatically as you near the end of a list. `r` reloads the focused panel; if something fails to load, the panel says so and `r` retries. On terminals narrower than 70 columns, one column shows at a time.

In the Notes panel (or with a note open), `n` writes a new note in a popup, into whichever tab is showing: the Inbox or the project. `Ctrl+S` saves it; `Esc` asks before throwing away what you wrote. `e` edits the note in place in the main panel, `v` moves it to the Inbox or another active project, and `d` deletes it permanently after you confirm (there is no Trash). In the Projects panel, `a` creates a project, `x` archives the selected one after you confirm, and `u` unarchives it from the Archived tab. Archived projects are read-only: their notes can be read, but not added to, edited, moved, or deleted until the project is unarchived.

If a save cannot be confirmed (for example, the connection drops), your text stays in the popup or editor; check the note list before retrying to avoid a duplicate. If the session has expired, the CLI signs you out and reopens your unsaved text once you sign in again. `Ctrl+C` asks before quitting if you have unsaved text.

Focus `[1]` (or press `Enter` there) for your account page. `e` edits your name and IANA time zone (`Tab` switches fields, `Enter` saves). The page lists your sessions with the current device marked: pick one with `j`/`k` and press `x` to revoke it, or `a` to revoke them all; `l` signs out of this device. Each asks for confirmation. Revoking this device's session, revoking all, or signing out clears the saved credential and returns to sign-in.

`D` requests account deletion. The page explains the consequences and requires typing `DELETE` exactly. When accepted, every session is revoked at once and the account can be restored by signing in with the same identity before the deadline shown, 30 days later. After that, the server's scheduled purge permanently erases the account and its data. If a change cannot be confirmed, check your account with `r` before retrying.

To try the workspace without a server, run `go run ./cmd/shortlog -demo`. It opens with sample data, and your changes last until you quit; nothing is sent anywhere.

Run tests with `go test ./...`.
