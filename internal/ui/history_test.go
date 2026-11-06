package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/history"
	"github.com/sfate/aeolus/internal/monitor"
)

func historyModel(t *testing.T) Model {
	t.Helper()
	m := NewModelWithBackend(&textBackend{}, &fakeMonitor{})
	s := testSession("%1")
	s.Kind = monitor.SessionShell
	s.Provider = monitor.ProviderUnknown
	m.sessions = []monitor.Session{s}
	m = key(m, t, "i")
	m = key(m, t, "draft ")
	return m
}

func TestHistorySearchInsertsForReviewOnly(t *testing.T) {
	m := historyModel(t)
	m.historyLoad = func() ([]history.Entry, error) {
		return []history.Entry{{Command: "echo old", Directory: "/tmp"}, {Command: "printf 空", Directory: "/project"}}, nil
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil || !m.history.active {
		t.Fatal("history did not open")
	}
	m, _ = updateModel(t, m, cmd())
	m = key(m, t, "空")
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.history.active || m.composer.pending || m.composer.draft != "draft printf 空" {
		t.Fatalf("insert state %#v", m.composer)
	}
}

func TestHistoryCancellationAndStaleLoad(t *testing.T) {
	m := historyModel(t)
	m.historyLoad = func() ([]history.Entry, error) { return nil, errors.New("unreadable history") }
	m, old := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = key(m, t, "esc")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m, _ = updateModel(t, m, old())
	if !m.history.loading || m.history.err != nil || m.composer.draft != "draft " {
		t.Fatal("stale reply changed reopened library")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.composer.pending {
		t.Fatal("history modal allowed send")
	}
}

func TestHistoryBlocksChangedTargetInsertion(t *testing.T) {
	m := historyModel(t)
	m.historyLoad = func() ([]history.Entry, error) { return []history.Entry{{Command: "echo old"}}, nil }
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m, _ = updateModel(t, m, cmd())
	m.sessions[0].Kind = monitor.SessionAgent
	m = key(m, t, "enter")
	if !m.history.active || m.history.err == nil || m.composer.draft != "draft " {
		t.Fatal("changed target accepted")
	}
}

func TestExplicitHistorySaveIsAsyncAndReportsError(t *testing.T) {
	m := historyModel(t)
	m.historyWrite = func(e history.Entry) (int, error) {
		if e.Command != "echo test" {
			t.Fatal("wrong command saved")
		}
		return 0, errors.New("disk full")
	}
	cmd := m.saveHistoryEntry(history.Entry{Command: "echo test"})
	if cmd == nil || !m.historySaving || m.saveHistoryEntry(history.Entry{Command: "duplicate"}) != nil {
		t.Fatal("save not serialized")
	}
	m, _ = updateModel(t, m, cmd())
	if m.historySaving || m.historyNotice == "" || m.composer.draft != "draft " {
		t.Fatal("save error lost or draft changed")
	}
}
