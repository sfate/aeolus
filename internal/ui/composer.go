package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfate/aeolus/internal/monitor"
)

// textSender is optional so existing transports remain compatible.
type textSender interface{ SendText(string, string) error }
type promptSentMsg struct {
	paneID string
	err    error
}
type promptComposer struct {
	active, pending bool
	paneID, draft   string
	provider        monitor.Provider
	kind            monitor.SessionKind
	position        int
	err             error
	context         contextInput
	library         promptLibrary
}

func (m *Model) composerKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.composer.active {
		if msg.String() != "i" {
			return false, nil
		}
		if m.cursor >= len(m.sessions) {
			return true, nil
		}
		m.search = outputSearch{}
		m.composer = promptComposer{active: true, paneID: m.sessions[m.cursor].Pane.PaneID, provider: m.sessions[m.cursor].Provider, kind: m.sessions[m.cursor].Kind}
		m.updateViewport()
		return true, nil
	}
	if m.composer.pending {
		return true, nil
	}
	if m.composer.library.active {
		handled, cmd := m.libraryKey(msg)
		return handled, cmd
	}
	if handled, cmd := m.contextKey(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.libraryKey(msg); handled {
		return true, cmd
	}
	c := &m.composer
	runes := []rune(c.draft)
	switch msg.String() {
	case "esc":
		m.composer = promptComposer{}
		m.updateViewport()
		return true, nil
	case "ctrl+s":
		sender, ok := m.backend.(textSender)
		switch {
		case !ok:
			c.err = fmt.Errorf("this transport does not support sending text")
		case !m.composerTargetAvailable():
			c.err = fmt.Errorf("target pane %s is no longer available; draft retained", c.paneID)
		case strings.TrimSpace(c.draft) == "":
			c.err = fmt.Errorf("enter a prompt before sending")
		default:
			c.pending = true
			c.err = nil
			target, text := c.paneID, c.draft
			m.updateViewport()
			return true, func() tea.Msg { return promptSentMsg{paneID: target, err: sender.SendText(target, text)} }
		}
	case "left":
		if c.position > 0 {
			c.position--
		}
	case "right":
		if c.position < len(runes) {
			c.position++
		}
	case "home", "ctrl+a":
		c.position = lineStart(runes, c.position)
	case "end", "ctrl+e":
		c.position = lineEnd(runes, c.position)
	case "up":
		start := lineStart(runes, c.position)
		if start > 0 {
			col := c.position - start
			previous := lineStart(runes, start-1)
			c.position = min(previous+col, start-1)
		}
	case "down":
		end := lineEnd(runes, c.position)
		if end < len(runes) {
			col := c.position - lineStart(runes, c.position)
			c.position = min(end+1+col, lineEnd(runes, end+1))
		}
	case "backspace", "ctrl+h":
		if c.position > 0 {
			c.draft = string(append(runes[:c.position-1], runes[c.position:]...))
			c.position--
		}
	case "delete":
		if c.position < len(runes) {
			c.draft = string(append(runes[:c.position], runes[c.position+1:]...))
		}
	case "enter":
		c.insert([]rune{'\n'})
	case "tab":
		c.insert([]rune{'\t'})
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			input := []rune(strings.ReplaceAll(strings.ReplaceAll(string(msg.Runes), "\r\n", "\n"), "\r", "\n"))
			if msg.Type == tea.KeySpace {
				input = []rune{' '}
			}
			// Pasted newlines and tabs are text; other terminal controls are discarded.
			clean := make([]rune, 0, len(input))
			for _, r := range input {
				if !unicode.IsControl(r) || r == '\n' || r == '\t' {
					clean = append(clean, r)
				}
			}
			c.insert(clean)
		}
	}
	m.updateViewport()
	return true, nil
}
func (c *promptComposer) insert(input []rune) {
	runes := []rune(c.draft)
	next := append([]rune{}, runes[:c.position]...)
	next = append(next, input...)
	next = append(next, runes[c.position:]...)
	c.draft = string(next)
	c.position += len(input)
}
func lineStart(r []rune, p int) int {
	for p > 0 && r[p-1] != '\n' {
		p--
	}
	return p
}
func lineEnd(r []rune, p int) int {
	for p < len(r) && r[p] != '\n' {
		p++
	}
	return p
}
func (m *Model) updateComposerViewport() {
	if m.composer.library.active {
		m.viewport.SetContent(m.libraryView())
		m.viewport.GotoTop()
		return
	}
	if m.composer.context.active {
		m.viewport.SetContent(m.contextView())
		m.viewport.GotoTop()
		return
	}
	c := m.composer
	r := []rune(c.draft)
	header := fmt.Sprintf("Compose for pane %s\n", c.paneID)
	if c.kind == monitor.SessionShell {
		header += "Shell command: Ctrl+S sends text and Enter, which executes commands.\n"
	}
	if c.pending {
		header += "Sending...\n"
	} else if !m.composerTargetAvailable() {
		header += "Target unavailable. Draft retained; Esc cancels.\n"
	}
	if c.err != nil {
		header += "Error: " + c.err.Error() + "\n"
	}
	header += "\n"
	before := ansi.Hardwrap(header+string(r[:c.position])+"▏", max(1, m.viewport.Width-1), true)
	content := ansi.Hardwrap(header+string(r[:c.position])+"▏"+string(r[c.position:]), max(1, m.viewport.Width-1), true)
	m.viewport.SetContent(content)
	line := strings.Count(before, "\n")
	if line < m.viewport.YOffset {
		m.viewport.SetYOffset(line)
	} else if line >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(line - m.viewport.Height + 1)
	}
}
func (m *Model) promptSent(msg promptSentMsg) {
	if !m.composer.active || !m.composer.pending || msg.paneID != m.composer.paneID {
		return
	}
	m.composer.pending = false
	if msg.err != nil {
		m.composer.err = msg.err
	} else {
		m.composer = promptComposer{}
	}
	m.updateViewport()
}

func (m Model) composerTargetAvailable() bool {
	source := m.allSessions
	if source == nil {
		source = m.sessions
	}
	for _, s := range source {
		if s.Pane.PaneID == m.composer.paneID && s.Provider == m.composer.provider && s.Kind == m.composer.kind {
			return true
		}
	}
	return false
}
