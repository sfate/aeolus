package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestLoadContextFile(t *testing.T) {
	base := t.TempDir()
	name := "a 猫 \"name.txt"
	want := "Enter Escape 猫\nsecond\tline"
	if err := os.WriteFile(filepath.Join(base, name), []byte(want), 0600); err != nil {
		t.Fatal(err)
	}
	path, text, err := loadContextFile(base, name)
	if err != nil || path != filepath.Join(base, name) || text != want {
		t.Fatalf("path=%q text=%q err=%v", path, text, err)
	}
	if _, _, err = loadContextFile("", name); err == nil {
		t.Fatal("relative path guessed cwd")
	}
	if _, _, err = loadContextFile("", filepath.Join(base, name)); err != nil {
		t.Fatal(err)
	}
}
func TestLoadContextFileValidation(t *testing.T) {
	base := t.TempDir()
	for name, data := range map[string][]byte{"invalid": {255}, "control": []byte("bad\x1b[0m"), "large": []byte(strings.Repeat("a", 64*1024+1)), "limit": []byte(strings.Repeat("a", 64*1024))} {
		if err := os.WriteFile(filepath.Join(base, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := loadContextFile(base, name)
		if (err == nil) != (name == "limit") {
			t.Fatalf("%s err=%v", name, err)
		}
	}
	for _, name := range []string{"missing", "."} {
		if _, _, err := loadContextFile(base, name); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
}

func TestContextModalReviewCancelAndErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "猫 file.txt")
	if err := os.WriteFile(file, []byte("literal Enter Escape\n猫"), 0600); err != nil {
		t.Fatal(err)
	}
	b := &textBackend{}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m = key(m, t, "i")
	m = key(m, t, "draft")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	m = key(m, t, file)
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.composer.context.pending {
		t.Fatal("read was not asynchronous")
	}
	m, send := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if send != nil || b.text != "" {
		t.Fatal("sent while attaching")
	}
	m, _ = updateModel(t, m, cmd())
	if m.composer.context.active || !strings.Contains(m.composer.draft, "literal Enter Escape\n猫") || b.text != "" {
		t.Fatal("attachment not inserted for review")
	}
	draft := m.composer.draft
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	m = key(m, t, "missing")
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, cmd())
	if m.composer.context.err == nil || m.composer.draft != draft {
		t.Fatal("read error lost draft")
	}
	m = key(m, t, "esc")
	if !m.composer.active || m.composer.context.active || m.composer.draft != draft {
		t.Fatal("cancel lost composer")
	}
}
func TestContextStaleReadAndRemovedTarget(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("contents"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1")}
	m = key(m, t, "i")
	m = key(m, t, "draft")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	m = key(m, t, file)
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	result := cmd()
	m = key(m, t, "esc")
	m = key(m, t, "esc")
	m = key(m, t, "i")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	m = key(m, t, file)
	m, next := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if next == nil {
		t.Fatal("second read missing")
	}
	m, _ = updateModel(t, m, result)
	if !m.composer.context.pending || m.composer.draft != "" {
		t.Fatal("stale result inserted")
	}
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{testSession("%2")}})
	m, _ = updateModel(t, m, next())
	if m.composer.draft != "" || m.composer.context.err == nil {
		t.Fatal("removed target accepted attachment")
	}
}

func TestContextRejectsFIFOAndFollowsRegularSymlink(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadContextFile("", fifo); err == nil {
		t.Fatal("FIFO accepted")
	}
	file := filepath.Join(dir, "regular")
	if err := os.WriteFile(file, []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, text, err := loadContextFile("", link); err != nil || text != "text" {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestContextRelativePathStaysOnOriginalPane(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "relative.txt"), []byte("correct target file"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewModel()
	first := testSession("%1")
	first.Pane.CurrentPath = base
	m.sessions = []monitor.Session{first, testSession("%2")}
	m = key(m, t, "i")
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{testSession("%2"), first}})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	m = key(m, t, "relative.txt")
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateModel(t, m, cmd())
	if !strings.Contains(m.composer.draft, "correct target file") || m.composer.context.err != nil {
		t.Fatal("relative file used incorrect pane directory")
	}
	m.composer.pending = true
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.composer.context.active {
		t.Fatal("attachment opened during pending send")
	}
}
