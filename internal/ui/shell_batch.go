package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
)

type shellBatchMode uint8

const (
	shellBatchSelect shellBatchMode = iota
	shellBatchEdit
	shellBatchReview
	shellBatchSending
	shellBatchDone
)

type shellBatchTarget struct {
	paneID, sessionID, sessionName, windowName, currentPath string
	selected                                                bool
}

type shellBatchResult struct {
	paneID, sessionName, windowName string
	err                             error
}

type shellBatchState struct {
	active  bool
	mode    shellBatchMode
	targets []shellBatchTarget
	cursor  int
	command string
	pos     int
	err     error
	results []shellBatchResult
}

type shellBatchMsg struct {
	err     error
	results []shellBatchResult
}

const maxShellBatchCommandBytes = 64 << 10

func newShellBatch(sessions []monitor.Session, selectedPane string) shellBatchState {
	b := shellBatchState{active: true, mode: shellBatchSelect}
	seen := map[string]bool{}
	for _, s := range sessions {
		if s.Kind != monitor.SessionShell || !s.Pane.Shell || s.Pane.PaneID == "" || seen[s.Pane.PaneID] {
			continue
		}
		seen[s.Pane.PaneID] = true
		target := shellBatchTarget{paneID: s.Pane.PaneID, sessionID: s.Pane.SessionID, sessionName: s.Pane.SessionName, windowName: s.Pane.WindowName, currentPath: s.Pane.CurrentPath}
		target.selected = s.Pane.PaneID == selectedPane
		b.targets = append(b.targets, target)
	}
	return b
}

func (m *Model) openShellBatch() {
	selected := ""
	if m.cursor >= 0 && m.cursor < len(m.sessions) {
		selected = m.sessions[m.cursor].Pane.PaneID
	}
	m.shellBatch = newShellBatch(m.sessions, selected)
	m.updateViewport()
}

func (m *Model) shellBatchKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	b := &m.shellBatch
	if !b.active {
		return false, nil
	}
	if b.mode == shellBatchSending {
		return true, nil
	}
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return true, tea.Quit
	}
	if b.mode == shellBatchDone {
		if msg.String() == "esc" || msg.String() == "enter" {
			m.shellBatch = shellBatchState{}
			m.updateViewport()
		}
		return true, nil
	}
	switch msg.String() {
	case "esc":
		switch b.mode {
		case shellBatchReview:
			b.mode = shellBatchEdit
		case shellBatchEdit:
			b.mode = shellBatchSelect
		default:
			m.shellBatch = shellBatchState{}
		}
		m.updateViewport()
		return true, nil
	case "n":
		if b.mode == shellBatchReview {
			m.shellBatch = shellBatchState{}
			m.updateViewport()
			return true, nil
		}
	}
	switch b.mode {
	case shellBatchSelect:
		switch msg.String() {
		case "up", "k":
			b.cursor = max(0, b.cursor-1)
		case "down", "j":
			b.cursor = min(len(b.targets)-1, b.cursor+1)
		case " ", "space":
			if len(b.targets) > 0 {
				b.targets[b.cursor].selected = !b.targets[b.cursor].selected
			}
		case "enter":
			if selectedShellTargetCount(b.targets) == 0 {
				b.err = fmt.Errorf("select at least one managed shell")
			} else {
				b.mode, b.err = shellBatchEdit, nil
			}
		}
	case shellBatchEdit:
		runes := []rune(b.command)
		switch msg.String() {
		case "ctrl+s":
			if strings.TrimSpace(b.command) == "" {
				b.err = fmt.Errorf("enter a command before review")
			} else {
				b.mode, b.err = shellBatchReview, nil
			}
		case "left":
			b.pos = max(0, b.pos-1)
		case "right":
			b.pos = min(len(runes), b.pos+1)
		case "home", "ctrl+a":
			b.pos = lineStart(runes, b.pos)
		case "end", "ctrl+e":
			b.pos = lineEnd(runes, b.pos)
		case "backspace", "ctrl+h":
			if b.pos > 0 {
				b.command = string(append(runes[:b.pos-1], runes[b.pos:]...))
				b.pos--
			}
		case "delete":
			if b.pos < len(runes) {
				b.command = string(append(runes[:b.pos], runes[b.pos+1:]...))
			}
		case "ctrl+u":
			b.command, b.pos = "", 0
		case "enter":
			if len(b.command)+1 <= maxShellBatchCommandBytes {
				b.command = string(append(append(append([]rune{}, runes[:b.pos]...), '\n'), runes[b.pos:]...))
				b.pos++
			}
		case "tab":
			if len(b.command)+1 <= maxShellBatchCommandBytes {
				b.command = string(append(append(append([]rune{}, runes[:b.pos]...), '\t'), runes[b.pos:]...))
				b.pos++
			}
		default:
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
				input := msg.Runes
				if msg.Type == tea.KeySpace {
					input = []rune{' '}
				}
				var cleaned []rune
				for _, r := range input {
					if !unicode.IsControl(r) || r == '\n' || r == '\t' {
						cleaned = append(cleaned, r)
					}
				}
				inserted := string(cleaned)
				if len(b.command)+len(inserted) <= maxShellBatchCommandBytes {
					b.command = string(append(append(append([]rune{}, runes[:b.pos]...), cleaned...), runes[b.pos:]...))
					b.pos += len(cleaned)
				}
			}
		}
	case shellBatchReview:
		if msg.String() == "y" {
			sender, ok := m.backend.(textSender)
			if !ok {
				b.mode, b.err = shellBatchDone, fmt.Errorf("this transport does not support sending text")
				break
			}
			b.mode, b.err = shellBatchSending, nil
			command := b.command
			targets := selectedShellBatchTargets(b.targets)
			backend := m.backend
			return true, func() tea.Msg {
				live, err := backend.ListAllPanes()
				if err != nil {
					return shellBatchMsg{err: fmt.Errorf("cannot verify shell targets: %w", err)}
				}
				byID := make(map[string]tmux.Pane, len(live))
				for _, pane := range live {
					byID[pane.PaneID] = pane
				}
				for _, target := range targets {
					pane, exists := byID[target.paneID]
					if !exists || !pane.Shell || pane.SessionID != target.sessionID || pane.SessionName != target.sessionName || pane.WindowName != target.windowName || pane.CurrentPath != target.currentPath {
						return shellBatchMsg{err: fmt.Errorf("shell target %s changed or is no longer available; nothing was sent", target.paneID)}
					}
				}
				results := make([]shellBatchResult, 0, len(targets))
				for _, target := range targets {
					results = append(results, shellBatchResult{paneID: target.paneID, sessionName: target.sessionName, windowName: target.windowName, err: sender.SendText(target.paneID, command)})
				}
				return shellBatchMsg{results: results}
			}
		}
	}
	m.updateViewport()
	return true, nil
}

