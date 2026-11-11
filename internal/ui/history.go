package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfate/aeolus/internal/history"
	"github.com/sfate/aeolus/internal/monitor"
)

type historyLibrary struct {
	active, loading bool
	request         uint64
	entries         []history.Entry
	query           string
	selection       int
	err             error
}

type historyLoadedMsg struct {
	request uint64
	entries []history.Entry
	err     error
}
type historySavedMsg struct {
	evicted int
	err     error
}

func (m *Model) historyKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	h := &m.history
	if !h.active {
		if msg.String() != "ctrl+r" || m.composer.kind != monitor.SessionShell {
			return false, nil
		}
		m.historyRequest++
		*h = historyLibrary{active: true, loading: true, request: m.historyRequest}
		request := h.request
		load := m.historyLoad
		if load == nil {
			load = func() ([]history.Entry, error) {
				path, err := history.DefaultPath()
				if err != nil {
					return nil, err
				}
				return history.Load(path)
			}
		}
		m.updateViewport()
		return true, func() tea.Msg { entries, err := load(); return historyLoadedMsg{request, entries, err} }
	}
	switch msg.String() {
	case "esc":
		m.history = historyLibrary{}
	case "ctrl+c":
		m.quitting = true
		return true, tea.Quit
	case "enter":
		if h.loading {
			break
		}
		matches := h.matches()
		if len(matches) == 0 {
			break
		}
		if !m.composerTargetAvailable() {
			h.err = fmt.Errorf("target shell changed or disappeared; draft retained")
			break
		}
		m.composer.insert([]rune(matches[h.selection].Command))
		m.history = historyLibrary{}
	case "up", "ctrl+p":
		if h.selection > 0 {
			h.selection--
		}
	case "down", "ctrl+n":
		if h.selection+1 < len(h.matches()) {
			h.selection++
		}
	case "backspace", "ctrl+h":
		runes := []rune(h.query)
		if len(runes) > 0 {
			h.query = string(runes[:len(runes)-1])
			h.selection = 0
		}
	case "ctrl+u":
		h.query = ""
		h.selection = 0
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			input := msg.Runes
			if msg.Type == tea.KeySpace {
				input = []rune{' '}
			}
			for _, r := range input {
				if !unicode.IsControl(r) && len(h.query) < 4096 {
					h.query += string(r)
				}
			}
			h.selection = 0
		}
	}
	m.updateViewport()
	return true, nil
}

func (h historyLibrary) matches() []history.Entry {
	result := []history.Entry{}
	query := strings.ToLower(h.query)
	for i := len(h.entries) - 1; i >= 0; i-- {
		entry := h.entries[i]
		if strings.Contains(strings.ToLower(entry.Command), query) || strings.Contains(strings.ToLower(entry.Directory), query) {
			result = append(result, entry)
		}
	}
	return result
}

func (m *Model) finishHistoryLoad(msg historyLoadedMsg) {
	if !m.history.active || !m.composer.active || m.history.request != msg.request {
		return
	}
	m.history.loading = false
	m.history.entries = msg.entries
	m.history.err = msg.err
	m.updateViewport()
}

func (m Model) historyView() string {
	h := m.history
	text := m.theme.screenTitle("Saved shell command history") + "\n\n" + m.theme.sectionLabel("Search") + " " + ansi.Truncate(h.query, max(4, m.viewport.Width-10), "…") + "▏\n\n"
	if h.loading {
		return text + "Loading... [esc] Back to draft"
	}
	if h.err != nil {
		text += m.theme.errorText("Error: "+safeReviewText(h.err.Error())) + "\n\n"
	}
	matches := h.matches()
	if len(matches) == 0 {
		text += "No matching saved commands. Commands are saved only explicitly from captured blocks.\n"
	}
	visible := max(1, m.viewport.Height-10)
	start := max(0, min(h.selection-visible/2, len(matches)-visible))
	end := min(len(matches), start+visible)
	for offset, entry := range matches[start:end] {
		i := start + offset
		marker := "  "
		if i == h.selection {
			marker = "> "
		}
		command := strings.ReplaceAll(entry.Command, "\n", " ↵ ")
		command = ansi.Truncate(safeReviewText(command), max(1, m.viewport.Width-2), "…")
		if i == h.selection {
			text += m.theme.selectedRow(command) + "\n"
		} else {
			text += fmt.Sprintf("%s%s\n", marker, command)
		}
	}
	if len(matches) > 0 {
		entry := matches[h.selection]
		status := "unknown"
		if entry.ExitCode != nil {
			status = fmt.Sprint(*entry.ExitCode)
		}
		text += m.theme.metadata(fmt.Sprintf("\nDirectory: %q  Exit: %s  Duration: %s\n", entry.Directory, status, entry.Duration))
	}
	return text + "\n" + m.theme.primaryHint("[↑↓/ctrl+p/ctrl+n] Select  [ctrl+u] Clear search  [enter] Insert for review  [esc] Back")
}

// saveHistoryEntry is invoked only by an explicit selected-block save action.
func (m *Model) saveHistoryEntry(entry history.Entry) tea.Cmd {
	if m.historySaving {
		return nil
	}
	save := m.historyWrite
	if save == nil {
		save = func(e history.Entry) (int, error) {
			path, err := history.DefaultPath()
			if err != nil {
				return 0, err
			}
			return history.Append(path, e)
		}
	}
	m.historySaving = true
	m.historyNotice = "Saving selected command..."
	return func() tea.Msg { evicted, err := save(entry); return historySavedMsg{evicted, err} }
}
