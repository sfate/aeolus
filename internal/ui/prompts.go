package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	store "github.com/sfate/aeolus/internal/prompts"
)

type promptLibrary struct {
	active, pending, naming, saving bool
	path, name                      string
	position, selected              int
	request                         uint64
	entries                         []store.Entry
	err                             error
}
type libraryResultMsg struct {
	request      uint64
	paneID, path string
	entries      []store.Entry
	err          error
	saved        bool
}

func (m *Model) loadLibrary() tea.Cmd {
	m.promptRequest++
	m.composer.library.request = m.promptRequest
	m.composer.library.pending = true
	request, target := m.promptRequest, m.composer.paneID
	return func() tea.Msg {
		path, err := store.DefaultPath()
		var entries []store.Entry
		if err == nil {
			entries, err = store.Load(path)
		}
		return libraryResultMsg{request: request, paneID: target, path: path, entries: entries, err: err}
	}
}
func (m *Model) libraryKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	c := &m.composer
	a := &c.library
	if !a.active {
		if msg.String() != "ctrl+p" {
			return false, nil
		}
		*a = promptLibrary{active: true}
		cmd := m.loadLibrary()
		m.updateViewport()
		return true, cmd
	}
	if a.pending {
		if !a.saving && msg.String() == "esc" {
			a.active = false
			m.updateViewport()
		}
		return true, nil
	}
	if a.naming {
		r := []rune(a.name)
		switch msg.String() {
		case "esc":
			a.naming = false
			a.err = nil
		case "enter":
			if !m.composerTargetAvailable() {
				a.err = fmt.Errorf("target changed or disappeared; draft retained")
				break
			}
			if a.path == "" {
				a.err = fmt.Errorf("library path unavailable; reopen after correcting data directory settings")
				break
			}
			m.promptRequest++
			a.request = m.promptRequest
			a.pending = true
			a.saving = true
			a.err = nil
			request, target, path, name, text := a.request, c.paneID, a.path, a.name, c.draft
			m.updateViewport()
			return true, func() tea.Msg {
				err := store.Save(path, name, text)
				var entries []store.Entry
				if err == nil {
					entries, err = store.Load(path)
				}
				return libraryResultMsg{request: request, paneID: target, path: path, entries: entries, err: err, saved: err == nil}
			}
		case "left":
			if a.position > 0 {
				a.position--
			}
		case "right":
			if a.position < len(r) {
				a.position++
			}
		case "home", "ctrl+a":
			a.position = 0
		case "end", "ctrl+e":
			a.position = len(r)
		case "backspace", "ctrl+h":
			if a.position > 0 {
				a.name = string(append(r[:a.position-1], r[a.position:]...))
				a.position--
			}
		case "delete":
			if a.position < len(r) {
				a.name = string(append(r[:a.position], r[a.position+1:]...))
			}
		case "ctrl+u":
			a.name = ""
			a.position = 0
		default:
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
				input := msg.Runes
				if msg.Type == tea.KeySpace {
					input = []rune{' '}
				}
				var clean []rune
				for _, v := range input {
					if !unicode.IsControl(v) {
						clean = append(clean, v)
					}
				}
				if len(r)+len(clean) <= 80 {
					next := append([]rune{}, r[:a.position]...)
					next = append(next, clean...)
					next = append(next, r[a.position:]...)
					a.name = string(next)
					a.position += len(clean)
				}
			}
		}
	} else {
		switch msg.String() {
		case "esc":
			a.active = false
		case "j", "down":
			if a.selected < len(a.entries)-1 {
				a.selected++
			}
		case "k", "up":
			if a.selected > 0 {
				a.selected--
			}
		case "s":
			a.naming = true
			a.name = ""
			a.position = 0
			a.err = nil
		case "ctrl+r":
			cmd := m.loadLibrary()
			m.updateViewport()
			return true, cmd
		case "enter":
			if !m.composerTargetAvailable() {
				a.err = fmt.Errorf("target changed or disappeared; draft retained")
				break
			}
			if a.selected < len(a.entries) {
				c.insert([]rune(a.entries[a.selected].Text))
				a.active = false
			}
		case "pgdown":
			m.viewport.SetYOffset(m.viewport.YOffset + m.viewport.Height)
			return true, nil
		case "pgup":
			m.viewport.SetYOffset(m.viewport.YOffset - m.viewport.Height)
			return true, nil
		}
	}
	m.updateViewport()
	return true, nil
}
func (m *Model) libraryResult(msg libraryResultMsg) {
	c := &m.composer
	a := &c.library
	if !c.active || !a.active || !a.pending || a.request != msg.request || c.paneID != msg.paneID {
		return
	}
	a.pending = false
	a.saving = false
	a.path = msg.path
	a.err = msg.err
	if msg.err == nil {
		a.entries = msg.entries
		if a.selected >= len(a.entries) {
			a.selected = 0
		}
		if msg.saved {
			a.naming = false
		}
	}
	m.updateViewport()
}
func (m Model) libraryView() string {
	a := m.composer.library
	var b strings.Builder
	b.WriteString(m.theme.screenTitle("Reusable prompts") + "\n" + m.theme.metadata("Insert for review; nothing sends automatically") + "\n")
	if a.pending {
		if a.saving {
			b.WriteString("Saving...\n")
		} else {
			b.WriteString("Loading...\n")
		}
	}
	if a.err != nil {
		fmt.Fprintf(&b, "%s\n", m.theme.errorText(fmt.Sprintf("Error: %q", a.err.Error())))
	}
	if a.naming {
		r := []rune(a.name)
		fmt.Fprintf(&b, "Save draft as: %s▏%s\nDuplicate names are rejected.\n", string(r[:a.position]), string(r[a.position:]))
		return b.String()
	}
	if len(a.entries) == 0 {
		b.WriteString("No saved prompts. Press s to save the current draft.\n")
	}
	start := max(0, a.selected-5)
	for i := start; i < len(a.entries) && i < start+10; i++ {
		marker := "  "
		if i == a.selected {
			marker = "> "
		}
		if i == a.selected {
			b.WriteString(m.theme.selectedRow(a.entries[i].Name) + "\n")
		} else {
			fmt.Fprintf(&b, "%s%s\n", marker, a.entries[i].Name)
		}
	}
	if a.selected < len(a.entries) {
		b.WriteString("\n" + m.theme.sectionLabel("Selected prompt") + "\n" + a.entries[a.selected].Text)
	}
	return b.String()
}
