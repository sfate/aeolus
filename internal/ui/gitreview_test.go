package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/gitreview"
	"github.com/sfate/aeolus/internal/monitor"
)

func reviewKey(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()
	return updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func TestGitReviewBoundDirectoryAndModalActions(t *testing.T) {
	backend := &fakeBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	s := testSession("%1")
	s.Pane.CurrentPath = "/chosen/project"
	s.Status = monitor.StatusPermission
	m.sessions = []monitor.Session{s}
	calls := []string{}
	m.review.read = func(_ context.Context, directory string) (gitreview.Snapshot, error) {
		calls = append(calls, directory)
		return gitreview.Snapshot{Directory: directory, UnstagedDiff: strings.Repeat("diff line\n", 100)}, nil
	}
	m, cmd := reviewKey(t, m, "g")
	if !m.review.active || !m.review.loading || cmd == nil {
		t.Fatal("review did not load asynchronously")
	}
	s.Pane.CurrentPath = "/other/project"
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{s}})
	m, _ = updateModel(t, m, cmd())
	if len(calls) != 1 || calls[0] != "/chosen/project" {
		t.Fatalf("review changed directory: %v", calls)
	}
	for _, key := range []string{"y", "n", "a", "x", "i", "f", "c", "R", "q"} {
		m, _ = reviewKey(t, m, key)
	}
	if backend.sent != "" || m.confirmKill != "" || m.composer.active || m.filters.editing || m.lifecycle.kind != lifecycleNone || m.quitting {
		t.Fatal("review leaked action shortcuts")
	}
	m, _ = reviewKey(t, m, "j")
	offset := m.review.viewport.YOffset
	if offset == 0 || m.cursor != 0 {
		t.Fatal("review scrolling changed selection or did not scroll")
	}
	m, _ = updateModel(t, m, refreshMsg{sessions: nil})
	if m.review.viewport.YOffset != offset {
		t.Fatal("session refresh reset review scroll")
	}
	m, cmd = reviewKey(t, m, "r")
	m, _ = updateModel(t, m, cmd())
	if len(calls) != 2 || calls[1] != "/chosen/project" {
		t.Fatalf("reload lost binding: %v", calls)
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.review.active {
		t.Fatal("review did not close")
	}
}

func TestGitReviewCancellationAndStaleResults(t *testing.T) {
	m := NewModel()
	s := testSession("%1")
	s.Pane.CurrentPath = "/project"
	m.sessions = []monitor.Session{s}
	var canceled bool
	m.review.read = func(ctx context.Context, _ string) (gitreview.Snapshot, error) {
		canceled = ctx.Err() != nil
		return gitreview.Snapshot{}, ctx.Err()
	}
	m, old := reviewKey(t, m, "g")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	oldMsg := old()
	if !canceled {
		t.Fatal("close did not cancel command context")
	}
	m, _ = reviewKey(t, m, "g")
	m, _ = updateModel(t, m, oldMsg)
	if !m.review.active || !m.review.loading {
		t.Fatal("stale result replaced newer review")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
}

func TestGitReviewRequiresPaneDirectory(t *testing.T) {
	m := NewModel()
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m.sessions = []monitor.Session{testSession("%1")}
	m, cmd := reviewKey(t, m, "g")
	m, _ = updateModel(t, m, cmd())
	if !strings.Contains(m.review.viewport.View(), "working directory is unavailable") {
		t.Fatalf("missing actionable error: %s", m.review.viewport.View())
	}
}

func TestGitReviewRespectsExistingModal(t *testing.T) {
	for _, mode := range []string{"composer", "filter", "lifecycle", "search", "kill"} {
		t.Run(mode, func(t *testing.T) {
			m := NewModel()
			m.sessions = []monitor.Session{testSession("%1")}
			switch mode {
			case "composer":
				m.composer.active = true
			case "filter":
				m.filters.editing = true
			case "lifecycle":
				m.lifecycle.kind = lifecycleRename
			case "search":
				m.search.editing = true
			case "kill":
				m.confirmKill = "%1"
			}
			m, _ = reviewKey(t, m, "g")
			if m.review.active {
				t.Fatal("review opened over existing modal")
			}
		})
	}
}

func TestGitReviewStripsTerminalControls(t *testing.T) {
	input := "normal\x1b[31mred\x1b[0m\x1b]52;c;bad\a\r\x00\x7f\n\tend"
	safe := safeReviewText(input)
	if strings.ContainsAny(safe, "\x1b\a\r\x00\x7f") || !strings.Contains(safe, "normalred") {
		t.Fatalf("unsafe text %q", safe)
	}
	content := reviewSnapshotText(gitreview.Snapshot{Directory: "/repo", Files: []gitreview.File{{Status: "??", Path: "line\n空"}}, UnstagedDiff: input, Truncated: true})
	if !strings.Contains(content, `"line\n空"`) || !strings.Contains(content, "Output truncated") {
		t.Fatalf("missing literal path/limit: %s", fmt.Sprint(content))
	}
}

func TestGitReviewAndFileAttachmentIsolation(t *testing.T) {
	m := NewModel()
	s := testSession("%1")
	s.Pane.CurrentPath = t.TempDir()
	m.sessions = []monitor.Session{s}
	m = key(m, t, "i")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	m = key(m, t, "g")
	if m.review.active || !m.composer.context.active || m.composer.context.path != "g" {
		t.Fatal("attachment path opened Git review")
	}
	m = key(m, t, "esc")
	m = key(m, t, "esc")
	m = key(m, t, "g")
	if !m.review.active {
		t.Fatal("Git review did not open after cancelling composer")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlO})
	m = key(m, t, "i")
	if m.composer.active {
		t.Fatal("Git review opened composer")
	}
	m = key(m, t, "esc")
}
