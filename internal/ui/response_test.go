package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
)

type responseBackend struct {
	fakeBackend
	failure error
	calls   int
}

func (b *responseBackend) SendKeys(id, text string) error {
	b.calls++
	b.sent = id + ":" + text
	return b.failure
}
func (b *responseBackend) SendKeySequence(id string, keys ...string) error {
	b.calls++
	b.sent = id + ":" + strings.Join(keys, ",")
	return b.failure
}

func TestResponseWaitsForDeliveryAndRetainsFailure(t *testing.T) {
	for _, key := range []string{"y", "n", "a"} {
		t.Run(key, func(t *testing.T) {
			b := &responseBackend{failure: errors.New("transport failed")}
			m := NewModelWithBackend(b, nil)
			s := testSession("%1")
			s.Status = monitor.StatusPermission
			s.Request = &monitor.PermissionRequest{Tool: "Bash", Command: "original"}
			m.sessions = []monitor.Session{s}
			m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			if b.calls != 0 || cmd == nil || m.sessions[0].Status != s.Status || m.sessions[0].Request == nil {
				t.Fatal("response did not wait for delivery")
			}
			for _, repeat := range []string{"y", "n", "a"} {
				var duplicate tea.Cmd
				m, duplicate = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(repeat)})
				if duplicate != nil {
					t.Fatal("duplicate response queued")
				}
			}
			m, _ = updateModel(t, m, cmd())
			if b.calls != 1 || m.sessions[0].Status != s.Status || m.sessions[0].Request == nil || m.err == nil || !strings.Contains(m.err.Error(), "transport failed") {
				t.Fatal("failure cleared request or was hidden")
			}
			_, retry := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			if retry == nil {
				t.Fatal("failed response cannot be retried")
			}
		})
	}
}

func TestResponseCompletionStaysBoundToTarget(t *testing.T) {
	b := &responseBackend{}
	m := NewModelWithBackend(b, nil)
	original := testSession("%1")
	original.Status = monitor.StatusPermission
	original.Request = &monitor.PermissionRequest{Command: "original"}
	other := testSession("%2")
	other.Status = monitor.StatusPermission
	m.sessions = []monitor.Session{original, other}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m.cursor = 1
	m, _ = updateModel(t, m, cmd())
	if b.sent != "%1:y" || m.sessions[0].Status != monitor.StatusWorking || m.sessions[1].Status != monitor.StatusPermission || m.selectedPaneID() != "%2" {
		t.Fatal("response completion affected selected pane instead of original target")
	}
}

func TestResponseCompletionPreservesChangedTarget(t *testing.T) {
	for _, change := range []string{"provider", "request", "status", "removed"} {
		t.Run(change, func(t *testing.T) {
			b := &responseBackend{}
			m := NewModelWithBackend(b, nil)
			s := testSession("%1")
			s.Status = monitor.StatusPermission
			s.Request = &monitor.PermissionRequest{Command: "original"}
			m.sessions = []monitor.Session{s}
			m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
			switch change {
			case "provider":
				s.Provider = monitor.ProviderCodex
			case "request":
				s.Request = &monitor.PermissionRequest{Command: "new"}
			case "status":
				s.Status = monitor.StatusQuestion
			case "removed":
				s.Pane.PaneID = "%2"
			}
			m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{s}})
			m, _ = updateModel(t, m, cmd())
			if m.sessions[0].Status != s.Status || m.sessions[0].Request != s.Request {
				t.Fatal("stale completion changed newer target state")
			}
		})
	}
}

func TestResponseCompletionUpdatesHiddenOriginalTarget(t *testing.T) {
	b := &responseBackend{}
	m := NewModelWithBackend(b, nil)
	s := testSession("%1")
	s.Status = monitor.StatusPermission
	m.sessions = []monitor.Session{s}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m.filters.applied = filterChoice{provider: 2}
	m.applyFilters()
	m, _ = updateModel(t, m, cmd())
	if len(m.sessions) != 0 || len(m.allSessions) != 1 || m.allSessions[0].Status != monitor.StatusWorking || b.sent != "%1:Down,Enter" {
		t.Fatal("hidden original target not updated")
	}
	m.filters.applied = filterChoice{}
	m.applyFilters()
	if len(m.sessions) != 1 || m.sessions[0].Status != monitor.StatusWorking {
		t.Fatal("clearing filters lost completed response")
	}
}

func TestStaleResponseCompletionDoesNotUnlockAnotherSend(t *testing.T) {
	b := &responseBackend{}
	m := NewModelWithBackend(b, nil)
	s := testSession("%1")
	s.Status = monitor.StatusPermission
	m.sessions = []monitor.Session{s}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	result := cmd()
	m, _ = updateModel(t, m, responseSentMsg{id: 999})
	if m.response == nil {
		t.Fatal("unrelated completion unlocked response")
	}
	m, _ = updateModel(t, m, result)
	if m.response != nil {
		t.Fatal("matching completion did not unlock response")
	}
	m.sessions[0].Status = monitor.StatusPermission
	m, second := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m, _ = updateModel(t, m, result)
	if m.response == nil || m.sessions[0].Status != monitor.StatusPermission {
		t.Fatal("old completion changed newer operation")
	}
	m, _ = updateModel(t, m, second())
	if m.response != nil || m.sessions[0].Status != monitor.StatusIdle {
		t.Fatal("newer completion was lost")
	}
}
