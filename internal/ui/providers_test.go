package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"strings"
	"testing"
)

func TestUnsupportedProvidersNeverRespond(t *testing.T) {
	for _, provider := range []monitor.Provider{monitor.ProviderCodex, monitor.ProviderGemini, monitor.ProviderUnknown} {
		for _, key := range []rune{'y', 'a', 'n'} {
			for _, status := range []monitor.Status{monitor.StatusPermission, monitor.StatusQuestion} {
				backend := &fakeBackend{}
				m := NewModelWithBackend(backend, &fakeMonitor{})
				s := testSession("%1")
				s.Provider = provider
				s.Status = status
				m.sessions = []monitor.Session{s}
				m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
				if backend.sent != "" || m.sessions[0].Status != status {
					t.Fatalf("%s %s %c: responded to unsupported provider", provider, status, key)
				}
				if strings.Contains(m.contentPaneText(), "[y]") {
					t.Fatal("unsupported response shown")
				}
				if strings.Contains(m.helpBar(), "Approve") {
					t.Fatal("unsupported help shown")
				}
			}
		}
	}
}

func TestProviderLabelsAndUnknownExplanation(t *testing.T) {
	m := NewModel()
	s := testSession("%1")
	s.Provider = monitor.ProviderCodex
	s.Status = monitor.StatusUnknown
	s.Content = "raw output"
	m.sessions = []monitor.Session{s}
	if !strings.Contains(sessionLabel(s), "Codex") {
		t.Fatal("missing provider label")
	}
	if content := m.contentPaneText(); !strings.Contains(content, "raw output") || !strings.Contains(content, "unknown") {
		t.Fatalf("unexpected content %q", content)
	}
}
