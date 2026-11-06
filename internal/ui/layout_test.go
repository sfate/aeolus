package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfate/aeolus/internal/monitor"
)

func assertFits(t *testing.T, text string, width, height int) {
	t.Helper()
	if !utf8.ValidString(text) {
		t.Fatal("invalid UTF-8")
	}
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		t.Fatalf("height %d exceeds %d", len(lines), height)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > width {
			t.Fatalf("line width %d exceeds %d: %q", ansi.StringWidth(line), width, line)
		}
	}
}

func TestLayoutBoundsAcrossModes(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {12, 5}, {24, 12}, {40, 20}, {79, 24}, {80, 24}, {120, 40}} {
		for _, mode := range []string{"dashboard", "permission", "question", "empty", "error", "missing", "launch", "rename", "composer", "attach", "filters", "review", "search", "local-search"} {
			t.Run(fmt.Sprintf("%s/%dx%d", mode, size[0], size[1]), func(t *testing.T) {
				m := NewModel()
				s := testSession("%1")
				s.Pane.SessionName = strings.Repeat("界é🙂", 30)
				s.Pane.WindowName = s.Pane.SessionName
				s.Pane.CurrentPath = "/" + s.Pane.SessionName
				s.Content = strings.Repeat("\x1b[31m界é🙂 long output\x1b[0m\n", 60)
				m.sessions = []monitor.Session{s}
				m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				switch mode {
				case "permission":
					m.sessions[0].Status = monitor.StatusPermission
					m.sessions[0].Request = &monitor.PermissionRequest{Command: strings.Repeat("long", 100)}
				case "question":
					m.sessions[0].Status = monitor.StatusQuestion
					m.sessions[0].Question = &monitor.QuestionRequest{Text: strings.Repeat("long", 100)}
				case "empty":
					m.sessions = nil
				case "error":
					m.err = errors.New(strings.Repeat("error", 100))
				case "missing":
					m.tmuxMissing = true
				case "launch":
					m.lifecycle = lifecycleForm{kind: lifecycleLaunch, text: strings.Repeat("界", 80), position: 80}
				case "rename":
					m.lifecycle = lifecycleForm{kind: lifecycleRename, text: strings.Repeat("界", 80), position: 80}
				case "composer":
					m.composer = promptComposer{active: true, paneID: "%1", provider: s.Provider, draft: strings.Repeat("界", 80), position: 80}
				case "attach":
					m.composer.active = true
					m.composer.context.active = true
					m.composer.context.path = strings.Repeat("界", 80)
					m.composer.context.position = 80
				case "filters":
					m.filters.editing = true
					m.filters.draft.directory = strings.Repeat("界", 80)
				case "review":
					m.review.active = true
					m.review.directory = strings.Repeat("界", 80)
					m.review.viewport.SetContent(strings.Repeat("diff\n", 100))
				case "search":
					m.search = outputSearch{editing: true, paneID: "%1", query: strings.Repeat("界", 80), position: 80}
				case "local-search":
					m.localSearch = localSearchState{active: true, query: strings.Repeat("界", 80), results: []localSearchResult{{kind: localPrompt, name: "Unicode", preview: strings.Repeat("界", 80)}}}
				}
				m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				assertFits(t, m.View(), size[0], size[1])
			})
		}
	}
}

func TestUnicodeSessionLabelsAndRows(t *testing.T) {
	for _, name := range []string{strings.Repeat("界", 20), strings.Repeat("é", 20), strings.Repeat("é", 20), strings.Repeat("👩‍💻", 20)} {
		s := testSession("%1")
		s.Pane.SessionName = name
		label := sessionLabel(s)
		if !utf8.ValidString(label) || ansi.StringWidth(label) > 20 {
			t.Fatalf("broken label %q", label)
		}
		m := NewModel()
		m.sessions = []monitor.Session{s}
		for _, width := range []int{1, 8, 24, 40} {
			assertFits(t, m.renderSessionList(width, 1), width, 1)
		}
	}
}

