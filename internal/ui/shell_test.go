package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
)

type shellBackend struct{ lifecycleFake }

func (b *shellBackend) CreateShellSession(directory string) (tmux.Pane, error) {
	b.directory = directory
	return tmux.Pane{PaneID: "%8", Shell: true, CurrentPath: directory}, b.err
}

func shellSession() monitor.Session {
	s := testSession("%8")
	s.Provider = monitor.ProviderUnknown
	s.Kind = monitor.SessionShell
	return s
}

func TestLaunchShellSession(t *testing.T) {
	b := &shellBackend{}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m = key(m, t, "c")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if !m.lifecycle.shell || !strings.Contains(m.lifecycleView(), "Shell (zsh)") {
		t.Fatal("shell choice unavailable")
	}
	m.lifecycle.text = t.TempDir()
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.lifecycle.busy {
		t.Fatal("shell launch must be asynchronous")
	}
	m, _ = updateModel(t, m, cmd())
	if b.directory == "" || len(m.sessions) != 1 || m.sessions[0].Kind != monitor.SessionShell || m.sessions[0].Provider != monitor.ProviderUnknown {
		t.Fatalf("shell launch state: %#v", m.sessions)
	}
}

func TestShellFilterAndComposer(t *testing.T) {
	b := &textBackend{}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1"), shellSession()}
	m.filters.applied.provider = len(filterProviders)
	m.applyFilters()
	if len(m.sessions) != 1 || !strings.Contains(m.filterSummary(), "Shell") || !strings.Contains(sessionLabel(m.sessions[0]), "Shell") {
		t.Fatal("shell filter or label missing")
	}
	m = key(m, t, "i")
	if !strings.Contains(m.viewport.View(), "executes commands") {
		t.Fatal("composer must warn that shell text executes")
	}
	m = key(m, t, "printf hello")
	m.allSessions[1].Kind = monitor.SessionAgent
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || b.text != "" || m.composer.err == nil {
		t.Fatal("changed session kind must block sending")
	}
}
