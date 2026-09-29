package tui

import "testing"

func TestTelegramBrowserURLAllowlist(t *testing.T) {
	for _, tc := range []struct {
		address string
		valid   bool
	}{
		{"https://oauth.telegram.org/auth?state=abc", true},
		{"http://oauth.telegram.org/auth", false},
		{"https://oauth.telegram.org.evil.example/auth", false},
		{"https://user@oauth.telegram.org/auth", false},
		{"javascript:alert(1)", false},
		{"https://oauth.telegram.org:443/auth", false},
	} {
		if validTelegramURL(tc.address) != tc.valid {
			t.Errorf("URL allowlist mismatch for %q", tc.address)
		}
	}
}
