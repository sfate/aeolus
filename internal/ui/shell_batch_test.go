package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
)

type batchBackend struct {
	fakeBackend
	calls []string
	errs  map[string]error
}

func (b *batchBackend) SendText(id, text string) error {
	b.calls = append(b.calls, id+":"+text)
	return b.errs[id]
}

func TestShellBatchRequiresReviewAndReportsEachTarget(t *testing.T) {
	first, second := testSession("%1"), testSession("%2")
	for _, s := range []*monitor.Session{&first, &second} {
		s.Kind, s.Pane.Shell = monitor.SessionShell, true
	}
	b := &batchBackend{fakeBackend: fakeBackend{panes: []tmux.Pane{first.Pane, second.Pane}}, errs: map[string]error{"%2": errors.New("offline")}}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m.sessions = []monitor.Session{first, second}
	m.cursor = 0
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlK})
	m.palette.query = "Send command to shells"
	m.selectPaletteFirst()
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.shellBatch.active || len(m.shellBatch.targets) != 2 || !m.shellBatch.targets[0].selected || m.shellBatch.targets[1].selected {
		t.Fatalf("unexpected batch selection: %+v", m.shellBatch)
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = key(m, t, "enter")
	m = key(m, t, "printf hi")
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || m.shellBatch.mode != shellBatchReview || len(b.calls) != 0 {
		t.Fatalf("command should be previewed without sending: cmd=%v mode=%v err=%v command=%q", cmd != nil, m.shellBatch.mode, m.shellBatch.err, m.shellBatch.command)
	}
	if !strings.Contains(m.shellBatchView(), "pane %1, session-1/window-1") || !strings.Contains(m.shellBatchView(), "pane %2, session-2/window-2") {
		t.Fatalf("review omitted a selected target: targets=%+v view=%q", m.shellBatch.targets, m.shellBatchView())
	}
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil || len(b.calls) != 0 {
		t.Fatal("confirmation must start an asynchronous send")
	}
	m, _ = updateModel(t, m, cmd())
	if len(b.calls) != 2 || m.shellBatch.mode != shellBatchDone || m.shellBatch.results[0].err != nil || m.shellBatch.results[1].err == nil {
		t.Fatalf("batch results: calls=%v state=%+v", b.calls, m.shellBatch)
	}
}

func TestShellBatchReviewCanCancelWithN(t *testing.T) {
	s := testSession("%1")
	s.Kind, s.Pane.Shell = monitor.SessionShell, true
	m := NewModelWithBackend(&batchBackend{}, &fakeMonitor{})
	m.shellBatch = newShellBatch([]monitor.Session{s}, "%1")
	m.shellBatch.mode = shellBatchReview
	m.shellBatch.command = "echo no"
	m = key(m, t, "n")
	if m.shellBatch.active {
		t.Fatal("n did not cancel the reviewed command")
	}
}

func TestShellBatchCancelAndStaleTargetSendNothing(t *testing.T) {
	s := testSession("%1")
	s.Kind, s.Pane.Shell = monitor.SessionShell, true
	b := &batchBackend{fakeBackend: fakeBackend{panes: []tmux.Pane{s.Pane}}}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m.sessions = []monitor.Session{s}
	m.shellBatch = newShellBatch([]monitor.Session{s}, "%1")
	m = key(m, t, "enter")
	m = key(m, t, "draft")
	m = key(m, t, "ctrl+s")
	m = key(m, t, "esc")
	m = key(m, t, "esc")
	m = key(m, t, "esc")
	if len(b.calls) != 0 || m.shellBatch.active {
		t.Fatal("cancel sent text or left the modal open")
	}
	m.shellBatch = newShellBatch([]monitor.Session{s}, "%1")
	m.shellBatch.mode = shellBatchReview
	m.shellBatch.command = "dangerous"
	b.panes = nil
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("confirmation did not start live validation")
	}
	m, _ = updateModel(t, m, cmd())
	if len(b.calls) != 0 || m.shellBatch.mode != shellBatchDone || m.shellBatch.err == nil {
		t.Fatalf("stale target was not rejected: calls=%v state=%+v", b.calls, m.shellBatch)
	}
}
