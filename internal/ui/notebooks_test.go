package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	store "github.com/sfate/aeolus/internal/notebooks"
)

func notebookSession() monitor.Session { return shellSession() }

func openNotebooks(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	cmd := m.openNotebookBrowser()
	if cmd == nil || !m.notebooks.active || !m.notebooks.loading {
		t.Fatal("notebook browser did not start loading")
	}
	m, _ = updateModel(t, m, cmd())
	return m, nil
}

func TestNotebookPaletteSearchAndOpenNeverRunsBlocks(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path, _ := store.DefaultPath()
	document, err := store.ParseMarkdown("Deploy runbook", "# Deploy\n\nReview first.\n\n```sh\necho deploy\n```")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(path, document); err != nil {
		t.Fatal(err)
	}
	backend := &textBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{notebookSession()}
	m = openPalette(t, m)
	m = key(m, t, "Browse local notebooks")
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.notebooks.active {
		t.Fatal("palette did not open local notebooks")
	}
	m, _ = updateModel(t, m, cmd())
	m = key(m, t, "deploy")
	if matches := m.notebooks.matches(); len(matches) != 1 || m.notebooks.documents[matches[0]].Title != "Deploy runbook" {
		t.Fatalf("fuzzy search did not find notebook: %#v", matches)
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.notebooks.viewing || backend.text != "" || !strings.Contains(m.notebookDocumentBody(document), "Runnable shell block") {
		t.Fatal("opening or selecting a notebook ran a command")
	}
}

func TestNotebookCanBeCreatedAndSavedFromTheTUI(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	backend := &textBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{notebookSession()}
	m, _ = openNotebooks(t, m)
	m = key(m, t, "N")
	m = key(m, t, "Deploy safely")
	m = key(m, t, "enter")
	m = key(m, t, "Notes before running.\n\n```sh\nprintf 'ready'\n```")
	if backend.text != "" || !m.notebooks.creating {
		t.Fatal("editing a notebook ran its command")
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil || !m.notebooks.saving {
		t.Fatal("Ctrl+S did not start a local save")
	}
	m, _ = updateModel(t, m, cmd())
	if m.notebooks.creating || len(m.notebooks.documents) != 1 || backend.text != "" {
		t.Fatalf("save did not return to notebook list: %+v", m.notebooks)
	}
	path, _ := store.DefaultPath()
	documents, err := store.Load(path)
	if err != nil || len(documents) != 1 || documents[0].Blocks[len(documents[0].Blocks)-1].Kind != store.Shell {
		t.Fatalf("TUI notebook save was not persisted: %#v, %v", documents, err)
	}
}

func TestNotebookCommandNeedsConfirmationAndRechecksTarget(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path, _ := store.DefaultPath()
	document, _ := store.ParseMarkdown("Dangerous", "```sh\nprintf secret\n```")
	if err := store.Save(path, document); err != nil {
		t.Fatal(err)
	}
	backend := &textBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	s := notebookSession()
	m.sessions, m.allSessions = []monitor.Session{s}, []monitor.Session{s}
	m, _ = openNotebooks(t, m)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !m.notebooks.confirmRun || backend.text != "" {
		t.Fatal("selecting Run skipped confirmation")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.notebooks.confirmRun || backend.text != "" {
		t.Fatal("cancel did not prevent command execution")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m.allSessions = []monitor.Session{}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd != nil || backend.text != "" || !strings.Contains(m.notebooks.err, "changed") {
		t.Fatal("changed shell target received a notebook command")
	}
}

func TestNotebookCommandRunsOnlyAfterExplicitConfirmation(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path, _ := store.DefaultPath()
	document, _ := store.ParseMarkdown("Safe", "```sh\necho reviewed\n```")
	if err := store.Save(path, document); err != nil {
		t.Fatal(err)
	}
	backend := &textBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	s := notebookSession()
	m.sessions, m.allSessions = []monitor.Session{s}, []monitor.Session{s}
	m, _ = openNotebooks(t, m)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if backend.text != "" {
		t.Fatal("command ran before confirmation")
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil || backend.text != "" {
		t.Fatal("confirmation did not schedule an explicit run")
	}
	m, _ = updateModel(t, m, cmd())
	if backend.target != s.Pane.PaneID || backend.text != "echo reviewed" || m.notebooks.running || !strings.Contains(m.notebooks.notice, "submitted") {
		t.Fatalf("confirmed notebook command did not run against its target: target=%q text=%q state=%+v", backend.target, backend.text, m.notebooks)
	}
}
