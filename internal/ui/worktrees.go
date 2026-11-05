package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
	"github.com/sfate/aeolus/internal/worktrees"
)

type worktreeForm struct {
	active, busy              bool
	launchUncertain           bool
	paneID, source            string
	original                  monitor.Provider
	fields                    [2]string
	field, position, provider int
	result                    worktrees.Result
	err                       error
	create                    func(context.Context, string, string, string) (worktrees.Result, error)
}

type worktreeCreatedMsg struct {
	result worktrees.Result
	err    error
}
type worktreeLaunchedMsg struct {
	pane     tmux.Pane
	provider monitor.Provider
	err      error
}

func (m Model) worktreeTargetValid() bool {
	sessions := m.allSessions
	if sessions == nil {
		sessions = m.sessions
	}
	for _, s := range sessions {
		if s.Pane.PaneID == m.worktree.paneID {
			return s.Provider == m.worktree.original && s.Pane.CurrentPath == m.worktree.source
		}
	}
	return false
}

func (m *Model) worktreeKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	f := &m.worktree
	if !f.active {
		if msg.String() != "W" {
			return false, nil
		}
		if m.cursor >= len(m.sessions) {
			return true, nil
		}
		s := m.sessions[m.cursor]
		create := f.create
		*f = worktreeForm{active: true, paneID: s.Pane.PaneID, source: s.Pane.CurrentPath, original: s.Provider, create: create}
		m.search = outputSearch{}
		return true, nil
	}
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return true, tea.Quit
	}
	if f.busy {
		return true, nil
	}
	switch msg.String() {
	case "esc":
		m.worktree = worktreeForm{}
	case "enter":
		if f.launchUncertain {
			f.err = fmt.Errorf("launch outcome is uncertain; inspect tmux sessions and retained worktree before reopening")
			return true, nil
		}
		if !m.worktreeTargetValid() {
			f.err = fmt.Errorf("the source pane, provider, or directory changed; reopen the form")
			return true, nil
		}
		if f.result.Directory != "" {
			if !f.result.Ready {
				f.err = fmt.Errorf("partial worktree retained at %q; inspect it before starting another operation", f.result.Directory)
				return true, nil
			}
			return true, m.launchWorktree()
		}
		if _, ok := m.backend.(tmux.Lifecycle); !ok {
			f.err = fmt.Errorf("session launch is unsupported by this backend")
			return true, nil
		}
		if f.source == "" {
			f.err = fmt.Errorf("source pane directory is unavailable")
			return true, nil
		}
		create := f.create
		if create == nil {
			create = worktrees.Git{}.Create
		}
		source, target, branch := f.source, f.fields[0], f.fields[1]
		f.busy = true
		f.err = nil
		return true, func() tea.Msg {
			result, err := create(context.Background(), source, target, branch)
			return worktreeCreatedMsg{result, err}
		}
	case "tab", "shift+tab":
		if f.result.Directory != "" {
			break
		}
		step := 1
		if msg.String() == "shift+tab" {
			step = -1
		}
		f.field = (f.field + step + 3) % 3
		if f.field < 2 {
			f.position = len([]rune(f.fields[f.field]))
		}
	default:
		if f.result.Directory != "" {
			break
		}
		if f.field == 2 {
			if msg.String() == "left" || msg.String() == "right" {
				step := 1
				if msg.String() == "left" {
					step = -1
				}
				f.provider = (f.provider + step + len(launchProviders)) % len(launchProviders)
			}
			break
		}
		text := []rune(f.fields[f.field])
		switch msg.String() {
		case "left":
			if f.position > 0 {
				f.position--
			}
		case "right":
			if f.position < len(text) {
				f.position++
			}
		case "home", "ctrl+a":
			f.position = 0
		case "end", "ctrl+e":
			f.position = len(text)
		case "ctrl+u":
			text = nil
			f.position = 0
		case "backspace", "ctrl+h":
			if f.position > 0 {
				text = append(text[:f.position-1], text[f.position:]...)
				f.position--
			}
		case "delete":
			if f.position < len(text) {
				text = append(text[:f.position], text[f.position+1:]...)
			}
		default:
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
				input := msg.Runes
				if msg.Type == tea.KeySpace {
					input = []rune{' '}
				}
				clean := []rune{}
				for _, r := range input {
					if !unicode.IsControl(r) {
						clean = append(clean, r)
					}
				}
				next := append([]rune{}, text[:f.position]...)
				next = append(next, clean...)
				next = append(next, text[f.position:]...)
				text = next
				f.position += len(clean)
			}
		}
		f.fields[f.field] = string(text)
	}
	return true, nil
}

