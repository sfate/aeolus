package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
)

func testSession(paneID string) monitor.Session {
	return monitor.Session{
		Provider: monitor.ProviderClaude,
		Pane: tmux.Pane{
			PaneID:      paneID,
			SessionName: "session-" + strings.TrimPrefix(paneID, "%"),
			SessionID:   "$" + strings.TrimPrefix(paneID, "%"),
			WindowName:  "window-" + strings.TrimPrefix(paneID, "%"),
		},
		Status:  monitor.StatusIdle,
		Content: "line 1\nline 2",
	}
}

func updateModel(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want ui.Model", next)
	}
	return updated, cmd
}

func TestHasPane(t *testing.T) {
	sessions := []monitor.Session{testSession("%1"), testSession("%2")}

	if !hasPane(sessions, "%1") {
		t.Fatal("hasPane returned false for an existing pane")
	}
	if hasPane(sessions, "%3") {
		t.Fatal("hasPane returned true for a missing pane")
	}
}

func TestRemovePane(t *testing.T) {
	sessions := []monitor.Session{testSession("%1"), testSession("%2"), testSession("%3")}

	got := removePane(sessions, "%2")

	if len(got) != 2 {
		t.Fatalf("len(removePane) = %d, want 2", len(got))
	}
	if got[0].Pane.PaneID != "%1" || got[1].Pane.PaneID != "%3" {
		t.Fatalf("removePane kept panes %q and %q, want %%1 and %%3", got[0].Pane.PaneID, got[1].Pane.PaneID)
	}
}

func TestKillConfirmationKeyFlow(t *testing.T) {
	t.Run("x asks for confirmation for selected pane", func(t *testing.T) {
		m := NewModel()
		m.sessions = []monitor.Session{testSession("%1"), testSession("%2")}
		m.cursor = 1

		m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

		if cmd != nil {
			t.Fatal("x returned a command, want nil")
		}
		if m.confirmKill != "%2" {
			t.Fatalf("confirmKill = %q, want %%2", m.confirmKill)
		}
	})

	t.Run("n cancels confirmation", func(t *testing.T) {
		m := NewModel()
		m.sessions = []monitor.Session{testSession("%1")}
		m.confirmKill = "%1"

		m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

		if cmd != nil {
			t.Fatal("n returned a command, want nil")
		}
		if m.confirmKill != "" {
			t.Fatalf("confirmKill = %q, want empty", m.confirmKill)
		}
	})

	t.Run("esc cancels confirmation", func(t *testing.T) {
		m := NewModel()
		m.sessions = []monitor.Session{testSession("%1")}
		m.confirmKill = "%1"

		m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})

		if cmd != nil {
			t.Fatal("esc returned a command, want nil")
		}
		if m.confirmKill != "" {
			t.Fatalf("confirmKill = %q, want empty", m.confirmKill)
		}
	})

	t.Run("other keys are ignored while confirming", func(t *testing.T) {
		m := NewModel()
		m.sessions = []monitor.Session{testSession("%1"), testSession("%2")}
		m.confirmKill = "%1"

		m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})

		if cmd != nil {
			t.Fatal("down returned a command, want nil")
		}
		if m.cursor != 0 {
			t.Fatalf("cursor = %d, want 0", m.cursor)
		}
		if m.confirmKill != "%1" {
			t.Fatalf("confirmKill = %q, want %%1", m.confirmKill)
		}
	})

	t.Run("y clears confirmation and returns kill command", func(t *testing.T) {
		m := NewModel()
		m.sessions = []monitor.Session{testSession("%1")}
		m.confirmKill = "%1"

		m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

		if cmd == nil {
			t.Fatal("y returned nil command, want kill command")
		}
		if m.confirmKill != "" {
			t.Fatalf("confirmKill = %q, want empty", m.confirmKill)
		}
		if len(m.sessions) != 1 {
			t.Fatalf("len(sessions) = %d, want 1 before kill result arrives", len(m.sessions))
		}
	})
}

