package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
)

func attentionRefresh(t *testing.T, m Model, sessions ...monitor.Session) Model {
	t.Helper()
	m, _ = updateModel(t, m, refreshMsg{sessions: sessions})
	return m
}

func TestAttentionTransitionsAndExplicitAcknowledgement(t *testing.T) {
	for _, transition := range []struct {
		from, to monitor.Status
		want     string
	}{{monitor.StatusIdle, monitor.StatusPermission, "Permission requested"}, {monitor.StatusWorking, monitor.StatusQuestion, "Question asked"}, {monitor.StatusWorking, monitor.StatusIdle, "Became idle"}} {
		m := NewModel()
		s := testSession("%1")
		s.Status = transition.from
		m = attentionRefresh(t, m, s)
		if strings.Contains(m.filterSummary(), "unread") {
			t.Fatal("initial snapshot created unread attention")
		}
		s.Status = transition.to
		m = attentionRefresh(t, m, s)
		if !strings.Contains(m.filterSummary(), "!1 unread") || !strings.Contains(m.contentPaneText(), transition.want) || !strings.Contains(m.renderSessionList(60, 10), "! ") {
			t.Fatalf("transition did not show attention: %q | %q | %q", m.filterSummary(), m.contentPaneText(), m.renderSessionList(60, 10))
		}
		m = attentionRefresh(t, m, s)
		if !strings.Contains(m.filterSummary(), "!1 unread") {
			t.Fatal("refresh cleared unread attention")
		}
		m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
		if strings.Contains(m.filterSummary(), "unread") || strings.Contains(m.contentPaneText(), "Unread:") {
			t.Fatal("explicit acknowledgement did not clear attention")
		}
		m = attentionRefresh(t, m, s)
		if strings.Contains(m.filterSummary(), "unread") {
			t.Fatal("unchanged snapshot recreated acknowledged attention")
		}
	}
}