func (m *Model) launchWorktree() tea.Cmd {
	f := &m.worktree
	if !m.worktreeTargetValid() {
		f.err = fmt.Errorf("source changed; worktree retained at %q", f.result.Directory)
		return nil
	}
	backend, ok := m.backend.(tmux.Lifecycle)
	if !ok {
		f.err = fmt.Errorf("session launch is unsupported; worktree retained at %q", f.result.Directory)
		return nil
	}
	directory, provider := f.result.Directory, launchProviders[f.provider]
	f.busy = true
	f.err = nil
	return func() tea.Msg {
		pane, err := backend.CreateSession(directory, string(provider))
		return worktreeLaunchedMsg{pane, provider, err}
	}
}

func (m Model) finishWorktreeCreate(msg worktreeCreatedMsg) (tea.Model, tea.Cmd) {
	if !m.worktree.active || !m.worktree.busy {
		return m, nil
	}
	m.worktree.busy = false
	m.worktree.result = msg.result
	m.worktree.err = msg.err
	if msg.err != nil {
		return m, nil
	}
	if !msg.result.Ready {
		m.worktree.err = fmt.Errorf("worktree checkout is incomplete; inspect retained artifacts")
		return m, nil
	}
	cmd := m.launchWorktree()
	return m, cmd
}

func (m Model) finishWorktreeLaunch(msg worktreeLaunchedMsg) (tea.Model, tea.Cmd) {
	if !m.worktree.active || !m.worktree.busy {
		return m, nil
	}
	m.worktree.busy = false
	m.worktree.err = msg.err
	if msg.err != nil {
		var uncertain *tmux.LaunchUncertainError
		m.worktree.launchUncertain = errors.As(msg.err, &uncertain)
		return m, nil
	}
	m.worktree = worktreeForm{}
	m.lifecycle = lifecycleForm{kind: lifecycleLaunch, busy: true}
	return m.finishLifecycle(lifecycleMsg{pane: msg.pane, provider: msg.provider})
}

func (m Model) worktreeView() string {
	f := m.worktree
	fields := f.fields
	if f.field < 2 {
		text := []rune(fields[f.field])
		fields[f.field] = string(text[:f.position]) + "▏" + string(text[f.position:])
	}
	status := "[tab/shift+tab] Field  [left/right] Agent  [enter] Create and launch  [esc] Cancel"
	if f.busy {
		status = "Working... [ctrl+c] Quit"
	} else if f.launchUncertain {
		status = "Launch outcome uncertain. Inspect tmux sessions before retrying. [esc] Close"
	} else if f.result.Ready {
		status = "Worktree retained. [enter] Retry launch only  [esc] Close"
	} else if f.result.Directory != "" {
		status = "Partial worktree retained. Inspect it manually. [esc] Close"
	}
	if f.err != nil {
		status = safeReviewText(f.err.Error()) + "\n\n" + status
	}
	return fmt.Sprintf("Launch agent in a new Git worktree\n\nSource: %s\nDestination (absolute new path): %s\nNew branch: %s\nAgent: %s\n\n%s", strconv.Quote(f.source), fields[0], fields[1], launchProviders[f.provider], status)
}
