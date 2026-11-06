package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"strings"
	"testing"
)

func key(m Model, t *testing.T, s string) Model {
	t.Helper()
	typ := tea.KeyRunes
	switch s {
	case "enter":
		typ = tea.KeyEnter
	case "esc":
		typ = tea.KeyEsc
	case "backspace":
		typ = tea.KeyBackspace
	case "ctrl+n":
		typ = tea.KeyCtrlN
	case "ctrl+p":
		typ = tea.KeyCtrlP
	case "ctrl+s":
		typ = tea.KeyCtrlS
	case "left":
		typ = tea.KeyLeft
	}
	msg := tea.KeyMsg{Type: typ}
	if typ == tea.KeyRunes {
		msg.Runes = []rune(s)
	}
	m, _ = updateModel(t, m, msg)
	return m
}
func TestOutputSearchInterceptsActions(t *testing.T) {
	b := &fakeBackend{}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m.sessions[0].Status = monitor.StatusPermission
	m = key(m, t, "/")
	m = key(m, t, "ynaxq")
	if b.sent != "" || b.killed != "" || m.confirmKill != "" || m.quitting {
		t.Fatal("query invoked a session action")
	}
	if m.search.query != "ynaxq" {
		t.Fatalf("query=%q", m.search.query)
	}
	m = key(m, t, "esc")
	if m.search.active() {
		t.Fatal("cancel left search active")
	}
}
func TestOutputSearchCaptureAndNavigation(t *testing.T) {
	m := NewModel()
	m.viewport.Height = 2
	m.sessions = []monitor.Session{testSession("%1")}
	m.sessions[0].Status = monitor.StatusPermission
	m.sessions[0].Content = "\x1b[31mneedle\x1b[0m\n" + strings.Repeat("other\n", 20) + "needle\nlast"
	m = key(m, t, "/")
	m = key(m, t, "needle")
	m = key(m, t, "enter")
	if len(m.search.matches) != 2 || m.viewport.YOffset != 0 {
		t.Fatalf("search=%+v offset=%d", m.search, m.viewport.YOffset)
	}
	m = key(m, t, "ctrl+n")
	if m.viewport.YOffset != 21 || m.cursor != 0 {
		t.Fatalf("offset=%d cursor=%d", m.viewport.YOffset, m.cursor)
	}
	m = key(m, t, "ctrl+n")
	if m.search.current != 0 {
		t.Fatal("next did not wrap")
	}
	m = key(m, t, "ctrl+p")
	if m.search.current != 1 {
		t.Fatal("previous did not wrap")
	}
	m = key(m, t, "esc")
	if m.search.active() || !strings.Contains(m.contentPaneText(), "PERMISSION REQUEST") {
		t.Fatal("escape did not restore permission panel")
	}
}
func TestOutputSearchUnicodeAndRefresh(t *testing.T) {
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1"), testSession("%2")}
	m = key(m, t, "/")
	m = key(m, t, "猫犬")
	m = key(m, t, "left")
	m = key(m, t, "backspace")
	if m.search.query != "犬" {
		t.Fatalf("query=%q", m.search.query)
	}
	m = key(m, t, "enter")
	first := testSession("%1")
	first.Content = "犬"
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{testSession("%2"), first}})
	if m.sessions[m.cursor].Pane.PaneID != "%1" || len(m.search.matches) != 1 {
		t.Fatalf("selection or search lost: %+v", m.search)
	}
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{testSession("%2")}})
	if m.search.active() {
		t.Fatal("search followed a removed pane")
	}
}

func TestOutputSearchNoMatchCancelAndKill(t *testing.T) {
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1")}
	m = key(m, t, "/")
	m = key(m, t, "absent")
	m = key(m, t, "enter")
	if len(m.search.matches) != 0 || !strings.Contains(m.searchHelp(), "0/0") {
		t.Fatal("missing no-match feedback")
	}
	m = key(m, t, "ctrl+n")
	m = key(m, t, "ctrl+p")
	m = key(m, t, "/")
	m = key(m, t, "new")
	m = key(m, t, "esc")
	if m.search.query != "absent" {
		t.Fatal("cancel did not restore query")
	}
	m = key(m, t, "x")
	if m.search.active() || !strings.Contains(m.contentPaneText(), "KILL PANE?") {
		t.Fatal("search obscured kill confirmation")
	}
}