func selectedShellTargetCount(targets []shellBatchTarget) int {
	count := 0
	for _, target := range targets {
		if target.selected {
			count++
		}
	}
	return count
}

func selectedShellBatchTargets(targets []shellBatchTarget) []shellBatchTarget {
	selected := make([]shellBatchTarget, 0, len(targets))
	for _, target := range targets {
		if target.selected {
			selected = append(selected, target)
		}
	}
	return selected
}

func (m *Model) finishShellBatch(msg shellBatchMsg) {
	m.shellBatch.mode = shellBatchDone
	m.shellBatch.err = msg.err
	m.shellBatch.results = msg.results
	m.updateViewport()
}

func (m Model) shellBatchView() string {
	b := m.shellBatch
	var out strings.Builder
	switch b.mode {
	case shellBatchSelect:
		out.WriteString(m.theme.screenTitle("Send command to managed shells") + "\n" + m.theme.metadata("Select targets with Space. The selected pane starts checked.") + "\n\n")
		for i, target := range b.targets {
			mark := "[ ]"
			if target.selected {
				mark = "[x]"
			}
			cursor := "  "
			if i == b.cursor {
				cursor = "> "
			}
			label := fmt.Sprintf("%s/%s  %s", safeReviewText(target.sessionName), safeReviewText(target.windowName), safeReviewText(target.currentPath))
			if i == b.cursor {
				out.WriteString(m.theme.selectedRow(mark+" "+label) + "\n")
			} else {
				fmt.Fprintf(&out, "%s%s %s\n", cursor, mark, label)
			}
		}
		out.WriteString("\n" + m.theme.primaryHint("[Space] Toggle  [Enter] Compose  [Esc] Cancel"))
	case shellBatchEdit:
		fmt.Fprintf(&out, "%s\n%s\n\n%s▏\n\n%s", m.theme.screenTitle(fmt.Sprintf("Command for %d managed shell(s)", selectedShellTargetCount(b.targets))), m.theme.metadata("Edit the literal command below. It executes when submitted."), safeReviewText(b.command), m.theme.primaryHint("[ctrl+s] Review targets and command  [Esc] Back"))
	case shellBatchReview:
		out.WriteString(m.theme.screenTitle("Review command and targets") + "\n" + m.theme.metadata("The command will be submitted to each selected shell.") + "\n\n" + m.theme.sectionLabel("Targets") + "\n")
		for _, target := range b.targets {
			if target.selected {
				fmt.Fprintf(&out, "- pane %s, %s/%s (%s)\n", target.paneID, safeReviewText(target.sessionName), safeReviewText(target.windowName), safeReviewText(target.currentPath))
			}
		}
		fmt.Fprintf(&out, "\n%s\n%s\n\n%s", m.theme.sectionLabel("Command"), safeReviewText(b.command), m.theme.primaryHint("[y] Send once to every selected pane  [n] Cancel  [Esc] Edit"))
	case shellBatchSending:
		out.WriteString(m.theme.screenTitle("Sending command") + "\n\n" + m.theme.metadata("Verifying shell targets. No automatic retries are made."))
	case shellBatchDone:
		if b.err != nil {
			fmt.Fprintf(&out, "%s\n", m.theme.errorText("No command sent: "+safeReviewText(b.err.Error())))
		}
		for _, result := range b.results {
			status := "sent"
			if result.err != nil {
				status = m.theme.errorText("failed: " + safeReviewText(result.err.Error()))
			} else {
				status = m.theme.primaryHint(status)
			}
			target := m.theme.metadata(fmt.Sprintf("%s/%s (%s): ", safeReviewText(result.sessionName), safeReviewText(result.windowName), result.paneID))
			fmt.Fprintf(&out, "%s%s\n", target, status)
		}
		out.WriteString("\n" + m.theme.primaryHint("[Enter/Esc] Close"))
	}
	if b.err != nil && b.mode != shellBatchDone {
		fmt.Fprintf(&out, "\n\n%s", m.theme.errorText(safeReviewText(b.err.Error())))
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	return ansi.Hardwrap(out.String(), max(1, width-4), true)
}
