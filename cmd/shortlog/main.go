package main

import (
	"flag"
	"fmt"
	"os"

	"charm.land/bubbletea/v2"
	"shortlog-cli/internal/api"
	"shortlog-cli/internal/session"
	"shortlog-cli/internal/tui"
)

func main() {
	address := flag.String("api-url", "http://127.0.0.1:8080", "Shortlog API origin")
	demo := flag.Bool("demo", false, "open the workspace with sample data and no server")
	flag.Parse()
	if *demo {
		if _, err := tea.NewProgram(tui.NewDemo()).Run(); err != nil {
			fmt.Fprintln(os.Stderr, "TUI failed:", err)
			os.Exit(1)
		}
		return
	}
	client, err := api.NewClient(*address, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := tea.NewProgram(tui.New(client, session.New(client.Origin()))).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "TUI failed:", err)
		os.Exit(1)
	}
}
