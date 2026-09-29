package tui

import (
	"errors"
	"net/url"
	"os/exec"
	"runtime"
)

// Only open the Telegram OAuth endpoint. Never pass an arbitrary server value
// to a shell or to a browser URL handler.
func validTelegramURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "oauth.telegram.org" && u.User == nil && u.Fragment == ""
}

func openTelegramBrowser(raw string) error {
	if !validTelegramURL(raw) {
		return errors.New("invalid Telegram authorization URL")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", raw).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", raw).Run()
	default:
		return exec.Command("xdg-open", raw).Run()
	}
}
