package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	store "github.com/sfate/aeolus/internal/layouts"
	"github.com/sfate/aeolus/internal/monitor"
)

func TestLayoutCanBeCreatedSearchedAndPreviewedWithoutLaunching(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	backend := &fakeBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{shellSession()}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = openPalette(t, m)
	m = key(m, t, "Browse tmux layouts")
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.layouts.active {
		t.Fatal("palette did not open local layouts")
	}
	m, _ = updateModel(t, m, cmd())
	m = key(m, t, "N")
	m = key(m, t, "ctrl+u")
	m = key(m, t, `{"name":"Review","directory":"/tmp/project","windows":[{"name":"Code","panes":[{"type":"agent","provider":"codex"},{"type":"shell","split":"vertical"}]}]}`)
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil || !m.layouts.saving {
		t.Fatal("Ctrl+S did not save a valid layout")
	}
	m, _ = updateModel(t, m, cmd())
	if len(m.layouts.layouts) != 1 || m.layouts.layouts[0].Name != "Review" {
		t.Fatalf("layout was not saved: %+v", m.layouts)
	}
	m = key(m, t, "review")
	if len(m.layouts.matches()) != 1 {
		t.Fatal("fuzzy search did not find saved layout")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	body := m.layoutBody()
	if !m.layouts.viewing || !strings.Contains(body, "Preview only") || !strings.Contains(body, "vertical") || backend.killed != "" {
		t.Fatalf("layout preview or launch boundary failed: %q", body)
	}
	m = key(m, t, "esc")
	if m.layouts.viewing {
		t.Fatal("Escape did not return to layout list")
	}
}

func TestInvalidLayoutEditorDoesNotSave(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := NewModelWithBackend(&fakeBackend{}, &fakeMonitor{})
	m, _ = openLayouts(t, m)
	m = key(m, t, "N")
	m = key(m, t, "ctrl+u")
	m = key(m, t, `{"name":"bad","directory":"relative","windows":[]}`)
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || m.layouts.saving || m.layouts.err == "" {
		t.Fatal("invalid layout started a save")
	}
	path, _ := store.DefaultPath()
	items, err := store.Load(path)
	if err != nil || len(items) != 0 {
		t.Fatalf("invalid layout reached storage: %#v %v", items, err)
	}
}

func openLayouts(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	cmd := m.openLayoutBrowser()
	if cmd == nil || !m.layouts.loading {
		t.Fatal("layout browser did not load")
	}
	m, _ = updateModel(t, m, cmd())
	return m, nil
}
