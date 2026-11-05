package ui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
	"testing"
)

type lifecycleFake struct {
	fakeBackend
	directory, command, target, name string
	err                              error
}

func (f *lifecycleFake) CreateSession(directory, command string) (tmux.Pane, error) {
	f.directory = directory
	f.command = command
	return tmux.Pane{PaneID: "%9", WindowName: command}, f.err
}
func (f *lifecycleFake) RenameWindow(target, name string) error {
	f.target = target
	f.name = name
	return f.err
}

func TestLaunchLifecycle(t *testing.T) {
	backend := &lifecycleFake{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.lifecycle.kind != lifecycleLaunch {
		t.Fatal("launch form not opened")
	}
	m.lifecycle.text = t.TempDir()
	m.lifecycle.position = len([]rune(m.lifecycle.text))
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.lifecycle.busy {
		t.Fatal("launch not asynchronous")
	}
	m, refresh := updateModel(t, m, cmd())
	if backend.command != "codex" || backend.directory == "" || refresh == nil || m.lifecycle.kind != lifecycleNone {
		t.Fatalf("launch state %#v backend %#v", m.lifecycle, backend)
	}
	if m.sessions[0].Provider != monitor.ProviderCodex || m.sessions[m.cursor].Pane.PaneID != "%9" {
		t.Fatal("created pane not selected")
	}
}

func TestRenameLifecycleKeepsTargetAndErrorInput(t *testing.T) {
	backend := &lifecycleFake{err: errors.New("rename failed")}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1"), testSession("%2")}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m.lifecycle.text = "review"
	m.lifecycle.position = 6
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{testSession("%2"), testSession("%1")}})
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, cmd())
	if backend.target != "%1" || m.lifecycle.text != "review" || m.lifecycle.err == nil || m.lifecycle.busy {
		t.Fatalf("state %#v target %q", m.lifecycle, backend.target)
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.lifecycle.kind != lifecycleNone {
		t.Fatal("cancel failed")
	}
}

func TestLifecycleModalDoesNotTriggerShortcuts(t *testing.T) {
	backend := &lifecycleFake{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m.lifecycle.text = ""
	m.lifecycle.position = 0
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("qynxR/c")})
	if m.quitting || backend.sent != "" || m.confirmKill != "" || m.search.active() || m.lifecycle.text != "qynxR/c" {
		t.Fatal("modal leaked shortcuts")
	}
}
