package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
)

type lifecycleKind int

const (
	lifecycleNone lifecycleKind = iota
	lifecycleLaunch
	lifecycleRename
)

type lifecycleForm struct {
	kind     lifecycleKind
	text     string
	position int
	provider int
	preset   int
	paneID   string
	busy     bool
	err      error
}

type lifecycleMsg struct {
	pane     tmux.Pane
	provider monitor.Provider
	err      error
}

var launchProviders = []monitor.Provider{monitor.ProviderClaude, monitor.ProviderCodex, monitor.ProviderGemini}

func (m *Model) lifecycleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.lifecycle.kind == lifecycleNone {
		if m.search.editing {
			return false, nil
		}
		switch msg.String() {
		case "c":
			directory, err := os.Getwd()
			m.lifecycle = lifecycleForm{kind: lifecycleLaunch, text: directory, position: len([]rune(directory)), provider: launchProviderIndex(m.defaultProvider), preset: -1, err: err}
		case "R":
			if m.cursor >= len(m.sessions) {
				return true, nil
			}
			pane := m.sessions[m.cursor].Pane
			m.lifecycle = lifecycleForm{kind: lifecycleRename, paneID: pane.PaneID, text: pane.WindowName, position: len([]rune(pane.WindowName))}
		default:
			return false, nil
		}
		m.search = outputSearch{}
		return true, nil
	}
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return true, tea.Quit
	}
	if m.lifecycle.busy {
		return true, nil
	}
	form := &m.lifecycle
	runes := []rune(form.text)
	switch msg.String() {
	case "esc":
		m.lifecycle = lifecycleForm{}
	case "ctrl+p":
		if form.kind == lifecycleLaunch {
			m.cycleProjectPreset()
		}
	case "tab", "shift+tab":
		if form.kind == lifecycleLaunch {
			step := 1
			if msg.String() == "shift+tab" {
				step = -1
			}
			form.provider = (form.provider + step + len(launchProviders)) % len(launchProviders)
			form.preset = -1
		}
	case "enter":
		transport, ok := m.backend.(tmux.Lifecycle)
		if !ok {
			form.err = fmt.Errorf("session lifecycle is not supported by this backend")
			return true, nil
		}
		if form.kind == lifecycleRename && !hasPane(m.sessions, form.paneID) {
			form.err = fmt.Errorf("the target pane is no longer available")
			return true, nil
		}
		text := form.text
		if strings.TrimSpace(text) == "" {
			field := "window name"
			if form.kind == lifecycleLaunch {
				field = "directory"
			}
			form.err = fmt.Errorf("enter a %s", field)
			return true, nil
		}
		if form.kind == lifecycleLaunch {
			if text == "~" || strings.HasPrefix(text, "~/") {
				home, err := os.UserHomeDir()
				if err != nil {
					form.err = err
					return true, nil
				}
				text = filepath.Join(home, strings.TrimPrefix(text, "~/"))
				if form.text == "~" {
					text = home
				}
			}
		}
		kind, paneID, provider := form.kind, form.paneID, launchProviders[form.provider]
		form.busy = true
		form.err = nil
		return true, func() tea.Msg {
			if kind == lifecycleLaunch {
				pane, err := transport.CreateSession(text, string(provider))
				return lifecycleMsg{pane: pane, provider: provider, err: err}
			}
			err := transport.RenameWindow(paneID, text)
			return lifecycleMsg{err: err}
		}
	case "left":
		if form.position > 0 {
			form.position--
		}
	case "right":
		if form.position < len(runes) {
			form.position++
		}
	case "home", "ctrl+a":
		form.position = 0
	case "end", "ctrl+e":
		form.position = len(runes)
	case "ctrl+u":
		form.text = ""
		form.position = 0
	case "backspace", "ctrl+h":
		if form.position > 0 {
			form.text = string(append(runes[:form.position-1], runes[form.position:]...))
			form.position--
		}
	case "delete":
		if form.position < len(runes) {
			form.text = string(append(runes[:form.position], runes[form.position+1:]...))
		}
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			input := msg.Runes
			if msg.Type == tea.KeySpace {
				input = []rune{' '}
			}
			clean := make([]rune, 0, len(input))
			for _, r := range input {
				if !unicode.IsControl(r) {
					clean = append(clean, r)
				}
			}
			updated := append([]rune{}, runes[:form.position]...)
			updated = append(updated, clean...)
			updated = append(updated, runes[form.position:]...)
			form.text = string(updated)
			form.position += len(clean)
		}
	}
	if form.kind == lifecycleLaunch && form.text != string(runes) && msg.String() != "ctrl+p" {
		form.preset = -1
	}
	return true, nil
}

func (m Model) finishLifecycle(msg lifecycleMsg) (tea.Model, tea.Cmd) {
	m.lifecycle.busy = false
	if msg.err != nil {
		m.lifecycle.err = msg.err
		return m, nil
	}
	if m.lifecycle.kind == lifecycleLaunch {
		m.syncFilterSource()
		m.allSessions = append(m.allSessions, monitor.Session{Pane: msg.pane, Provider: msg.provider, Status: monitor.StatusUnknown})
		// A newly launched session should be visible and selected.
		m.filters.applied = filterChoice{}
		m.rebuildFilteredSessions(msg.pane.PaneID)
	} else {
		m.syncFilterSource()
		for i := range m.allSessions {
			if m.allSessions[i].Pane.PaneID == m.lifecycle.paneID {
				m.allSessions[i].Pane.WindowName = m.lifecycle.text
			}
		}
		m.rebuildFilteredSessions(m.lifecycle.paneID)
	}
	m.lifecycle = lifecycleForm{}
	m.err = nil
	m.tmuxMissing = false
	m.updateViewport()
	return m, m.doRefresh()
}

func (m Model) lifecycleView() string {
	form := m.lifecycle
	runes := []rune(form.text)
	input := string(runes[:form.position]) + "▏" + string(runes[form.position:])
	title := "Rename window for pane " + form.paneID
	details := "Window name: " + input + "\n\nRenames the entire tmux window, including other panes."
	if form.kind == lifecycleLaunch {
		title = "Launch agent"
		details = fmt.Sprintf("Agent: %s  [tab/shift+tab] Change\nDirectory: %s\n\nCreates a dedicated detached tmux session.", launchProviders[form.provider], input)
		details += "\nPreset: " + m.projectPresetLabel() + "  [ctrl+p] Next configured project"
	}
	status := "[enter] Submit  [esc] Cancel"
	if form.busy {
		status = "Working...  [ctrl+c] Quit"
	}
	if form.err != nil {
		status = form.err.Error() + "\n\n" + status
	}
	return title + "\n\n" + details + "\n\n" + status
}
