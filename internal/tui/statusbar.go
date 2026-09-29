package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// statusLevel ranks a status-line message. Info fades on its own; warnings and
// errors stay until the next key press, like Vim's message line.
type statusLevel int

const (
	statusInfo statusLevel = iota
	statusWarn
	statusError
)

const (
	statusInfoTTL      = 4 * time.Second
	statusHistoryLimit = 50
	// statusBeatInterval is one spinner frame. The beat runs at this pace even
	// when idle, so a spinner starts moving as soon as a request does; Bubble
	// Tea's renderer already wakes about 60 times a second, so this adds little.
	statusBeatInterval = 100 * time.Millisecond
)

const (
	statusGutter   = 1 // columns kept clear at each edge of the footer
	statusSpacing  = 2 // minimum gap between the message and the context
	statusFallback = "Working…"
)

var (
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	spinnerFrames = spinner.MiniDot.Frames
)

type statusEntry struct {
	level statusLevel
	text  string
	at    time.Time
}

// statusBeat drives the spinner and info expiry. It runs on its own clock,
// started once from Init, so no screen has to batch timers into its commands.
type statusBeat struct{}

// statusBar is the footer's message line plus a short history of past
// messages, kept for a later message-log view.
type statusBar struct {
	current statusEntry
	history []statusEntry
	frame   int
}

func (s *statusBar) set(level statusLevel, text string, now time.Time) {
	s.current = statusEntry{level: level, text: safeText(text), at: now}
	s.history = append(s.history, s.current)
	if over := len(s.history) - statusHistoryLimit; over > 0 {
		s.history = append([]statusEntry(nil), s.history[over:]...)
	}
}

func (s *statusBar) clear() { s.current = statusEntry{} }

// dismiss drops a warning or error once the user acts again.
func (s *statusBar) dismiss() {
	if s.current.level != statusInfo {
		s.clear()
	}
}

// beat advances the spinner, expires stale info when idle, and schedules the
// next beat. The spinner turns even when nothing is busy, because some views
// show one while waiting on someone else, such as Telegram approval.
func (s *statusBar) beat(busy bool, now time.Time) tea.Cmd {
	s.frame++
	if !busy && s.current.level == statusInfo && s.current.text != "" && now.Sub(s.current.at) >= statusInfoTTL {
		s.clear()
	}
	return statusTick()
}

// spinner is the current spinner frame, for views that show their own.
func (s statusBar) spinner() string {
	return menuRailStyle.Render(spinnerFrames[s.frame%len(spinnerFrames)])
}

func statusTick() tea.Cmd {
	return tea.Tick(statusBeatInterval, func(time.Time) tea.Msg { return statusBeat{} })
}

// view renders the message line: a spinner or level marker, the message, and
// a right-aligned context segment that gives way first when space runs out.
func (s statusBar) view(width int, busy bool, context string) string {
	width = max(1, width-2*statusGutter)
	text, level := s.current.text, s.current.level
	if busy && text == "" {
		text = statusFallback
	}
	text = strings.ReplaceAll(text, "\n", " ")
	marker, style := "", dimStyle
	switch {
	case busy:
		marker, style = menuRailStyle.Render(spinnerFrames[s.frame%len(spinnerFrames)])+" ", lipgloss.NewStyle()
	case level == statusError:
		marker, style = errStyle.Render("✗")+" ", errStyle
	case level == statusWarn:
		marker, style = warnStyle.Render("!")+" ", warnStyle
	}
	room := width - lipgloss.Width(marker)
	left := ""
	if text != "" {
		left = marker + style.Render(ansi.Truncate(text, max(1, room), "…"))
	}
	line := left
	if context != "" {
		if gap := width - lipgloss.Width(left) - lipgloss.Width(context); gap >= statusSpacing {
			line = left + strings.Repeat(" ", gap) + dimStyle.Render(context)
		}
	}
	return lipgloss.NewStyle().PaddingLeft(statusGutter).Render(line)
}
