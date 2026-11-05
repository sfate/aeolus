package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/preferences"
)

var preferenceProviders = []string{"", "claude", "codex", "gemini", "unknown"}
var preferenceStatuses = []string{"", "unknown", "idle", "working", "permission", "question"}

type preferencesSavedMsg struct{ err error }

// LoadSavedPreferences restores explicit filter choices before terminal startup.
func (m *Model) LoadSavedPreferences() {
	path, err := preferences.DefaultPath()
	if err != nil {
		m.preferenceNotice = err.Error()
		return
	}
	saved, err := preferences.Load(path)
	if err != nil {
		m.preferenceNotice = fmt.Sprintf("Cannot restore saved filters: %v", err)
		return
	}
	m.restorePreferences(saved)
}

func (m *Model) restorePreferences(saved preferences.Snapshot) {
	choice := filterChoice{directory: saved.Directory}
	for i, p := range preferenceProviders {
		if p == saved.Provider {
			choice.provider = i
			break
		}
	}
	for i, s := range preferenceStatuses {
		if s == saved.Status {
			choice.status = i
			break
		}
	}
	m.filters.applied = choice
}

func (m *Model) savePreferences() tea.Cmd {
	if m.preferencesSaving {
		return nil
	}
	choice := m.filters.applied
	snapshot := preferences.Snapshot{Provider: preferenceProviders[choice.provider], Status: preferenceStatuses[choice.status], Directory: choice.directory}
	save := m.preferenceSave
	if save == nil {
		save = func(s preferences.Snapshot) error {
			path, err := preferences.DefaultPath()
			if err != nil {
				return err
			}
			return preferences.Save(path, s)
		}
	}
	m.preferencesSaving = true
	m.preferenceNotice = "Saving dashboard filters..."
	return func() tea.Msg { return preferencesSavedMsg{err: save(snapshot)} }
}
