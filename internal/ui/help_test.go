package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
)

func helpKey(key rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}} }

func TestHelpOpensScrollsAndIsolatesActions(t *testing.T) {
	backend := &fakeBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1"), testSession("%2")}
	m.sessions[0].Status = monitor.StatusPermission
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 16})
	m, cmd := updateModel(t, m, helpKey('?'))
	if cmd != nil || !m.help.active || !strings.Contains(m.View(), "Keyboard help") {
		t.Fatal("? did not open help")
	}
	for _, key := range []rune{'y', 'a', 'n', 'x', 'c', 'R', 'g', 'f', 'i', '/', 'r', 'q'} {
		m, cmd = updateModel(t, m, helpKey(key))
		if cmd != nil || m.cursor != 0 || m.confirmKill != "" || m.quitting || m.review.active || m.composer.active || m.filters.editing || m.lifecycle.kind != lifecycleNone || backend.sent != "" || backend.switched != "" {
			t.Fatalf("help leaked action %q", key)
		}
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.help.viewport.YOffset == 0 {
		t.Fatal("help did not scroll")
	}
	offset := m.help.viewport.YOffset
	m, _ = updateModel(t, m, refreshMsg{sessions: m.sessions})
	if m.help.viewport.YOffset != offset {
		t.Fatal("refresh reset help scrolling")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyHome})
	if m.help.viewport.YOffset != 0 {
		t.Fatal("home did not return to top")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	if !m.help.viewport.AtBottom() {
		t.Fatal("end did not reach bottom")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.help.active || m.cursor != 0 {
		t.Fatal("Escape did not restore dashboard")
	}
}

func TestHelpDoesNotInterruptExistingModalsAndCanQuit(t *testing.T) {
	for _, mode := range []string{"kill", "review", "filters"} {
		m := NewModel()
		m.sessions = []monitor.Session{testSession("%1")}
		switch mode {
		case "kill":
			m.confirmKill = "%1"
		case "review":
			m.review.active = true
		case "filters":
			m.filters.editing = true
		}
		m, _ = updateModel(t, m, helpKey('?'))
		if m.help.active {
			t.Fatalf("help interrupted %s", mode)
		}
	}
	m := NewModel()
	m, _ = updateModel(t, m, helpKey('?'))
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.quitting || cmd == nil {
		t.Fatal("Ctrl+C did not quit help")
	}
}

func TestHelpProviderActionsTrackRefresh(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	session := testSession("%1")
	session.Status = monitor.StatusPermission
	m.sessions = []monitor.Session{session}
	m, _ = updateModel(t, m, helpKey('?'))
	if !strings.Contains(m.helpText(), "[a] Approve for session") {
		t.Fatal("Claude permission actions missing")
	}
	session.Status = monitor.StatusQuestion
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{session}})
	if strings.Contains(m.helpText(), "[a]") || !strings.Contains(m.helpText(), "[y] Answer yes") {
		t.Fatal("question actions inaccurate")
	}
	if !strings.Contains(m.help.viewport.View(), "Answer yes") {
		t.Fatal("visible help context did not refresh")
	}
	for _, provider := range []monitor.Provider{monitor.ProviderClaude, monitor.ProviderCodex, monitor.ProviderGemini} {
		session.Provider, session.Status = provider, monitor.StatusUnknown
		m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{session}})
		if strings.Contains(m.helpText(), "[y]") || strings.Contains(m.helpText(), "[a]") || !strings.Contains(m.helpText(), "Open the pane to inspect or respond") {
			t.Fatalf("unsupported actions advertised for %s", provider)
		}
	}
	m, _ = updateModel(t, m, refreshMsg{})
	if !strings.Contains(m.helpText(), "No session selected") {
		t.Fatal("removed session context retained")
	}
}

func TestHelpResizesAndPreservesExistingInputModes(t *testing.T) {
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1")}
	m, _ = updateModel(t, m, helpKey('?'))
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 44, Height: 12})
	if m.help.viewport.Width != 44 || m.help.viewport.Height != 8 {
		t.Fatalf("help dimensions %dx%d", m.help.viewport.Width, m.help.viewport.Height)
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	for _, mode := range []rune{'i', '/', 'c'} {
		current, _ := updateModel(t, m, helpKey(mode))
		current, _ = updateModel(t, current, helpKey('?'))
		if current.help.active {
			t.Fatalf("? hijacked %q input", mode)
		}
		switch mode {
		case 'i':
			if current.composer.draft != "?" {
				t.Fatal("composer lost ?")
			}
		case '/':
			if current.search.query != "?" {
				t.Fatal("search lost ?")
			}
		case 'c':
			if !strings.HasSuffix(current.lifecycle.text, "?") {
				t.Fatal("launch input lost ?")
			}
		}
	}
}
