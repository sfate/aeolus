package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
)

func TestFiltersPreserveSourceAndIdentity(t *testing.T) {
	a, b, c := testSession("%1"), testSession("%2"), testSession("%3")
	b.Provider = monitor.ProviderCodex
	b.Status = monitor.StatusUnknown
	m := NewModel()
	m.sessions = []monitor.Session{a, b, c}
	m.cursor = 2
	m.filters.applied.provider = 1
	m.applyFilters()
	if len(m.sessions) != 2 || m.sessions[m.cursor].Pane.PaneID != "%3" {
		t.Fatalf("filtered=%v cursor=%d", m.sessions, m.cursor)
	}
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{c, b, a}})
	if len(m.sessions) != 2 || m.sessions[m.cursor].Pane.PaneID != "%3" {
		t.Fatal("refresh changed selected pane")
	}
	m.filters.applied = filterChoice{}
	m.applyFilters()
	if len(m.sessions) != 3 || m.sessions[m.cursor].Pane.PaneID != "%3" {
		t.Fatal("clearing filter lost source or selection")
	}
	m.filters.applied = filterChoice{provider: 2, status: 1}
	m.applyFilters()
	if len(m.sessions) != 1 || m.sessions[0].Pane.PaneID != "%2" {
		t.Fatal("unknown status filter failed")
	}
}

func TestFilterModalAndEmptyActions(t *testing.T) {
	backend := &fakeBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if backend.sent != "" {
		t.Fatal("modal sent approval")
	}
	m.filters.draft.provider = 2
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.sessions) != 1 {
		t.Fatal("cancel applied draft")
	}
	m.filters.applied.provider = 2
	m.applyFilters()
	for _, k := range []string{"y", "a", "n", "x", "/"} {
		m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
	if backend.sent != "" || m.confirmKill != "" || m.search.active() {
		t.Fatal("empty filter acted on hidden pane")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.sessions) != 1 {
		t.Fatal("clear did not restore hidden pane")
	}
}

func TestFilteredActionsAndRemoval(t *testing.T) {
	backend := &fakeBackend{}
	hidden, visible := testSession("%1"), testSession("%2")
	hidden.Provider = monitor.ProviderCodex
	visible.Status = monitor.StatusPermission
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{hidden, visible}
	m.cursor = 1
	m.filters.applied = filterChoice{provider: 1, status: 4}
	m.applyFilters()
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if backend.sent != "%2:y" || len(m.sessions) != 0 {
		t.Fatalf("sent=%q visible=%d", backend.sent, len(m.sessions))
	}
	m.filters.applied = filterChoice{}
	m.applyFilters()
	if len(m.sessions) != 2 || m.sessions[1].Status != monitor.StatusWorking {
		t.Fatal("response not saved in full source")
	}
	m.cursor = 1
	m, _ = updateModel(t, m, killPaneMsg{paneID: "%1"})
	if len(m.sessions) != 1 || m.sessions[0].Pane.PaneID != "%2" || len(m.allSessions) != 1 {
		t.Fatal("kill result lost selection/source")
	}
}

func TestFiltersAndSearchModes(t *testing.T) {
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1")}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if m.filters.editing || m.search.query != "f" {
		t.Fatal("filter intercepted search text")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m.filters.draft.provider = 2
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.search.active() || len(m.sessions) != 0 {
		t.Fatal("search remained bound to hidden pane")
	}
	m, _ = updateModel(t, m, refreshMsg{sessions: nil})
	if len(m.allSessions) != 0 || len(m.sessions) != 0 || m.cursor != 0 {
		t.Fatal("empty refresh retained sessions")
	}
}

func TestUnknownProviderAndFilterSummary(t *testing.T) {
	m := NewModel()
	s := testSession("%9")
	s.Provider = monitor.ProviderUnknown
	m.sessions = []monitor.Session{s, testSession("%1")}
	m.filters.applied.provider = 4
	m.applyFilters()
	if len(m.sessions) != 1 || m.sessions[0].Pane.PaneID != "%9" {
		t.Fatal("unknown provider not selectable")
	}
	if !strings.Contains(m.filterSummary(), "Sessions: 1/2") {
		t.Fatalf("summary=%s", m.filterSummary())
	}
}
