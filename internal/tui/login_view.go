package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) loginBody() string {
	switch m.stage {
	case emailStage:
		return "Sign in with email\n\nEmail\n" + m.inputs[emailInput].View() + "\n\n" + dimStyle.Render("Enter send code · Esc login options · Ctrl+C quit")
	case telegramStage:
		body := "Sign in with Telegram\n\nApprove access in your browser. This attempt expires in about 10 minutes.\n\n" + dimStyle.Render("r check now · o reopen browser · Esc cancel")
		if strings.Contains(m.message, "Could not open the browser") {
			body += "\n\nOpen manually: " + safeText(m.telegramURL)
		}
		return body
	case codeStage:
		return fmt.Sprintf("Code sent to %s\n\n8-digit code\n%s\n\n%s", safeText(m.email), m.inputs[codeInput].View(), dimStyle.Render("Enter verify · Esc change email · Ctrl+C quit"))
	case profileStage:
		return "Finish creating your account\n\nName\n" + m.inputs[usernameInput].View() + "\n\nTime zone (IANA)\n" + m.inputs[zoneInput].View() + "\n\n" + dimStyle.Render("Tab switch field · Enter continue · Esc cancel/back")
	case restoreStage:
		return "This account is scheduled for deletion.\nRestoring it keeps its projects and notes, but previously signed-in devices remain signed out.\n\nRestore this account? [y/N]"
	}
	return ""
}

func (m Model) loginView() string {
	logo := "SHORTLOG"
	switch {
	case m.width >= lipgloss.Width(largeLoginLogo)+4 && m.height >= 18:
		logo = largeLoginLogo
	case m.width >= lipgloss.Width(smallLoginLogo)+4 && m.height >= 12:
		logo = smallLoginLogo
	}
	if m.message != "" && m.height < 16 {
		logo = "SHORTLOG"
	}

	// Lip Gloss centers the entire composition within the terminal. Bubbles
	// owns the option rows, their selection, and keyboard navigation.
	heading := titleStyle.Render(logo)
	if m.height >= 16 {
		heading = lipgloss.JoinVertical(lipgloss.Center, heading, "", dimStyle.Render("A quiet place for your notes"))
	}
	help := "↑/↓ or j/k select · Enter continue · 1/2 choose · q quit"
	if m.width < 59 {
		help = "j/k select · Enter · 1/2 direct · q quit"
	}
	if m.width < 40 {
		help = "Enter select · 1/2 direct"
	}
	if m.width < 27 {
		help = "1/2 choose"
	}
	// The list's item strings have different lengths. Give its viewport a fixed
	// width before centering, rather than centering each rendered row separately.
	options := lipgloss.NewStyle().Width(m.loginOptions.Width()).Render(m.loginOptions.View())
	rows := []string{heading, "", options}
	if m.message == "" || m.height >= 16 {
		rows = append(rows, "", dimStyle.Render(help))
	}
	if m.message != "" {
		status := lipgloss.NewStyle().Width(min(54, max(1, m.width-4))).Render(errStyle.Render(m.message))
		rows = append(rows, "", status)
	}
	content := lipgloss.JoinVertical(lipgloss.Center, rows...)
	if lipgloss.Height(content) > m.height && m.message != "" {
		status := lipgloss.NewStyle().Width(min(54, max(1, m.width-4))).Render(errStyle.Render(m.message))
		content = lipgloss.JoinVertical(lipgloss.Center, options, status)
	}
	return lipgloss.Place(max(1, m.width), max(1, m.height), lipgloss.Center, lipgloss.Center, content)
}