func TestNarrowLayoutAndResizePreserveSelection(t *testing.T) {
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1"), testSession("%2")}
	m.cursor = 1
	m.sessions[1].Content = strings.Repeat("output\n", 80)
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 40, Height: 24})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "session-2") || !strings.Contains(view, "output") {
		t.Fatal("narrow screen lost session list or output")
	}
	m, _ = updateModel(t, m, tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if m.viewport.YOffset >= m.viewport.TotalLineCount()-m.viewport.Height {
		t.Fatal("mouse scrolling did not reach output")
	}
	for _, width := range []int{120, 40, 24, 80} {
		m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: width, Height: 24})
		if m.selectedPaneID() != "%2" {
			t.Fatal("resize lost selection")
		}
		assertFits(t, m.View(), width, 24)
	}
}

func TestDashboardHierarchyAndResponsiveShortcuts(t *testing.T) {
	for _, size := range []struct {
		width int
		want  []string
	}{{40, []string{"AEOLUS", "Move", "Help"}}, {80, []string{"AEOLUS", "Navigate", "Search", "Help"}}, {120, []string{"AEOLUS", "TERMINAL SESSION DASHBOARD", "Launch", "Palette"}}} {
		m := NewModel()
		m.sessions = []monitor.Session{testSession("%1")}
		m.keybindings = map[string]string{"next": "n"}
		m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: size.width, Height: 24})
		view := ansi.Strip(m.View())
		for _, want := range size.want {
			if !strings.Contains(view, want) {
				t.Errorf("%dx24 dashboard missing %q: %s", size.width, want, view)
			}
		}
		if !strings.Contains(view, "↓/n") {
			t.Errorf("%dx24 dashboard omitted custom navigation binding: %s", size.width, view)
		}
		assertFits(t, m.View(), size.width, 24)
	}
}

func TestEditingCursorsStayVisibleOnNarrowScreens(t *testing.T) {
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1")}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 24, Height: 12})
	m.composer = promptComposer{active: true, paneID: "%1", provider: monitor.ProviderClaude, draft: strings.Repeat("界é", 80), position: 240}
	m.updateViewport()
	if !strings.Contains(m.View(), "▏") || !strings.Contains(m.View(), "ctrl+s") {
		t.Fatal("composer cursor/send hint hidden")
	}
	m.composer = promptComposer{}
	m.lifecycle = lifecycleForm{kind: lifecycleLaunch, text: strings.Repeat("界", 80), position: 80, err: errors.New(strings.Repeat("error\n", 20))}
	view := m.View()
	if !strings.Contains(view, "▏") || !strings.Contains(view, "[esc] Cancel") {
		t.Fatalf("launch cursor or cancellation hidden: %q", view)
	}
	m.lifecycle = lifecycleForm{}
	m.search = outputSearch{editing: true, paneID: "%1", query: strings.Repeat("界", 80), position: 80}
	m.updateViewport()
	if !strings.Contains(m.View(), "▏") {
		t.Fatal("search cursor hidden")
	}
}

func TestGitReviewMouseAndResize(t *testing.T) {
	m := NewModel()
	m.review.active = true
	m.review.viewport.SetContent(strings.Repeat("diff\n", 100))
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 40, Height: 24})
	m, _ = updateModel(t, m, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.review.viewport.YOffset == 0 {
		t.Fatal("mouse did not scroll review viewport")
	}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 24, Height: 12})
	assertFits(t, m.View(), 24, 12)
	if m.review.viewport.Width != 24 || m.review.viewport.Height != 5 {
		t.Fatal("review viewport ignored resize")
	}
}

func TestClippingPreservesANSIAndGraphemes(t *testing.T) {
	text := "\x1b[31m界é👩‍💻and-more\x1b[0m"
	got := fitLine(text, 6)
	if ansi.Strip(got) != "界é👩‍💻…" || !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("broken ANSI or grapheme: %q", got)
	}
}
