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

type commandPalette struct {
	active          bool
	query, selected string
	position        int
	paneID          string
	provider        monitor.Provider
}

type paletteAction struct{ label, key string }

func (m Model) paletteActions() []paletteAction {
	actions := []paletteAction{{"Keyboard help", "?"}, {"Filter sessions", "f"}, {"Refresh sessions", "r"}, {"Quit Aeolus", "q"}}
	_, lifecycle := m.backend.(tmux.Lifecycle)
	if lifecycle {
		actions = append(actions, paletteAction{"Launch agent", "c"})
	}
	if m.cursor >= 0 && m.cursor < len(m.sessions) {
		s := m.sessions[m.cursor]
		if s.Pane.PaneID == m.palette.paneID && s.Provider == m.palette.provider {
			actions = append(actions, paletteAction{"Search local library", "local-search"})
			if s.Kind == monitor.SessionShell {
				actions = append(actions, paletteAction{"Browse local notebooks", "notebooks"})
				actions = append(actions, paletteAction{"Browse tmux layouts", "layouts"})
			}
			actions = append(actions, paletteAction{"Switch to selected pane", "enter"}, paletteAction{"Search captured output", "/"}, paletteAction{"Kill pane (confirm first)", "x"})
			if m.cursor > 0 {
				actions = append(actions, paletteAction{"Previous session", "k"})
			}
			if m.cursor < len(m.sessions)-1 {
				actions = append(actions, paletteAction{"Next session", "j"})
			}
			if m.search.active() && m.search.paneID == s.Pane.PaneID {
				actions = append(actions, paletteAction{"Clear output search", "esc"})
			}
			if lifecycle {
				actions = append(actions, paletteAction{"Rename selected window", "R"})
			}
			if _, ok := m.backend.(textSender); ok {
				actions = append(actions, paletteAction{"Compose prompt", "i"})
			}
			if s.Pane.CurrentPath != "" {
				actions = append(actions, paletteAction{"Git review", "g"})
			}
			if s.CanRespond() {
				switch s.Status {
				case monitor.StatusPermission:
					actions = append(actions, paletteAction{"Approve request", "y"}, paletteAction{"Approve for session", "a"}, paletteAction{"Deny request", "n"})
				case monitor.StatusQuestion:
					actions = append(actions, paletteAction{"Answer yes", "y"}, paletteAction{"Answer no", "n"})
				}
			}
		}
	}
	query := strings.ToLower(m.palette.query)
	filtered := make([]paletteAction, 0, len(actions))
	for _, action := range actions {
		if strings.Contains(strings.ToLower(action.label), query) {
			filtered = append(filtered, action)
		}
	}
	return filtered
}

func (m *Model) selectPaletteFirst() {
	m.palette.selected = ""
	if actions := m.paletteActions(); len(actions) > 0 {
		m.palette.selected = actions[0].key
	}
}

func (m Model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.palette.active {
		m.palette = commandPalette{active: true}
		if m.cursor >= 0 && m.cursor < len(m.sessions) {
			s := m.sessions[m.cursor]
			m.palette.paneID, m.palette.provider = s.Pane.PaneID, s.Provider
		}
		m.selectPaletteFirst()
		return m, nil
	}
	p := &m.palette
	runes := []rune(p.query)
	switch msg.String() {
	case "esc":
		m.palette = commandPalette{}
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "enter":
		for _, action := range m.paletteActions() {
			if action.key == p.selected {
				m.palette = commandPalette{}
				key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(action.key)}
				if action.key == "enter" {
					key = tea.KeyMsg{Type: tea.KeyEnter}
				}
				if action.key == "esc" {
					key = tea.KeyMsg{Type: tea.KeyEsc}
				}
				if action.key == "local-search" {
					cmd := m.openLocalSearch()
					return m, cmd
				}
				if action.key == "notebooks" {
					cmd := m.openNotebookBrowser()
					return m, cmd
				}
				if action.key == "layouts" {
					return m, m.openLayoutBrowser()
				}
				return m.Update(key)
			}
		}
	case "up", "down", "ctrl+p", "ctrl+n":
		actions := m.paletteActions()
		if len(actions) == 0 {
			return m, nil
		}
		index := -1
		for i, action := range actions {
			if action.key == p.selected {
				index = i
				break
			}
		}
		step := 1
		if msg.String() == "up" || msg.String() == "ctrl+p" {
			step = -1
		}
		if index < 0 {
			index = 0
		} else {
			index = (index + step + len(actions)) % len(actions)
		}
		p.selected = actions[index].key
	case "left":
		p.position = max(0, p.position-1)
	case "right":
		p.position = min(len(runes), p.position+1)
	case "home", "ctrl+a":
		p.position = 0
	case "end", "ctrl+e":
		p.position = len(runes)
	case "ctrl+u":
		p.query, p.position = "", 0
		m.selectPaletteFirst()
	case "backspace", "ctrl+h":
		if p.position > 0 {
			p.query = string(append(runes[:p.position-1], runes[p.position:]...))
			p.position--
			m.selectPaletteFirst()
		}
	case "delete":
		if p.position < len(runes) {
			p.query = string(append(runes[:p.position], runes[p.position+1:]...))
			m.selectPaletteFirst()
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
			text := append([]rune{}, runes[:p.position]...)
			text = append(text, clean...)
			text = append(text, runes[p.position:]...)
			p.query = string(text)
			p.position += len(clean)
			m.selectPaletteFirst()
		}
	}
	return m, nil
}

func (m Model) paletteView() string {
	p := m.palette
	runes := []rune(p.query)
	query := string(runes[:p.position]) + "▏" + string(runes[p.position:])
	actions := m.paletteActions()
	var text strings.Builder
	fmt.Fprintf(&text, "Command palette\nFilter: %s\n\n", query)
	selected := -1
	for i, action := range actions {
		if action.key == p.selected {
			selected = i
			break
		}
	}
	rows := max(1, m.height-7)
	start := max(0, selected-rows+1)
	for i := start; i < len(actions) && i < start+rows; i++ {
		marker := "  "
		if i == selected {
			marker = "> "
		}
		fmt.Fprintf(&text, "%s%s [%s]\n", marker, actions[i].label, actions[i].key)
	}
	if len(actions) == 0 {
		text.WriteString("No matching actions.\n")
	}
	if len(actions) > 0 && selected < 0 {
		text.WriteString("Selection unavailable; use arrows to select.\n")
	}
	text.WriteString("\n[↑↓/ctrl+p/ctrl+n] Select  [enter] Execute  [esc] Cancel")
	return ansi.Hardwrap(text.String(), max(1, m.width), true)
}
