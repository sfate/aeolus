package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/config"
	"github.com/sfate/aeolus/internal/preferences"
)

func TestIntegrationPaletteUsesConfiguredAction(t *testing.T) {
	m := NewModelWithBackend(&lifecycleFake{}, nil)
	m.keybindings = map[string]string{"launch": "L"}
	m = key(m, t, "ctrl+k")
	m.palette.query = "Launch"
	m.selectPaletteFirst()
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.lifecycle.kind != lifecycleLaunch || m.palette.active {
		t.Fatal("palette canonical action lost after key remapping")
	}
}

func TestIntegrationWorktreeFormOwnsInput(t *testing.T) {
	m := NewModelWithBackend(&lifecycleFake{}, nil)
	s := testSession("%1")
	s.Pane.CurrentPath = "/tmp"
	m.sessions = append(m.sessions, s)
	m = key(m, t, "W")
	m = key(m, t, "?")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.help.active || m.palette.active || m.worktree.fields[0] != "?" {
		t.Fatalf("help/palette escaped active worktree form: active=%v help=%v palette=%v text=%q", m.worktree.active, m.help.active, m.palette.active, m.worktree.fields[0])
	}
}

func TestIntegrationShellPresetAndPreferences(t *testing.T) {
	m := NewModelWithBackend(&shellBackend{}, nil)
	m.projects = []config.Project{{Name: "agent", Directory: t.TempDir(), Provider: "codex"}}
	m = key(m, t, "c")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if !m.lifecycle.shell {
		t.Fatal("shell choice missing")
	}
	m = key(m, t, "ctrl+p")
	if m.lifecycle.shell || launchProviders[m.lifecycle.provider] != "codex" {
		t.Fatal("agent preset retained shell mode")
	}
	m.lifecycle = lifecycleForm{}
	m.filters.applied = filterChoice{provider: len(filterProviders)}
	var saved preferences.Snapshot
	m.preferenceSave = func(s preferences.Snapshot) error {
		saved = s
		return preferences.Save(t.TempDir()+"/dashboard.json", s)
	}
	cmd := m.savePreferences()
	if cmd == nil {
		t.Fatal("save not scheduled")
	}
	result := cmd().(preferencesSavedMsg)
	if result.err != nil || saved.Provider != "shell" {
		t.Fatalf("shell filter not saved: %#v %v", saved, result.err)
	}
	m.restorePreferences(saved)
	if m.filters.applied.provider != len(filterProviders) {
		t.Fatal("shell filter not restored")
	}
}

func TestIntegrationNarrowFooterUsesConfiguredKeys(t *testing.T) {
	m := NewModel()
	m.keybindings = map[string]string{"next": "N", "previous": "P", "launch": "L", "quit": "Q"}
	m.width = 40
	m.height = 24
	footer := m.layoutFooter()
	if !strings.Contains(footer, "[L]") || !strings.Contains(footer, "[Q]") || strings.Contains(footer, "[c]") {
		t.Fatalf("narrow footer misrepresented configured shortcuts: %q",footer)
	}
}

func TestIntegrationHelpUsesConfiguredKeys(t *testing.T) {
	m := NewModel()
	m.keybindings = map[string]string{"launch": "L", "next": "N", "previous": "P"}
	text := m.helpText()
	if !strings.Contains(text, "[L] Launch") || strings.Contains(text, "[c] Launch") || strings.Contains(text, "[j/down]") {
		t.Fatal("help displayed old bindings")
	}
}