func TestAttentionFilteringSelectionAndRemoval(t *testing.T) {
	m := NewModel()
	a, b := testSession("%1"), testSession("%2")
	a.Status = monitor.StatusWorking
	b.Status = monitor.StatusIdle
	m = attentionRefresh(t, m, a, b)
	m.cursor = 1
	a.Status = monitor.StatusPermission
	m = attentionRefresh(t, m, a, b)
	if m.selectedPaneID() != "%2" {
		t.Fatal("attention changed selection")
	}
	m.filters.applied = filterChoice{status: 2}
	m.applyFilters()
	if !strings.Contains(m.filterSummary(), "!1 unread (1 hidden)") {
		t.Fatal("hidden attention disappeared from count")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if !strings.Contains(m.filterSummary(), "!1 unread") {
		t.Fatal("acknowledgement affected hidden target")
	}
	m.filters.applied = filterChoice{}
	m.applyFilters()
	m = attentionRefresh(t, m, b)
	if strings.Contains(m.filterSummary(), "unread") {
		t.Fatal("removed pane retained attention")
	}
	m = attentionRefresh(t, m, a, b)
	if strings.Contains(m.filterSummary(), "unread") {
		t.Fatal("rediscovery generated stale attention")
	}
}

func TestAttentionDoesNotInferUnsupportedStates(t *testing.T) {
	for _, provider := range []monitor.Provider{monitor.ProviderCodex, monitor.ProviderGemini, monitor.ProviderUnknown} {
		m := NewModel()
		s := testSession("%1")
		s.Provider = provider
		s.Status = monitor.StatusWorking
		m = attentionRefresh(t, m, s)
		for _, status := range []monitor.Status{monitor.StatusIdle, monitor.StatusPermission, monitor.StatusQuestion, monitor.StatusUnknown} {
			s.Status = status
			m = attentionRefresh(t, m, s)
			if strings.Contains(m.filterSummary(), "unread") {
				t.Fatalf("invented %s attention", provider)
			}
		}
	}
	m := NewModel()
	s := testSession("%1")
	s.Status = monitor.StatusWorking
	m = attentionRefresh(t, m, s)
	s.Status = monitor.StatusUnknown
	m = attentionRefresh(t, m, s)
	if strings.Contains(m.filterSummary(), "unread") {
		t.Fatal("unknown interpreted as completion")
	}
}

func TestAttentionIdentityFailureAndNewRequests(t *testing.T) {
	m := NewModel()
	s := testSession("%1")
	s.Status = monitor.StatusIdle
	m = attentionRefresh(t, m, s)
	s.Status = monitor.StatusPermission
	m = attentionRefresh(t, m, s)
	m, _ = updateModel(t, m, refreshMsg{err: errors.New("transport unavailable")})
	if !strings.Contains(m.filterSummary(), "!1 unread") {
		t.Fatal("failed refresh cleared unread attention")
	}
	s.Provider = monitor.ProviderCodex
	m = attentionRefresh(t, m, s)
	if strings.Contains(m.filterSummary(), "unread") {
		t.Fatal("provider identity change kept stale badge")
	}
	s.Provider = monitor.ProviderClaude
	s.Status = monitor.StatusWorking
	m = attentionRefresh(t, m, s)
	s.Status = monitor.StatusQuestion
	s.Pane.PID++
	m = attentionRefresh(t, m, s)
	if strings.Contains(m.filterSummary(), "unread") {
		t.Fatal("replacement process generated stale attention")
	}
	s.Status = monitor.StatusPermission
	m = attentionRefresh(t, m, s)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	s.Status = monitor.StatusWorking
	m = attentionRefresh(t, m, s)
	s.Status = monitor.StatusPermission
	m = attentionRefresh(t, m, s)
	if !strings.Contains(m.filterSummary(), "!1 unread") {
		t.Fatal("new transition did not create attention after acknowledgement")
	}
}

func TestAttentionModalPrecedenceAndKill(t *testing.T) {
	m := NewModelWithBackend(&fakeBackend{}, nil)
	s := testSession("%1")
	s.Status = monitor.StatusWorking
	m = attentionRefresh(t, m, s)
	s.Status = monitor.StatusPermission
	m = attentionRefresh(t, m, s)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.filterSummary(), "!1 unread") {
		t.Fatal("enter implicitly acknowledged attention")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if m.composer.draft != "!" || !strings.Contains(m.filterSummary(), "!1 unread") {
		t.Fatal("composer did not retain exclamation as draft text")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = updateModel(t, m, killPaneMsg{paneID: "%1"})
	if strings.Contains(m.filterSummary(), "unread") {
		t.Fatal("confirmed kill retained badge")
	}
}

func TestAttentionBaselineLatestEventAndSelectedAcknowledgement(t *testing.T) {
	m := NewModel()
	a, b := testSession("%1"), testSession("%2")
	a.Status = monitor.StatusPermission
	b.Status = monitor.StatusQuestion
	m = attentionRefresh(t, m, a, b)
	if strings.Contains(m.filterSummary(), "unread") {
		t.Fatal("initial waiting snapshot flooded alerts")
	}
	a.Status = monitor.StatusWorking
	b.Status = monitor.StatusWorking
	m = attentionRefresh(t, m, a, b)
	a.Status = monitor.StatusIdle
	b.Status = monitor.StatusPermission
	m = attentionRefresh(t, m, a, b)
	if !strings.Contains(m.filterSummary(), "!2 unread") {
		t.Fatal("multiple attention events not counted")
	}
	a.Status = monitor.StatusWorking
	m = attentionRefresh(t, m, a, b)
	if !strings.Contains(m.filterSummary(), "!2 unread") {
		t.Fatal("later activity implicitly acknowledged event")
	}
	a.Status = monitor.StatusQuestion
	m = attentionRefresh(t, m, a, b)
	if !strings.Contains(m.contentPaneText(), "Question asked") {
		t.Fatal("latest transition did not replace earlier event")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if !strings.Contains(m.filterSummary(), "!1 unread") || strings.Contains(m.contentPaneText(), "Unread:") {
		t.Fatal("selected acknowledgement cleared other panes or retained current event")
	}
}
