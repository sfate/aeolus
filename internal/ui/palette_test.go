package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
)

func openPalette(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlK})
	if !m.palette.active {
		t.Fatal("Ctrl+K did not open palette")
	}
	return m
}

func TestPaletteFiltersAndUsesExistingKillConfirmation(t *testing.T) {
	backend := &fakeBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m = openPalette(t, m)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("KiLl")})
	if actions := m.paletteActions(); len(actions) != 1 || actions[0].key != "x" {
		t.Fatalf("matches %#v", actions)
	}
	if !strings.Contains(m.View(), "Kill pane") {
		t.Fatal("filtered action missing")
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.palette.active || m.confirmKill != "%1" || cmd != nil || backend.killed != "" {
		t.Fatal("palette bypassed confirmation")
	}
}

func TestPaletteCapabilitiesAndProviderGating(t *testing.T) {
	m := NewModelWithBackend(&fakeBackend{}, &fakeMonitor{})
	s := testSession("%1")
	s.Status = monitor.StatusPermission
	m.sessions = []monitor.Session{s}
	m = openPalette(t, m)
	labels := func(m Model) string {
		var text strings.Builder
		for _, a := range m.paletteActions() {
			text.WriteString(a.label + "\n")
		}
		return text.String()
	}
	if got := labels(m); strings.Contains(got, "Launch agent") || strings.Contains(got, "Rename") || strings.Contains(got, "Compose") || !strings.Contains(got, "Approve for session") {
		t.Fatalf("capabilities %s", got)
	}
	for _, provider := range []monitor.Provider{monitor.ProviderCodex, monitor.ProviderGemini} {
		s.Provider = provider
		m.sessions[0] = s
		m.palette.provider = provider
		if got := labels(m); strings.Contains(got, "Approve") || strings.Contains(got, "Deny") {
			t.Fatalf("unsupported provider actions %s", got)
		}
	}
}

func TestPaletteStaleTargetCannotExecuteAnotherSession(t *testing.T) {
	backend := &fakeBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m = openPalette(t, m)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Switch")})
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{testSession("%2")}})
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || backend.switched != "" || !m.palette.active {
		t.Fatal("stale palette acted on replacement session")
	}
}

func TestPaletteRemovedApprovalCannotFallThroughToAnotherAction(t *testing.T) {
	backend := &fakeBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	s := testSession("%1")
	s.Status = monitor.StatusPermission
	m.sessions = []monitor.Session{s}
	m = openPalette(t, m)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Approve for")})
	s.Status = monitor.StatusIdle
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{s}})
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || backend.sent != "" || !m.palette.active {
		t.Fatal("removed approval remained executable")
	}
}

func TestPaletteUsesExistingHelpHandler(t *testing.T) {
	m := openPalette(t, NewModel())
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.palette.active || !m.help.active {
		t.Fatal("help action did not use existing handler")
	}
}

func TestPaletteKeyboardEditingIsolationAndEmptyResults(t *testing.T) {
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1")}
	m = openPalette(t, m)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("qyn/gif空")})
	if m.quitting || m.review.active || m.composer.active || m.filters.editing || m.confirmKill != "" {
		t.Fatal("palette text leaked")
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !m.palette.active {
		t.Fatal("empty results executed action")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.palette.query != "qyn/gif" {
		t.Fatal("Unicode editing failed")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.palette.selected == "" {
		t.Fatal("arrow navigation lost selection")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.palette.active {
		t.Fatal("Escape did not close palette")
	}
	m, _ = updateModel(t, m, helpKey('i'))
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.palette.active || !m.composer.active {
		t.Fatal("palette interrupted composer")
	}
}
