package ui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"testing"
)

type textBackend struct {
	fakeBackend
	target, text string
	err          error
}

func (b *textBackend) SendText(id, text string) error { b.target = id; b.text = text; return b.err }
func TestComposerLiteralSendAndPending(t *testing.T) {
	b := &textBackend{}
	m := NewModelWithBackend(b, &fakeMonitor{})
	s := testSession("%7")
	s.Provider = monitor.ProviderCodex
	s.Status = monitor.StatusUnknown
	m.sessions = []monitor.Session{s}
	m = key(m, t, "i")
	m = key(m, t, "Enter Escape 猫")
	m = key(m, t, "enter")
	m = key(m, t, "ynaxq")
	if b.sent != "" || b.killed != "" || m.quitting {
		t.Fatal("composer invoked shortcuts")
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil || !m.composer.pending || b.text != "" {
		t.Fatal("send not asynchronous")
	}
	m, again := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if again != nil {
		t.Fatal("duplicate send")
	}
	msg := cmd()
	m, _ = updateModel(t, m, msg)
	if b.target != "%7" || b.text != "Enter Escape 猫\nynaxq" || m.composer.active {
		t.Fatalf("target=%q text=%q composer=%+v", b.target, b.text, m.composer)
	}
}
func TestComposerFailureAndRemovedTarget(t *testing.T) {
	b := &textBackend{err: errors.New("failed")}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1"), testSession("%2")}
	m = key(m, t, "i")
	m = key(m, t, "draft")
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{testSession("%2"), testSession("%1")}})
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, _ = updateModel(t, m, cmd())
	if m.composer.draft != "draft" || m.composer.err == nil || b.target != "%1" {
		t.Fatal("failed delivery lost draft or target")
	}
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{testSession("%2")}})
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || m.composer.err == nil {
		t.Fatal("removed target accepted")
	}
	m = key(m, t, "esc")
	if m.composer.active {
		t.Fatal("cancel remained active")
	}
}

func TestComposerChangedProviderAndUnicode(t *testing.T) {
	b := &textBackend{}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m = key(m, t, "i")
	m = key(m, t, "猫犬")
	m = key(m, t, "left")
	m = key(m, t, "backspace")
	if m.composer.draft != "犬" {
		t.Fatal("Unicode edit corrupted draft")
	}
	m.sessions[0].Provider = monitor.ProviderCodex
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || m.composer.err == nil {
		t.Fatal("changed provider accepted")
	}
	m = key(m, t, "esc")
	m = key(m, t, "i")
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil || m.composer.err == nil {
		t.Fatal("empty prompt accepted")
	}
}
