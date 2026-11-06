package ui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/history"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/shellblocks"
	"github.com/sfate/aeolus/internal/tmux"
)

const testCaptureID = "0123456789abcdef0123456789abcdef"

func capturedSession(paneID, captureID string) monitor.Session {
	s := shellSession()
	s.Pane.PaneID = paneID
	s.Pane.CaptureID = captureID
	return s
}

func TestBlockBrowserReadsOnlySelectedCapture(t *testing.T) {
	m := NewModelWithBackend(&fakeBackend{}, &fakeMonitor{})
	m.sessions = []monitor.Session{capturedSession("%2", testCaptureID)}
	m.allSessions = append([]monitor.Session{}, m.sessions...)
	called := ""
	m.blockRead = func(id string) ([]shellblocks.Block, error) {
		called = id
		return []shellblocks.Block{{ID: 1, Command: "printf secret", Directory: "/tmp/project", StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Duration: time.Second, ExitCode: intPtr(0), Output: []byte("\x1b[31msecret output\x1b[0m")}}, nil
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	if !m.blocks.active || cmd == nil {
		t.Fatal("capture browser did not open")
	}
	m, _ = updateModel(t, m, cmd())
	if called != testCaptureID || !strings.Contains(m.viewport.View(), "printf secret") || !strings.Contains(m.viewport.View(), "secret output") || strings.Contains(m.viewport.View(), "\x1b[") {
		t.Fatalf("capture view mismatch: id=%q text=%q", called, m.viewport.View())
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.blocks.active {
		t.Fatal("capture browser did not close")
	}
}

func TestBlockBrowserRejectsInvalidAndChangedCapture(t *testing.T) {
	m := NewModelWithBackend(&fakeBackend{}, &fakeMonitor{})
	m.sessions = []monitor.Session{capturedSession("%2", "../../other")}
	m.allSessions = append([]monitor.Session{}, m.sessions...)
	reads := 0
	m.blockRead = func(string) ([]shellblocks.Block, error) {
		reads++
		return nil, nil
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	if cmd != nil || m.blocks.active || reads != 0 {
		t.Fatal("invalid capture ID reached the reader")
	}
	m.sessions = []monitor.Session{capturedSession("%2", testCaptureID)}
	m.allSessions = append([]monitor.Session{}, m.sessions...)
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	if cmd == nil || !m.blocks.active {
		t.Fatal("valid capture did not start loading")
	}
	m, _ = updateModel(t, m, refreshMsg{sessions: []monitor.Session{capturedSession("%2", "")}})
	if m.blocks.active {
		t.Fatal("pane refresh retained a stale capture view")
	}
	_, _ = updateModel(t, m, cmd())
	if reads != 1 {
		t.Fatalf("unexpected read count %d", reads)
	}
}

func TestBlockBrowserShowsReadErrors(t *testing.T) {
	m := NewModelWithBackend(&fakeBackend{}, &fakeMonitor{})
	s := capturedSession("%2", testCaptureID)
	m.sessions, m.allSessions = []monitor.Session{s}, []monitor.Session{s}
	m.blockRead = func(string) ([]shellblocks.Block, error) { return nil, errors.New("snapshot missing") }
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	m, _ = updateModel(t, m, cmd())
	if !strings.Contains(m.viewport.View(), "snapshot missing") {
		t.Fatal("capture read error not shown")
	}
}

func TestBlockHistorySaveIsExplicitAndExcludesOutput(t *testing.T) {
	m := NewModelWithBackend(&fakeBackend{}, &fakeMonitor{})
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	s := capturedSession("%2", testCaptureID)
	m.sessions, m.allSessions = []monitor.Session{s}, []monitor.Session{s}
	code := 7
	m.blockRead = func(string) ([]shellblocks.Block, error) {
		return []shellblocks.Block{{ID: 1, Command: "dangerous --secret=value", Directory: "/tmp/project", ExitCode: &code, Duration: 2 * time.Second, StartedAt: time.Now(), Output: []byte("private terminal output")}}, nil
	}
	saved := []history.Entry{}
	m.historyWrite = func(entry history.Entry) (int, error) {
		saved = append(saved, entry)
		return 0, nil
	}
	if len(saved) != 0 {
		t.Fatal("loading blocks saved history automatically")
	}
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	m, _ = updateModel(t, m, cmd())
	if len(saved) != 0 {
		t.Fatal("opening the block browser saved history automatically")
	}
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil || len(saved) != 0 || !m.historySaving {
		t.Fatal("explicit save did not schedule one history write")
	}
	m, _ = updateModel(t, m, cmd())
	if len(saved) != 1 || saved[0].Command != "dangerous --secret=value" || saved[0].Directory != "/tmp/project" || saved[0].ExitCode == nil || *saved[0].ExitCode != code || saved[0].Duration != 2*time.Second || saved[0].SavedAt.IsZero() {
		t.Fatalf("saved history metadata mismatch: %#v", saved)
	}
	encoded, err := json.Marshal(saved[0])
	if err != nil || strings.Contains(string(encoded), "private terminal output") || !strings.Contains(m.historyNotice, "saved") {
		t.Fatalf("save result or output exclusion mismatch: history=%s notice=%q err=%v", encoded, m.historyNotice, err)
	}
}

func TestIncompleteBlockCannotBeSaved(t *testing.T) {
	m := NewModelWithBackend(&fakeBackend{}, &fakeMonitor{})
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	s := capturedSession("%2", testCaptureID)
	m.sessions, m.allSessions = []monitor.Session{s}, []monitor.Session{s}
	m.blockRead = func(string) ([]shellblocks.Block, error) {
		return []shellblocks.Block{{ID: 1, Command: "still-running", Directory: "/tmp", StartedAt: time.Now(), Incomplete: true}}, nil
	}
	writes := 0
	m.historyWrite = func(history.Entry) (int, error) { writes++; return 0, nil }
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	m, _ = updateModel(t, m, cmd())
	m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd != nil || writes != 0 || m.blocks.err == nil {
		t.Fatal("incomplete command was saved")
	}
}

type capturedLifecycleFake struct{ shellBackend }

func (b *capturedLifecycleFake) CreateCapturedShellSession(directory string) (tmux.Pane, error) {
	b.directory = directory
	return tmux.Pane{PaneID: "%8", Shell: true, CaptureID: testCaptureID}, nil
}

func TestCaptureMustBeExplicitAndRequiresSupportedBackend(t *testing.T) {
	b := &capturedLifecycleFake{}
	m := NewModelWithBackend(b, &fakeMonitor{})
	m.width, m.height = 80, 24
	m = key(m, t, "c")
	for i := 0; i < len(launchProviders); i++ {
		m = key(m, t, "tab")
	}
	if !m.lifecycle.shell || m.lifecycle.capture || !strings.Contains(m.lifecycleView(), "contain secrets") {
		t.Fatal("captured shell should be opt-in with disclosure")
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if !m.lifecycle.capture {
		t.Fatal("capture toggle did not enable capture")
	}
	m.lifecycle.text = t.TempDir()
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("captured launch not scheduled")
	}
	m, _ = updateModel(t, m, cmd())
	if m.sessions[m.cursor].Pane.CaptureID != testCaptureID {
		t.Fatal("capture ID was not retained on launched pane")
	}

	unsupported := NewModelWithBackend(&shellBackend{}, &fakeMonitor{})
	unsupported = key(unsupported, t, "c")
	for i := 0; i < len(launchProviders); i++ {
		unsupported = key(unsupported, t, "tab")
	}
	unsupported, _ = updateModel(t, unsupported, tea.KeyMsg{Type: tea.KeyCtrlB})
	unsupported.lifecycle.text = t.TempDir()
	unsupported, cmd = updateModel(t, unsupported, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || unsupported.lifecycle.err == nil {
		t.Fatal("unsupported capture request was accepted")
	}
}

func intPtr(value int) *int { return &value }
