package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/config"
	"github.com/sfate/aeolus/internal/monitor"
)

func TestProjectPresetsAndDefaultProvider(t *testing.T) {
	settings := config.Config{DefaultProvider: "codex", Projects: []config.Project{{Name: "雪", Directory: "/project/雪", Provider: "gemini"}, {Name: "second", Directory: "/second", Provider: "claude"}}}
	m := NewModelWithConfig(settings)
	settings.Projects[0].Name = "changed"
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if launchProviders[m.lifecycle.provider] != monitor.ProviderCodex || m.lifecycle.preset != -1 {
		t.Fatal("launch did not use default provider")
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if cmd != nil || m.lifecycle.text != "/project/雪" || launchProviders[m.lifecycle.provider] != monitor.ProviderGemini || m.projects[0].Name != "雪" {
		t.Fatal("preset cycle/copy failed")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.lifecycle.text != "/second" {
		t.Fatal("second preset missing")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.lifecycle.preset != -1 {
		t.Fatal("manual provider edit retained preset label")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.lifecycle.preset != -1 {
		t.Fatal("manual directory edit retained preset label")
	}
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || m.lifecycle.kind != lifecycleNone {
		t.Fatal("cancel performed an action")
	}
	m = NewModel()
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	old := m.lifecycle.text
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.lifecycle.text != old || launchProviders[m.lifecycle.provider] != monitor.ProviderClaude {
		t.Fatal("defaults/no presets regressed")
	}
}

func TestPresetLaunchUsesSelectedDirectoryAndAgent(t *testing.T) {
	backend := &lifecycleFake{}
	m := NewModelWithConfig(config.Config{Projects: []config.Project{{Name: "project", Directory: "/configured/path", Provider: "gemini"}}})
	m.backend = backend
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if cmd != nil || backend.directory != "" || backend.sent != "" {
		t.Fatal("selecting preset launched or sent input")
	}
	_, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submit did not return launch command")
	}
	_ = cmd()
	if backend.directory != "/configured/path" || backend.command != "gemini" {
		t.Fatalf("backend=%#v", backend)
	}
}
