package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	store "github.com/sfate/aeolus/internal/workflows"
)

func shellWorkflowModel(t *testing.T) Model {
	t.Helper()
	m := NewModelWithBackend(&textBackend{}, &fakeMonitor{})
	s := testSession("%1")
	s.Kind, s.Provider, s.Status = monitor.SessionShell, monitor.ProviderUnknown, monitor.StatusUnknown
	m.sessions = []monitor.Session{s}
	return key(m, t, "i")
}

func openWorkflows(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlW})
	if cmd == nil || !m.composer.workflows.active || !m.composer.workflows.pending {
		t.Fatal("workflow load did not start")
	}
	return m, cmd
}

func TestWorkflowSaveParameterEntryAndExplicitSend(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := shellWorkflowModel(t)
	template := `printf '%s' {{value}}`
	m = key(m, t, template)
	m, cmd := openWorkflows(t, m)
	m, _ = updateModel(t, m, cmd())
	m = key(m, t, "s")
	m = key(m, t, "Print value")
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("save was not asynchronous")
	}
	m, again := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if again != nil {
		t.Fatal("duplicate save allowed")
	}
	m, _ = updateModel(t, m, cmd())
	if m.composer.workflows.err != nil {
		t.Fatal(m.composer.workflows.err)
	}
	m = key(m, t, "esc")
	if m.composer.draft != template {
		t.Fatal("saving changed draft")
	}
	m = key(m, t, "esc")
	m = key(m, t, "i")
	m, cmd = openWorkflows(t, m)
	m, _ = updateModel(t, m, cmd())
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	value := "secret-$(literal)-空'"
	m = key(m, t, value)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	want, err := store.Render(template, map[string]string{"value": value})
	if err != nil || m.composer.draft != want || m.composer.workflows.active {
		t.Fatalf("inserted %q want %q %v", m.composer.draft, want, err)
	}
	backend := m.backend.(*textBackend)
	if backend.text != "" || backend.target != "" {
		t.Fatal("workflow sent automatically")
	}
	path, _ := store.DefaultPath()
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), value) {
		t.Fatal("parameter values persisted")
	}
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("explicit send unavailable")
	}
	_, _ = updateModel(t, m, cmd())
	if backend.target != "%1" || backend.text != want {
		t.Fatal("explicit send did not use reviewed command")
	}
}

func TestWorkflowCancelledLoadCannotChangeReopenedComposer(t *testing.T) {
	m := shellWorkflowModel(t)
	calls := 0
	m.workflowIO.path = func() (string, error) { return "/test/workflows.json", nil }
	m.workflowIO.load = func(string) ([]store.Entry, error) {
		calls++
		return []store.Entry{{Name: strings.Repeat("x", calls), Template: "echo hi"}}, nil
	}
	m, cmd := openWorkflows(t, m)
	old := cmd()
	m = key(m, t, "esc")
	m = key(m, t, "esc")
	m = key(m, t, "i")
	m, cmd = openWorkflows(t, m)
	m, _ = updateModel(t, m, old)
	if !m.composer.workflows.pending || len(m.composer.workflows.entries) != 0 {
		t.Fatal("cancelled load affected reopened composer")
	}
	m, _ = updateModel(t, m, cmd())
	if m.composer.workflows.pending || m.composer.workflows.entries[0].Name != "xx" {
		t.Fatal("fresh load lost")
	}
}

func TestWorkflowChangedTargetsRetainDraftAndParameters(t *testing.T) {
	for _, change := range []string{"pane", "provider", "kind"} {
		m := shellWorkflowModel(t)
		m.composer.draft = "original"
		m.composer.position = len("original")
		m.workflowIO.path = func() (string, error) { return "/test/workflows.json", nil }
		m.workflowIO.load = func(string) ([]store.Entry, error) {
			return []store.Entry{{Name: "Test", Template: "echo {{value}}"}}, nil
		}
		m, cmd := openWorkflows(t, m)
		m, _ = updateModel(t, m, cmd())
		m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m = key(m, t, "value")
		switch change {
		case "pane":
			m.sessions[0].Pane.PaneID = "%2"
		case "provider":
			m.sessions[0].Provider = monitor.ProviderCodex
		case "kind":
			m.sessions[0].Kind = monitor.SessionAgent
		}
		m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil || m.composer.draft != "original" || m.composer.workflows.err == nil || !m.composer.workflows.active {
			t.Fatalf("changed %s target accepted", change)
		}
	}
}

func TestWorkflowModalPrecedenceAndSaveErrors(t *testing.T) {
	m := shellWorkflowModel(t)
	m.workflowIO.path = func() (string, error) { return filepath.Join(t.TempDir(), "workflows.json"), nil }
	m.workflowIO.load = func(string) ([]store.Entry, error) { return nil, nil }
	m.workflowIO.save = func(string, string, string) error { return errors.New("duplicate name") }
	m = key(m, t, "echo hi")
	m, cmd := openWorkflows(t, m)
	m, send := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if send != nil || m.composer.pending {
		t.Fatal("sent during load")
	}
	m, _ = updateModel(t, m, cmd())
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.composer.context.active {
		t.Fatal("workflow opened nested attachment")
	}
	m = key(m, t, "s")
	m = key(m, t, "Duplicate")
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, cmd())
	if m.composer.workflows.err == nil || m.composer.workflows.name != "Duplicate" || m.composer.draft != "echo hi" {
		t.Fatal("save error discarded input")
	}
	m = key(m, t, "esc")
	m = key(m, t, "esc")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlW})
	if m.composer.workflows.active || !m.composer.context.active {
		t.Fatal("attachment opened workflow modal")
	}
	agent := NewModel()
	agent.sessions = []monitor.Session{testSession("%3")}
	agent = key(agent, t, "i")
	agent, _ = updateModel(t, agent, tea.KeyMsg{Type: tea.KeyCtrlW})
	if agent.composer.workflows.active {
		t.Fatal("agent composer exposed shell workflows")
	}
}

func TestWorkflowParameterNavigationAndCancellation(t *testing.T) {
	m := shellWorkflowModel(t)
	m.workflowIO.path = func() (string, error) { return "/test/workflows.json", nil }
	m.workflowIO.load = func(string) ([]store.Entry, error) {
		return []store.Entry{{Name: "Two values", Template: "echo {{first}} {{second}}"}}, nil
	}
	m, cmd := openWorkflows(t, m)
	m, _ = updateModel(t, m, cmd())
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = key(m, t, "first 空")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = key(m, t, "second'")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.composer.workflows.values[0] != "first 空" {
		t.Fatal("parameter navigation lost value")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.composer.pending || m.backend.(*textBackend).text != "" {
		t.Fatal("sent from parameter form")
	}
	m = key(m, t, "esc")
	if m.composer.draft != "" || len(m.composer.workflows.values) != 0 || !m.composer.workflows.active {
		t.Fatal("parameter cancellation changed draft or retained values")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = key(m, t, "first")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = key(m, t, "second")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.composer.draft != "echo 'first' 'second'" || m.composer.workflows.active {
		t.Fatalf("rendered command %q", m.composer.draft)
	}
}
