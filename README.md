# Shortlog CLI

Go terminal client for the [Shortlog server](../shortlog-server/README.md). The first milestone implements email sign-in; notes and projects are not yet available.

## Run

Start the Shortlog server and its database, with email OTP delivery configured. Then run:

```sh
go run ./cmd/shortlog
```

The default API origin is `http://127.0.0.1:8080`. For another server, pass `-api-url https://your-host`. Use HTTPS for a remote server: email codes and session tokens must not travel over plaintext HTTP.

Enter your email, then the eight-digit code. New accounts are prompted for a name and IANA time zone. If an account is pending deletion, restoring it requires an explicit `y` confirmation. `Esc` goes back and `Ctrl+C` quits. The token remains only in memory and is **not** persisted; exiting signs you out of the CLI until credential storage and authenticated screens are implemented.

Run tests with `go test ./...`.
