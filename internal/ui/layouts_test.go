package ui

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	store "github.com/sfate/aeolus/internal/layouts"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
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
	if !m.layouts.viewing || !strings.Contains(body, "Press l to launch") || !strings.Contains(body, "vertical") || backend.killed != "" {
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

type fakeLayoutBackend struct {
	fakeBackend
	result tmux.LayoutResult
	err    error
	calls  int
	layout store.Layout
}

func (f *fakeLayoutBackend) CreateLayout(layout store.Layout) (tmux.LayoutResult, error) {
	f.calls++
	f.layout = layout
	return f.result, f.err
}

func savedLayoutForTest(t *testing.T) store.Layout {
	t.Helper()
	layout := store.Layout{Name: "Review", Directory: "/tmp/project", Windows: []store.Window{{Name: "Code", Panes: []store.Pane{{Type: store.Agent, Provider: "codex"}, {Type: store.Shell, Split: store.Vertical}}}}}
	path, err := store.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(path, layout); err != nil {
		t.Fatal(err)
	}
	return layout
}
func openLayoutPreview(t *testing.T, m Model) (Model, store.Layout) {
	t.Helper()
	layout := savedLayoutForTest(t)
	m, _ = openLayouts(t, m)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.layouts.viewing {
		t.Fatal("saved layout did not open")
	}
	return m, layout
}
func launchLayoutInput(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m = key(m, t, "l")
	if !m.layouts.confirmRun {
		t.Fatal("launch request did not ask for confirmation")
	}
	return updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
}

func TestLayoutLaunchRequiresConfirmationAndSupportsCancel(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	backend := &fakeLayoutBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m, _ = openLayoutPreview(t, m)
	m = key(m, t, "l")
	if !m.layouts.confirmRun || backend.calls != 0 {
		t.Fatal("launch began without confirmation")
	}
	m = key(m, t, "n")
	if m.layouts.confirmRun || backend.calls != 0 {
		t.Fatal("cancel did not stop launch")
	}
}

func TestConfirmedLayoutLaunchAddsAndSelectsCreatedPanes(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	backend := &fakeLayoutBackend{result: tmux.LayoutResult{SessionName: "aeolus-test", SessionID: "$1", Panes: []tmux.Pane{{PaneID: "%1", SessionID: "$1", SessionName: "aeolus-test", WindowName: "Code", CurrentCommand: "/bin/codex"}, {PaneID: "%2", SessionID: "$1", SessionName: "aeolus-test", WindowName: "Code", CurrentCommand: "zsh", Shell: true}}}}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%old")}
	m.allSessions = append([]monitor.Session{}, m.sessions...)
	m.filters.applied = filterChoice{provider: 1}
	m, _ = openLayoutPreview(t, m)
	m, cmd := launchLayoutInput(t, m)
	if cmd == nil || !m.layouts.launching || backend.calls != 0 {
		t.Fatal("confirmed launch did not start asynchronously")
	}
	m, refreshCmd := updateModel(t, m, cmd())
	if refreshCmd == nil || backend.calls != 1 {
		t.Fatalf("layout launch did not finish or request refresh: cmd=%v calls=%d active=%v notice=%q", refreshCmd, backend.calls, m.layouts.active, m.layoutNotice)
	}
	if m.layouts.active || len(m.sessions) != 3 || len(m.allSessions) != 3 || m.sessions[m.cursor].Pane.PaneID != "%1" || m.filters.applied != (filterChoice{}) {
		t.Fatalf("created panes were not added, unfiltered, and selected: cursor=%d sessions=%+v", m.cursor, m.sessions)
	}
	if m.sessions[1].Provider != monitor.ProviderCodex || m.sessions[2].Kind != monitor.SessionShell {
		t.Fatalf("pane kinds were not mapped: %+v", m.sessions)
	}
	if !strings.Contains(m.layoutNotice, "Code/%1") || !strings.Contains(m.layoutNotice, "First pane selected") {
		t.Fatalf("launch summary missing: %s", m.layoutNotice)
	}
}

func TestLayoutLaunchReportsPartialPanesAndKeepsThem(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	backend := &fakeLayoutBackend{fakeBackend: fakeBackend{panes: []tmux.Pane{{PaneID: "%9", SessionName: "aeolus-test", WindowName: "Code", CurrentCommand: "codex"}, {PaneID: "%10", SessionName: "aeolus-test", WindowName: "Code", CurrentCommand: "zsh", Shell: true}}}, result: tmux.LayoutResult{SessionName: "aeolus-test", Panes: []tmux.Pane{{PaneID: "%9", SessionName: "aeolus-test", WindowName: "Code", CurrentCommand: "codex"}}}, err: &tmux.LayoutLaunchError{Window: "Code", Pane: 2, Err: errors.New("tmux split failed")}}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m, _ = openLayoutPreview(t, m)
	m, cmd := launchLayoutInput(t, m)
	if cmd == nil {
		t.Fatal("partial launch did not start")
	}
	m, _ = updateModel(t, m, cmd())
	if m.layouts.active || len(m.sessions) != 2 || m.cursor != 0 || m.sessions[0].Pane.PaneID != "%9" || m.sessions[1].Pane.PaneID != "%10" {
		t.Fatalf("partial pane was not retained and selected: %+v", m.sessions)
	}
	if !strings.Contains(m.layoutNotice, "partially created") || !strings.Contains(m.layoutNotice, "pane 2") || !strings.Contains(m.layoutNotice, "%10") {
		t.Fatalf("partial failure details missing: %s", m.layoutNotice)
	}
}

func TestLayoutLaunchRejectsSavedLayoutChangedAfterPreview(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	backend := &fakeLayoutBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m, _ = openLayoutPreview(t, m)
	path, _ := store.DefaultPath()
	replacement := `{"version":1,"layouts":[{"name":"Review","directory":"/tmp/changed","windows":[{"name":"Code","panes":[{"type":"agent","provider":"codex"},{"type":"shell","split":"vertical"}]}]}]}`
	if err := os.WriteFile(path, []byte(replacement), 0600); err != nil {
		t.Fatal(err)
	}
	m, cmd := launchLayoutInput(t, m)
	if cmd == nil {
		t.Fatal("stale validation did not run")
	}
	m, _ = updateModel(t, m, cmd())
	if backend.calls != 0 || !m.layouts.active || !strings.Contains(m.layouts.err, "changed after preview") {
		t.Fatalf("stale layout launched or error not shown: calls=%d err=%s", backend.calls, m.layouts.err)
	}
}
