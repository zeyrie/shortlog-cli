# Shortlog CLI

Go terminal client for the [Shortlog server](../shortlog-server/README.md): email and Telegram sign-in, and a lazygit-style workspace for your projects and notes.

## Run

Start the Shortlog server and its database, with email OTP delivery configured. Then run:

```sh
go run ./cmd/shortlog
```

The default API origin is `http://127.0.0.1:8080`. For another server, pass `-api-url https://your-host`. Use HTTPS for a remote server: email codes and session tokens must not travel over plaintext HTTP.

Every screen ends with a two-line footer: the first line lists the shortcuts for the current screen (trailing ones are dropped on narrow terminals), and the second is a status line for progress and messages, with the signed-in name and server on the right. Status messages fade after a few seconds; warnings and errors stay until you press a key. Prompts and important notices, such as a scheduled deletion date, stay in the main area.

Colours follow your terminal: the CLI asks the terminal for its background and uses a light or dark palette to match.

Sign-in opens a full-screen ShortLog welcome screen. Choose Email or Telegram with `j`/`k` or the arrow keys and `Enter`, or press `1` for Email or `2` for Telegram directly. The block-letter wordmark switches to a compact one on narrow terminals. The email path uses inline forms: type your address and press `Enter`, then the eight-digit code (pasting it as `1234 5678` or `1234-5678` works); the new-account step adds name and IANA time zone fields, with `Tab` and `Shift+Tab` moving between them. Invalid entries are flagged inline and a failed request keeps what you typed. Or approve Telegram sign-in in your browser. While you approve, the screen shows the time left on the attempt and checks approval automatically; press `r` to check immediately, `o` to reopen the browser, `c` to copy the link (for example, to approve on another device), or `Esc` to cancel. If the browser fails to open, the link is shown below the card; in terminals that support hyperlinks it is clickable. Telegram must be configured on the server; it signs in with a Telegram identity and does **not** merge accounts with matching email or name. If an account is pending deletion, restoring it requires an explicit `y` confirmation; `n` keeps the deletion. `Esc` returns to the previous sign-in step or provider selection, and `Ctrl+C` quits.

After sign-in, the CLI saves the session in your OS credential store (macOS Keychain, Windows Credential Manager, or a Linux Secret Service). It is scoped to the API origin and checked against `/v1/me` on launch. A 401 clears the expired saved session; a network failure does not. If the credential store is unavailable, the workspace remains usable for this run, but you may need to sign in again next time. No token is written to a config file or printed in the terminal.

## Workspace

After sign-in the workspace opens, laid out like lazygit: on the left, `[1]` Account, `[2]` Projects (with Active and Archived tabs), and `[3]` Notes (with the Inbox and the selected project as tabs); on the right, `[0]` shows the selected note. Move between panels with `0`–`3` or `Tab`, within a panel with `j`/`k` or the arrow keys (`PgUp`/`PgDn` page), and switch a panel's tabs with `[` and `]`. `Enter` opens: a project's notes, a note in the reader, or your account page from `[1]`; `Esc` goes back. Selecting a project loads its notes, and older notes load automatically as you near the end of a list. `r` reloads the focused panel; if something fails to load, the panel says so and `r` retries. On terminals narrower than 70 columns, one column shows at a time.

The workspace is being built in steps and is read-only for now. Creating, editing, moving, and deleting notes, managing projects, and the account actions (editing your profile, revoking sessions, signing out, and deleting your account) return in the next steps.

To try the workspace without a server, run `go run ./cmd/shortlog -demo`. It opens with sample data; nothing is sent anywhere.

Run tests with `go test ./...`.
