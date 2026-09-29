package main

import (
	"errors"
	"testing"

	"shortlog-cli/internal/config"
)

func TestChooseServer(t *testing.T) {
	for _, tc := range []struct {
		name, flag, saved string
		loadErr           error
		want              string
		warns, fails      bool
	}{
		{name: "default", want: defaultAPIURL},
		{name: "saved", saved: "https://notes.example", want: "https://notes.example"},
		{name: "flag beats saved", flag: "https://flag.example", saved: "https://notes.example", want: "https://flag.example"},
		{name: "invalid saved falls back", saved: "notes.example", want: defaultAPIURL, warns: true},
		{name: "unreadable settings fall back", loadErr: errors.New("bad json"), saved: "https://notes.example", want: defaultAPIURL, warns: true},
		{name: "invalid flag fails", flag: "notes.example", fails: true},
	} {
		client, warning, err := chooseServer(tc.flag, config.Config{APIURL: tc.saved}, tc.loadErr)
		switch {
		case tc.fails:
			if err == nil {
				t.Errorf("%s: expected an error", tc.name)
			}
		case err != nil || client.Origin() != tc.want || (warning != "") != tc.warns:
			t.Errorf("%s: origin %v, warning %q, err %v", tc.name, client, warning, err)
		}
	}
}
