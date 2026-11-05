package ui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
	"github.com/sfate/aeolus/internal/worktrees"
)

func worktreeModel(t *testing.T) Model {
	t.Helper()
	m := NewModelWithBackend(&lifecycleFake{}, &fakeMonitor{})
	s := testSession("%1")
	s.Pane.CurrentPath = t.TempDir()
	m.sessions = []monitor.Session{s}
	return key(m, t, "W")
}

func TestWorktreeLaunchRetainsCheckoutForRetry(t *testing.T) {
	m := worktreeModel(t)
	calls := 0
	source := m.sessions[0].Pane.CurrentPath
	m.worktree.create = func(_ context.Context, directory, target, branch string) (worktrees.Result, error) {
		calls++
		if directory != source || target != "/tmp/new-checkout" || branch != "feature/test" {
			t.Fatal("incorrect create arguments")
		}
		return worktrees.Result{Directory: target, Branch: branch, Ready: true}, nil
	}
	m.worktree.fields = [2]string{"/tmp/new-checkout", "feature/test"}
	backend := m.backend.(*lifecycleFake)
	backend.err = errors.New("agent unavailable")
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.worktree.busy {
		t.Fatal("creation must be asynchronous")
	}
	m, duplicate := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if duplicate != nil {
		t.Fatal("duplicate creation allowed")
	}
	m, cmd = updateModel(t, m, cmd())
	if cmd == nil || !m.worktree.result.Ready {
		t.Fatal("checkout did not proceed to launch")
	}
	m, _ = updateModel(t, m, cmd())
	if m.worktree.err == nil || !m.worktree.active || calls != 1 {
		t.Fatal("launch failure lost checkout")
	}
	backend.err = nil
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, cmd())
	if calls != 1 || m.worktree.active || m.selectedPaneID() != "%9" || backend.directory != "/tmp/new-checkout" {
		t.Fatal("retry recreated checkout or failed to select launch")
	}
}

func TestWorktreeRejectsChangedSourceAndPreservesModal(t *testing.T) {
	m := worktreeModel(t)
	m = key(m, t, "qyn/gif")
	if m.quitting || m.composer.active || m.review.active || m.filters.editing {
		t.Fatal("modal shortcuts escaped")
	}
	s := m.sessions[0]
	s.Provider = monitor.ProviderCodex
	m.sessions[0] = s
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.worktree.err == nil {
		t.Fatal("changed source accepted")
	}
}

func TestWorktreePartialFailureCannotRecreate(t *testing.T) {
	m := worktreeModel(t)
	m.worktree.create = func(context.Context, string, string, string) (worktrees.Result, error) {
		return worktrees.Result{Directory: "/tmp/retained", Branch: "retained"}, errors.New("partial creation")
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, cmd())
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.worktree.err == nil {
		t.Fatal("partial failure retried mutation")
	}
	m = key(m, t, "esc")
	if m.worktree.active {
		t.Fatal("cannot dismiss retained failure")
	}
}

func TestWorktreeRefreshDuringCreationBlocksLaunch(t *testing.T) {
	m := worktreeModel(t)
	m.worktree.create = func(context.Context, string, string, string) (worktrees.Result, error) {
		return worktrees.Result{Directory: "/tmp/retained", Branch: "topic", Ready: true}, nil
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, refreshMsg{sessions: nil})
	m, next := updateModel(t, m, cmd())
	if next != nil || m.worktree.err == nil || !m.worktree.result.Ready {
		t.Fatal("launched after target disappeared or lost retained checkout")
	}
}

func TestWorktreeKeyboardEditingAndModalEntry(t *testing.T) {
	m := worktreeModel(t)
	m = key(m, t, "空ab")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.worktree.fields[0] != "空b" {
		t.Fatalf("Unicode edit %q", m.worktree.fields[0])
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = key(m, t, "branch")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.worktree.fields[1] != "branch" || launchProviders[m.worktree.provider] != monitor.ProviderCodex {
		t.Fatal("field navigation failed")
	}
	m = key(m, t, "esc")
	m = key(m, t, "i")
	m = key(m, t, "W")
	if m.worktree.active || m.composer.draft != "W" {
		t.Fatal("composer W escaped into worktree form")
	}
}

func TestWorktreeUncertainLaunchRequiresInspection(t *testing.T) {
	m := worktreeModel(t)
	m.worktree.result = worktrees.Result{Directory: "/tmp/retained", Branch: "topic", Ready: true}
	m.worktree.busy = true
	m, _ = updateModel(t, m, worktreeLaunchedMsg{err: &tmux.LaunchUncertainError{Err: errors.New("metadata unavailable")}})
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !m.worktree.active || !m.worktree.launchUncertain {
		t.Fatal("uncertain launch was retried")
	}
}
