package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// outputSearch is scoped to one selected pane and counts matching capture lines.
type outputSearch struct {
	editing  bool
	query    string
	saved    string
	position int
	paneID   string
	matches  []int
	current  int
}

func (s outputSearch) active() bool { return s.editing || s.query != "" }

func (m *Model) searchKey(msg tea.KeyMsg) bool {
	if m.search.editing {
		runes := []rune(m.search.query)
		switch msg.String() {
		case "enter":
			m.search.editing = false
		case "esc":
			m.search.query = m.search.saved
			m.search.editing = false
		case "ctrl+c":
			m.quitting = true
		case "left":
			if m.search.position > 0 {
				m.search.position--
			}
		case "right":
			if m.search.position < len(runes) {
				m.search.position++
			}
		case "home", "ctrl+a":
			m.search.position = 0
		case "end", "ctrl+e":
			m.search.position = len(runes)
		case "backspace", "ctrl+h":
			if m.search.position > 0 {
				runes = append(runes[:m.search.position-1], runes[m.search.position:]...)
				m.search.position--
				m.search.query = string(runes)
			}
		case "delete":
			if m.search.position < len(runes) {
				m.search.query = string(append(runes[:m.search.position], runes[m.search.position+1:]...))
			}
		case "ctrl+u":
			m.search.query = ""
			m.search.position = 0
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
				updated := append([]rune{}, runes[:m.search.position]...)
				updated = append(updated, clean...)
				updated = append(updated, runes[m.search.position:]...)
				m.search.query = string(updated)
				m.search.position += len(clean)
			}
		}
		m.search.current = 0
		m.updateViewport()
		return true
	}
	switch msg.String() {
	case "/":
		if m.cursor >= len(m.sessions) {
			return true
		}
		m.search.editing = true
		m.search.saved = m.search.query
		m.search.position = len([]rune(m.search.query))
		m.search.paneID = m.sessions[m.cursor].Pane.PaneID
		m.updateViewport()
		return true
	case "esc":
		if m.search.active() {
			m.search = outputSearch{}
			m.updateViewport()
			return true
		}
	case "ctrl+n", "ctrl+p":
		if m.search.active() {
			if len(m.search.matches) > 0 {
				step := 1
				if msg.String() == "ctrl+p" {
					step = -1
				}
				m.search.current = (m.search.current + step + len(m.search.matches)) % len(m.search.matches)
				m.updateViewport()
			}
			return true
		}
	}
	return false
}

func (m *Model) updateSearchViewport() {
	if m.cursor >= len(m.sessions) || m.sessions[m.cursor].Pane.PaneID != m.search.paneID {
		m.search = outputSearch{}
		return
	}
	lines := strings.Split(ansi.Strip(m.sessions[m.cursor].Content), "\n")
	m.search.matches = nil
	for i, line := range lines {
		if m.search.query != "" && strings.Contains(strings.ToLower(line), strings.ToLower(m.search.query)) {
			m.search.matches = append(m.search.matches, i)
		}
	}
	if m.search.current >= len(m.search.matches) {
		m.search.current = 0
	}
	for i := range lines {
		prefix := "  "
		if len(m.search.matches) > 0 && i == m.search.matches[m.search.current] {
			prefix = "> "
		}
		lines[i] = prefix + lines[i]
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
	if len(m.search.matches) > 0 {
		m.viewport.SetYOffset(m.search.matches[m.search.current])
	} else {
		m.viewport.GotoTop()
	}
}

func (m Model) searchHelp() string {
	current := 0
	if len(m.search.matches) > 0 {
		current = m.search.current + 1
	}
	query := m.search.query
	if m.search.editing {
		r := []rune(query)
		query = string(r[:m.search.position]) + "▏" + string(r[m.search.position:])
		return styleHelp.Render(fmt.Sprintf("Search: %s  (%d/%d matching lines)  [enter] Keep  [esc] Cancel", query, current, len(m.search.matches)))
	}
	return styleHelp.Render(fmt.Sprintf("Search: %s  (%d/%d matching lines)  [ctrl+n/p] Next/previous  [/] Edit  [esc] Clear", query, current, len(m.search.matches)))
}
