package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
)

// Index zero means all values; later entries select one provider or status.
type filterChoice struct {
	provider, status int
	directory        string
}
type sessionFilters struct {
	editing        bool
	projectEditing bool
	projectCursor  int
	applied, draft filterChoice
}

var filterProviders = []monitor.Provider{monitor.ProviderUnknown, monitor.ProviderClaude, monitor.ProviderCodex, monitor.ProviderGemini, monitor.ProviderUnknown}
var filterStatuses = []monitor.Status{monitor.Status(-1), monitor.StatusUnknown, monitor.StatusIdle, monitor.StatusWorking, monitor.StatusPermission, monitor.StatusQuestion}

func (f filterChoice) matches(s monitor.Session) bool {
	providerMatches := f.provider == 0
	if f.provider == len(filterProviders) {
		providerMatches = s.Kind == monitor.SessionShell
	} else if f.provider != 0 {
		providerMatches = s.Kind != monitor.SessionShell && s.Provider == filterProviders[f.provider]
	}
	return providerMatches && (f.status == 0 || s.Status == filterStatuses[f.status]) && strings.Contains(s.Pane.CurrentPath, f.directory)
}
func (f filterChoice) description() string {
	provider, status := "All", "All"
	if f.provider == len(filterProviders) {
		provider = "Shell"
	} else if f.provider != 0 {
		provider = filterProviders[f.provider].String()
	}
	if f.status != 0 {
		status = filterStatuses[f.status].String()
	}
	return fmt.Sprintf("Provider: %s  Status: %s  Working dir contains: %q", provider, status, f.directory)
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
		m.filters.projectEditing = false
		m.filters.projectCursor = len([]rune(m.filters.draft.directory))
		m.updateViewport()
		return true
	}
	if msg.String() == "tab" {
		m.filters.projectEditing = !m.filters.projectEditing
		m.updateViewport()
		return true
	}
	if m.filters.projectEditing && msg.String() != "enter" && msg.String() != "esc" && msg.String() != "ctrl+c" {
		m.editProjectFilter(msg)
		m.updateViewport()
		return true
	}
	switch msg.String() {
	case "p":
		m.filters.draft.provider = (m.filters.draft.provider + 1) % (len(filterProviders) + 1)
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
	summary := fmt.Sprintf("%s  Sessions: %d/%d", m.filters.applied.description(), len(m.sessions), total)
	if m.cursor >= 0 && m.cursor < len(m.sessions) {
		summary += fmt.Sprintf("  Selected dir: %q", m.sessions[m.cursor].Pane.CurrentPath)
	}
	return summary
}

func (m *Model) editProjectFilter(msg tea.KeyMsg) {
	r := []rune(m.filters.draft.directory)
	p := m.filters.projectCursor
	switch msg.String() {
	case "left":
		if p > 0 {
			p--
		}
	case "right":
		if p < len(r) {
			p++
		}
	case "home", "ctrl+a":
		p = 0
	case "end", "ctrl+e":
		p = len(r)
	case "ctrl+u":
		r = nil
		p = 0
	case "backspace", "ctrl+h":
		if p > 0 {
			r = append(r[:p-1], r[p:]...)
			p--
		}
	case "delete":
		if p < len(r) {
			r = append(r[:p], r[p+1:]...)
		}
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			input := msg.Runes
			if msg.Type == tea.KeySpace {
				input = []rune{' '}
			}
			clean := make([]rune, 0, len(input))
			for _, c := range input {
				if !unicode.IsControl(c) {
					clean = append(clean, c)
				}
			}
			next := append([]rune{}, r[:p]...)
			next = append(next, clean...)
			next = append(next, r[p:]...)
			r = next
			p += len(clean)
		}
	}
	m.filters.draft.directory = string(r)
	m.filters.projectCursor = p
}

func (m Model) filterModalText() string {
	text := "Filter sessions\n\n" + m.filters.draft.description() + "\n\n[tab] Toggle controls / working-directory text\n"
	if m.filters.projectEditing {
		r := []rune(m.filters.draft.directory)
		p := m.filters.projectCursor
		return text + "Editing working-directory substring: " + string(r[:p]) + "▏" + string(r[p:]) + "\n[ctrl+u] Clear text  [enter] Apply  [esc] Cancel"
	}
	return text + "[p] Cycle provider  [s] Cycle status\n[c] Clear all  [enter] Apply  [esc] Cancel"
}
