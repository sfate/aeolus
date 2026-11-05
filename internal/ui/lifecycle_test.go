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

func TestLifecycleModalIsolation(t *testing.T) {
	m := NewModelWithBackend(&lifecycleFake{}, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m = key(m, t, "i")
	m = key(m, t, "cRf")
	if m.lifecycle.kind != lifecycleNone || m.filters.editing || m.composer.draft != "cRf" {
		t.Fatal("composer shortcuts opened another modal")
	}
	m = key(m, t, "esc")
	m = key(m, t, "f")
	m = key(m, t, "c")
	m = key(m, t, "R")
	if m.lifecycle.kind != lifecycleNone || m.composer.active {
		t.Fatal("filter shortcuts opened lifecycle modal")
	}
	m = key(m, t, "esc")
	m = key(m, t, "c")
	m = key(m, t, "if")
	if m.composer.active || m.filters.editing || m.lifecycle.kind != lifecycleLaunch {
		t.Fatal("lifecycle input opened another modal")
	}
}

func TestLaunchAddsToSourceAndSelectsDespiteFilters(t *testing.T) {
	m := NewModelWithBackend(&lifecycleFake{}, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m.filters.applied.provider = 1
	m.applyFilters()
	m.lifecycle = lifecycleForm{kind: lifecycleLaunch, busy: true}
	launched := tmux.Pane{PaneID: "%9", WindowName: "codex"}
	m, _ = updateModel(t, m, lifecycleMsg{pane: launched, provider: monitor.ProviderCodex})
	if len(m.allSessions) != 2 || m.selectedPaneID() != "%9" || m.filters.applied != (filterChoice{}) {
		t.Fatal("launch was hidden or missing from source")
	}
	m.lifecycle = lifecycleForm{kind: lifecycleRename, paneID: "%9", text: "review", busy: true}
	m, _ = updateModel(t, m, lifecycleMsg{})
	if m.sessions[m.cursor].Pane.WindowName != "review" || m.allSessions[1].Pane.WindowName != "review" {
		t.Fatal("rename failed to update full source")
	}
}
