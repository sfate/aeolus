package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	store "github.com/sfate/aeolus/internal/prompts"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptLibrarySaveInsertAndCancel(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	b := &textBackend{}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m = key(m, t, "i")
	m = key(m, t, "draft 猫\nEnter Escape")
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if cmd == nil || !m.composer.library.pending {
		t.Fatal("load not asynchronous")
	}
	m, _ = updateModel(t, m, cmd())
	m = key(m, t, "s")
	m = key(m, t, "saved")
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("save missing")
	}
	m, _ = updateModel(t, m, cmd())
	path, _ := store.DefaultPath()
	entries, err := store.Load(path)
	if err != nil || len(entries) != 1 || entries[0].Text != "draft 猫\nEnter Escape" {
		t.Fatal(entries, err)
	}
	draft := m.composer.draft
	m = key(m, t, "enter")
	if m.composer.library.active || m.composer.draft != draft+draft || b.text != "" {
		t.Fatal("prompt was not inserted for review")
	}
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m, _ = updateModel(t, m, cmd())
	m = key(m, t, "esc")
	if !m.composer.active || m.composer.library.active {
		t.Fatal("cancel lost composer")
	}
}
func TestPromptLibraryCorruptAndStale(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path, _ := store.DefaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1")}
	m = key(m, t, "i")
	m = key(m, t, "draft")
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	result := cmd()
	m = key(m, t, "esc")
	m = key(m, t, "esc")
	m = key(m, t, "i")
	m, next := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m, _ = updateModel(t, m, result)
	if !m.composer.library.pending {
		t.Fatal("stale result applied")
	}
	m, _ = updateModel(t, m, next())
	if m.composer.library.err == nil || !strings.Contains(m.libraryView(), "Error") {
		t.Fatal("corruption hidden")
	}
}

func TestPromptLibraryDuplicateAndModalTargetGuards(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path, _ := store.DefaultPath()
	if err := store.Save(path, "saved", "text"); err != nil {
		t.Fatal(err)
	}
	b := &textBackend{}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m = key(m, t, "i")
	m = key(m, t, "draft")
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m, _ = updateModel(t, m, cmd())
	m = key(m, t, "s")
	m = key(m, t, "saved")
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = key(m, t, "esc")
	if !m.composer.library.pending || !m.composer.library.active {
		t.Fatal("save cancellation falsely implied no write")
	}
	m, _ = updateModel(t, m, cmd())
	if m.composer.library.err == nil || m.composer.draft != "draft" {
		t.Fatal("duplicate clobbered draft")
	}
	m = key(m, t, "esc")
	m.sessions[0].Provider = monitor.ProviderCodex
	m = key(m, t, "enter")
	if !m.composer.library.active || m.composer.draft != "draft" || b.text != "" {
		t.Fatal("changed target accepted insertion")
	}
	m = key(m, t, "esc")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if cmd != nil || m.composer.library.active || !m.composer.context.active {
		t.Fatal("library opened over file attachment")
	}
	m = key(m, t, "esc")
	m.composer.pending = true
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if cmd != nil || m.composer.library.active {
		t.Fatal("library opened during send")
	}
}
