package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var stripStatusANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestStatusInfoExpiresOnlyWhenIdle(t *testing.T) {
	now := time.Now()
	var s statusBar
	s.set(statusInfo, "Note saved", now)
	s.beat(true, now.Add(statusInfoTTL+time.Second))
	if s.current.text == "" {
		t.Fatal("info expired while a request was running")
	}
	s.beat(false, now.Add(statusInfoTTL-time.Second))
	if s.current.text == "" {
		t.Fatal("info expired early")
	}
	s.beat(false, now.Add(statusInfoTTL))
	if s.current.text != "" {
		t.Fatal("info did not expire")
	}
}

func TestStatusErrorsPersistUntilKeyPress(t *testing.T) {
	now := time.Now()
	var s statusBar
	s.set(statusError, "Could not send a code.", now)
	s.beat(false, now.Add(time.Hour))
	if s.current.text == "" {
		t.Fatal("error expired on a timer")
	}
	m := New(&fakeAPI{}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	m.status = s
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if next.(Model).status.current.text != "" {
		t.Fatal("key press did not dismiss the error")
	}
	if len(next.(Model).status.history) != 1 {
		t.Fatal("dismissing a message dropped it from history")
	}
}

func TestStatusHistoryIsBounded(t *testing.T) {
	var s statusBar
	for i := 0; i < statusHistoryLimit+10; i++ {
		s.set(statusInfo, fmt.Sprint(i), time.Now())
	}
	if len(s.history) != statusHistoryLimit || s.history[0].text != "10" {
		t.Fatalf("history len %d, oldest %q", len(s.history), s.history[0].text)
	}
}

func TestStatusBeatIsFastOnlyWhileBusy(t *testing.T) {
	var s statusBar
	if s.beat(true, time.Now()) == nil || s.frame != 1 {
		t.Fatal("busy beat did not advance the spinner")
	}
	s.beat(false, time.Now())
	if s.frame != 1 {
		t.Fatal("idle beat advanced the spinner")
	}
}

func TestStatusLineFitsAndContextYields(t *testing.T) {
	var s statusBar
	s.set(statusError, "Could not start Telegram sign-in. Check your connection and try again.", time.Now())
	for _, width := range []int{80, 40, 12} {
		line := stripStatusANSI.ReplaceAllString(s.view(width, false, "127.0.0.1:8080"), "")
		if lipgloss.Width(line) > width {
			t.Errorf("width %d: status line overflows: %q", width, line)
		}
		if !strings.HasPrefix(strings.TrimSpace(line), "✗") {
			t.Errorf("width %d: error marker missing: %q", width, line)
		}
	}
	if line := s.view(40, false, "127.0.0.1:8080"); strings.Contains(line, "127.0.0.1") {
		t.Error("context kept although the message had no room")
	}
	s.set(statusInfo, "Code sent.", time.Now())
	if line := s.view(80, false, "127.0.0.1:8080"); !strings.Contains(line, "127.0.0.1:8080") {
		t.Error("context missing beside a short message")
	}
	s.clear()
	if line := stripStatusANSI.ReplaceAllString(s.view(80, true, ""), ""); !strings.Contains(line, statusFallback) {
		t.Errorf("busy line without a message: %q", line)
	}
}

func TestFooterFitsEveryScreen(t *testing.T) {
	for s := startupStage; s <= telegramStage; s++ {
		for _, size := range []struct{ width, height int }{{80, 24}, {42, 16}, {20, 10}, {20, 6}} {
			m := New(&fakeAPI{}, &fakeStore{})
			m.stage, m.busy = s, false
			m.width, m.height = size.width, size.height
			footer := stripStatusANSI.ReplaceAllString(m.footer(), "")
			if lipgloss.Height(footer) != m.footerHeight() {
				t.Errorf("stage %d %dx%d: footer is %d rows, want %d", s, size.width, size.height, lipgloss.Height(footer), m.footerHeight())
			}
			for _, line := range strings.Split(footer, "\n") {
				if lipgloss.Width(line) > size.width {
					t.Errorf("stage %d %dx%d: footer overflows: %q", s, size.width, size.height, line)
				}
			}
			if m.footerHeight() == 2 && len(m.shortcuts()) == 0 {
				t.Errorf("stage %d: no shortcuts", s)
			}
		}
	}
}

func TestLoginErrorsGoToStatusLine(t *testing.T) {
	m := New(&fakeAPI{telegramStartErr: fmt.Errorf("offline")}, &fakeStore{})
	m.stage, m.busy = loginStage, false
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if m.status.current.text != "Starting Telegram sign-in…" {
		t.Fatalf("busy status = %q", m.status.current.text)
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.status.current.level != statusError || m.message != "" {
		t.Fatalf("failure: level %d, text %q, body message %q", m.status.current.level, m.status.current.text, m.message)
	}
	lines := strings.Split(stripStatusANSI.ReplaceAllString(m.View(), ""), "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "Could not start Telegram") {
		t.Fatalf("error not on the last row: %q", last)
	}
}