func TestKillPaneMsg(t *testing.T) {
	t.Run("success removes pane and refreshes", func(t *testing.T) {
		m := NewModel()
		m.sessions = []monitor.Session{testSession("%1"), testSession("%2")}
		m.cursor = 1
		m.confirmKill = "%2"

		m, cmd := updateModel(t, m, killPaneMsg{paneID: "%2"})

		if cmd == nil {
			t.Fatal("killPaneMsg success returned nil command, want refresh command")
		}
		if m.confirmKill != "" {
			t.Fatalf("confirmKill = %q, want empty", m.confirmKill)
		}
		if len(m.sessions) != 1 || m.sessions[0].Pane.PaneID != "%1" {
			t.Fatalf("sessions after kill = %#v, want only %%1", m.sessions)
		}
		if m.cursor != 0 {
			t.Fatalf("cursor = %d, want 0", m.cursor)
		}
	})

	t.Run("error records err and leaves sessions intact", func(t *testing.T) {
		killErr := errors.New("tmux failed")
		m := NewModel()
		m.sessions = []monitor.Session{testSession("%1")}
		m.confirmKill = "%1"

		m, cmd := updateModel(t, m, killPaneMsg{paneID: "%1", err: killErr})

		if cmd != nil {
			t.Fatal("killPaneMsg error returned a command, want nil")
		}
		if !errors.Is(m.err, killErr) {
			t.Fatalf("err = %v, want %v", m.err, killErr)
		}
		if m.confirmKill != "" {
			t.Fatalf("confirmKill = %q, want empty", m.confirmKill)
		}
		if len(m.sessions) != 1 || m.sessions[0].Pane.PaneID != "%1" {
			t.Fatalf("sessions after failed kill = %#v, want original %%1", m.sessions)
		}
	})
}

func TestRefreshClearsStaleKillConfirmation(t *testing.T) {
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1"), testSession("%2")}
	m.confirmKill = "%2"

	m, cmd := updateModel(t, m, refreshMsg{sessions: []monitor.Session{testSession("%1")}})

	if cmd != nil {
		t.Fatal("refreshMsg returned a command, want nil")
	}
	if m.confirmKill != "" {
		t.Fatalf("confirmKill = %q, want empty", m.confirmKill)
	}
	if len(m.sessions) != 1 || m.sessions[0].Pane.PaneID != "%1" {
		t.Fatalf("sessions after refresh = %#v, want only %%1", m.sessions)
	}
}

type fakeBackend struct {
	listErr error
	tmux.Backend
	panes    []tmux.Pane
	killed   string
	switched string
	sent     string
}

func (f *fakeBackend) ListAllPanes() ([]tmux.Pane, error) { return f.panes, f.listErr }
func (f *fakeBackend) KillPane(id string) error           { f.killed = id; return nil }
func (f *fakeBackend) SwitchToPane(p tmux.Pane) error     { f.switched = p.PaneID; return nil }
func (f *fakeBackend) SendKeys(id, text string) error     { f.sent = id + ":" + text; return nil }
func (f *fakeBackend) SendKeySequence(id string, keys ...string) error {
	f.sent = id + ":" + strings.Join(keys, ",")
	return nil
}

type fakeMonitor struct{ panes []tmux.Pane }

func (f *fakeMonitor) Refresh(panes []tmux.Pane) []monitor.Session {
	f.panes = panes
	return []monitor.Session{testSession(panes[0].PaneID)}
}

func TestInjectedBackend(t *testing.T) {
	backend := &fakeBackend{panes: []tmux.Pane{{PaneID: "%7"}}}
	detector := &fakeMonitor{}
	m := NewModelWithBackend(backend, detector)
	result := m.doRefresh()().(refreshMsg)
	if result.err != nil || len(detector.panes) != 1 || result.sessions[0].Pane.PaneID != "%7" {
		t.Fatalf("refresh=%#v", result)
	}
	m.sessions = result.sessions
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if backend.switched != "%7" {
		t.Fatal("switch did not use backend")
	}
	m.sessions[0].Status = monitor.StatusPermission
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if backend.sent != "%7:Down,Enter" {
		t.Fatalf("sent=%q", backend.sent)
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	_, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	msg := cmd().(killPaneMsg)
	if msg.err != nil || backend.killed != "%7" {
		t.Fatalf("kill=%#v", msg)
	}
}

func TestInjectedResponseKeys(t *testing.T) {
	for _, key := range []rune{'y', 'n'} {
		for _, status := range []monitor.Status{monitor.StatusPermission, monitor.StatusQuestion} {
			backend := &fakeBackend{}
			m := NewModelWithBackend(backend, &fakeMonitor{})
			m.sessions = []monitor.Session{testSession("%1")}
			m.sessions[0].Status = status
			m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
			if backend.sent != "%1:"+string(key) {
				t.Fatalf("key %c status %v sent %q", key, status, backend.sent)
			}
			want := monitor.StatusWorking
			if key == 'n' {
				want = monitor.StatusIdle
			}
			if m.sessions[0].Status != want {
				t.Fatalf("status %v want %v", m.sessions[0].Status, want)
			}
		}
	}
}
func TestInjectedRefreshError(t *testing.T) {
	expected := errors.New("no server")
	m := NewModelWithBackend(&fakeBackend{listErr: expected}, nil)
	result := m.doRefresh()().(refreshMsg)
	if !errors.Is(result.err, expected) {
		t.Fatalf("error=%v", result.err)
	}
}
