package ui

import (
	"errors"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/preferences"
)

func TestExplicitPreferenceSaveCapturesAppliedFilters(t *testing.T) {
	m := NewModelWithBackend(&fakeBackend{}, &fakeMonitor{})
	m.filters.applied = filterChoice{provider: 2, status: 1, directory: "空/project"}
	var saved preferences.Snapshot
	m.preferenceSave = func(s preferences.Snapshot) error { saved = s; return errors.New("disk full") }
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	if cmd == nil || !m.preferencesSaving {
		t.Fatal("save not async")
	}
	m, next := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	if next != nil {
		t.Fatal("duplicate save")
	}
	m, _ = updateModel(t, m, cmd())
	if saved.Provider != "codex" || saved.Status != "unknown" || saved.Directory != "空/project" || m.preferencesSaving || m.preferenceNotice == "" {
		t.Fatal("wrong save or error lost")
	}
}

func TestPreferencesRestoreAndTextInputIsolation(t *testing.T) {
	m := NewModelWithBackend(&fakeBackend{}, &fakeMonitor{})
	m.restorePreferences(preferences.Snapshot{Provider: "gemini", Status: "working", Directory: "project"})
	if m.filters.applied != (filterChoice{provider: 3, status: 3, directory: "project"}) {
		t.Fatal("preferences not restored")
	}
	m.sessions = []monitor.Session{testSession("%1")}
	m = key(m, t, "i")
	m = key(m, t, "S")
	if m.preferencesSaving {
		t.Fatal("composer saved preferences")
	}
}

func TestStartupPreferenceLoadReportsErrorsAndRestores(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	path, err := preferences.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := preferences.Snapshot{Provider: "codex", Status: "idle", Directory: "project"}
	if err := preferences.Save(path, want); err != nil {
		t.Fatal(err)
	}
	m := NewModelWithBackend(&fakeBackend{}, &fakeMonitor{})
	m.LoadSavedPreferences()
	if m.filters.applied.provider != 2 || m.filters.applied.status != 2 || m.filters.applied.directory != "project" {
		t.Fatal("startup filters not restored")
	}
	if err := os.WriteFile(path, []byte(`{"version":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	m.LoadSavedPreferences()
	if m.preferenceNotice == "" || m.filters.applied.provider != 2 {
		t.Fatal("invalid state lost last valid preferences or error")
	}
}
