package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
)

// Index zero means all values; later entries select one provider or status.
type filterChoice struct{ provider, status int }
type sessionFilters struct {
	editing        bool
	applied, draft filterChoice
}

var filterProviders = []monitor.Provider{monitor.ProviderUnknown, monitor.ProviderClaude, monitor.ProviderCodex, monitor.ProviderGemini, monitor.ProviderUnknown}
var filterStatuses = []monitor.Status{monitor.Status(-1), monitor.StatusUnknown, monitor.StatusIdle, monitor.StatusWorking, monitor.StatusPermission, monitor.StatusQuestion}

func (f filterChoice) matches(s monitor.Session) bool {
	return (f.provider == 0 || s.Provider == filterProviders[f.provider]) && (f.status == 0 || s.Status == filterStatuses[f.status])
}
func (f filterChoice) description() string {
	provider, status := "All", "All"
	if f.provider != 0 {
		provider = filterProviders[f.provider].String()
	}
	if f.status != 0 {
		status = filterStatuses[f.status].String()
	}
	return fmt.Sprintf("Provider: %s  Status: %s", provider, status)
}

func (m Model) selectedPaneID() string {
	if m.cursor >= 0 && m.cursor < len(m.sessions) {
		return m.sessions[m.cursor].Pane.PaneID
	}
	return ""
}

// Save local response state before rebuilding the visible subset.
func (m *Model) syncFilterSource() {
	if m.allSessions == nil {
		m.allSessions = append([]monitor.Session{}, m.sessions...)
		return
	}
	visible := make(map[string]monitor.Session, len(m.sessions))
	for _, s := range m.sessions {
		visible[s.Pane.PaneID] = s
	}
	for i, s := range m.allSessions {
		if updated, ok := visible[s.Pane.PaneID]; ok {
			m.allSessions[i] = updated
		}
	}
}

func (m *Model) applyFilters() {
	selected := m.selectedPaneID()
	m.syncFilterSource()
	m.rebuildFilteredSessions(selected)
}

func (m *Model) rebuildFilteredSessions(selected string) {
	m.sessions = make([]monitor.Session, 0, len(m.allSessions))
	for _, s := range m.allSessions {
		if m.filters.applied.matches(s) {
			m.sessions = append(m.sessions, s)
		}
	}
	m.cursor = 0
	for i, s := range m.sessions {
		if s.Pane.PaneID == selected {
			m.cursor = i
			break
		}
	}
	if !hasPane(m.sessions, m.confirmKill) {
		m.confirmKill = ""
	}
	m.updateViewport()
}

func (m *Model) filterKey(msg tea.KeyMsg) bool {
	if !m.filters.editing {
		if msg.String() != "f" {
			return false
		}
		m.filters.editing = true
		m.filters.draft = m.filters.applied
		m.updateViewport()
		return true
	}
	switch msg.String() {
	case "p":
		m.filters.draft.provider = (m.filters.draft.provider + 1) % len(filterProviders)
	case "s":
		m.filters.draft.status = (m.filters.draft.status + 1) % len(filterStatuses)
	case "c":
		m.filters.draft = filterChoice{}
	case "enter":
		m.filters.applied = m.filters.draft
		m.filters.editing = false
		m.applyFilters()
	case "esc":
		m.filters.editing = false
	case "ctrl+c":
		m.quitting = true
	}
	m.updateViewport()
	return true
}

func (m Model) filterSummary() string {
	total := len(m.allSessions)
	if m.allSessions == nil {
		total = len(m.sessions)
	}
	return fmt.Sprintf("%s  Sessions: %d/%d", m.filters.applied.description(), len(m.sessions), total)
}
