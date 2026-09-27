# Shortlog CLI

Go terminal client for the [Shortlog server](../shortlog-server/README.md). Email sign-in and Inbox note management are available; projects are not yet available.

## Run

Start the Shortlog server and its database, with email OTP delivery configured. Then run:

```sh
go run ./cmd/shortlog
```

The default API origin is `http://127.0.0.1:8080`. For another server, pass `-api-url https://your-host`. Use HTTPS for a remote server: email codes and session tokens must not travel over plaintext HTTP.

Enter your email, then the eight-digit code. New accounts are prompted for a name and IANA time zone. If an account is pending deletion, restoring it requires an explicit `y` confirmation. `Esc` goes back and `Ctrl+C` quits.

After sign-in, the CLI saves the session in your OS credential store (macOS Keychain, Windows Credential Manager, or a Linux Secret Service). It is scoped to the API origin and checked against `/v1/me` on launch. A 401 clears the expired saved session; a network failure does not. If the credential store is unavailable, the Inbox remains usable for this run, but you may need to sign in again next time. No token is written to a config file or printed in the terminal.

The Inbox starts with the newest 50 notes; press `m` to load each older page or `r` to refresh from the newest page. Use `j`/`k` or the arrow keys to select, `Enter` to read, `n` to create, `e` to edit, and `d` to permanently delete a note (confirmation required; there is no Trash). The multiline editor uses `Enter` for a new line and `Ctrl+S` to save; it shows a live character count and prompts before discarding unsaved changes with `Esc` or quitting with `Ctrl+C`. If a save cannot be confirmed, the draft stays open; check the note before retrying to avoid duplicates. Use `s` to remove the saved session and sign in with another email, or `q` to quit. Switching email removes the local credential but does not revoke the server-side device session. Projects and moving notes are not yet available in the TUI.

Run tests with `go test ./...`.
