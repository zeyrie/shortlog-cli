package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"shortlog-cli/internal/api"
	"shortlog-cli/internal/config"
	"shortlog-cli/internal/session"
	"shortlog-cli/internal/tui"
)

const defaultAPIURL = "http://127.0.0.1:8080"

func main() {
	address := flag.String("api-url", "", "Shortlog API origin for this run (default: the saved server, or "+defaultAPIURL+")")
	demo := flag.Bool("demo", false, "open the workspace with sample data and no server")
	flag.Parse()
	if *demo {
		run(tui.NewDemo())
		return
	}

	settings, settingsErr := config.Default()
	var saved config.Config
	var loadErr error
	if settingsErr == nil {
		saved, loadErr = settings.Load()
	}
	client, warning, err := chooseServer(*address, saved, loadErr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	m := tui.New(client, session.New(client.Origin())).WithServers(servers{settings: settings, settingsErr: settingsErr})
	if warning != "" {
		m = m.WithWarning(warning)
	}
	run(m)
}

// chooseServer picks the server to open with: -api-url for this run, else
// the one saved from the app, else the local default. A saved address that
// is unreadable or invalid falls back to the default with a warning, so it
// can never lock the user out; an invalid -api-url is an error.
func chooseServer(flagValue string, saved config.Config, loadErr error) (*api.Client, string, error) {
	if flagValue != "" {
		client, err := api.NewClient(flagValue, nil)
		return client, "", err
	}
	warning := ""
	origin := defaultAPIURL
	switch {
	case loadErr != nil:
		warning = "Could not read the saved settings (" + loadErr.Error() + "); using the default server."
	case saved.APIURL != "":
		origin = saved.APIURL
	}
	client, err := api.NewClient(origin, nil)
	if err != nil {
		warning = "The saved server address is not valid; using the default. Press s to change it."
		client, err = api.NewClient(defaultAPIURL, nil)
	}
	return client, warning, err
}

func run(m tui.Model) {
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "TUI failed:", err)
		os.Exit(1)
	}
}

// servers connects the TUI to Shortlog servers: an API client and a session
// store per server, and the settings file for the one to open with.
type servers struct {
	settings    config.Store
	settingsErr error // no settings directory; switching still works for the run
}

func (servers) Connect(origin string) (tui.Client, tui.SessionStore, error) {
	client, err := api.NewClient(origin, nil)
	if err != nil {
		return nil, nil, err
	}
	return client, session.New(client.Origin()), nil
}

func (servers) Check(ctx context.Context, origin string) error {
	client, err := api.NewClient(origin, nil)
	if err != nil {
		return err
	}
	return client.Health(ctx)
}

func (s servers) Remember(origin string) error {
	if s.settingsErr != nil {
		return s.settingsErr
	}
	return s.settings.Save(config.Config{APIURL: origin})
}
