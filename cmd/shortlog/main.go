package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbletea"
	"shortlog-cli/internal/api"
	"shortlog-cli/internal/session"
	"shortlog-cli/internal/tui"
)

func main() {
	address := flag.String("api-url", "http://127.0.0.1:8080", "Shortlog API origin")
	flag.Parse()
	client, err := api.NewClient(*address, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := tea.NewProgram(tui.New(client, session.New(client.Origin())), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "TUI failed:", err)
		os.Exit(1)
	}
}
