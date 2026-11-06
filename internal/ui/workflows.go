package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
	store "github.com/sfate/aeolus/internal/workflows"
)

type workflowMode int

const (
	workflowBrowse workflowMode = iota
	workflowNaming
	workflowParameters
)

type workflowModal struct {
	active, pending, saving       bool
	mode                          workflowMode
	path, name, template          string
	position, selected, parameter int
	request                       uint64
	entries                       []store.Entry
	names, values                 []string
	err                           error
}

type workflowOperations struct {
	path func() (string, error)
	load func(string) ([]store.Entry, error)
	save func(string, string, string) error
}

type workflowResultMsg struct {
	request            uint64
	paneID, path, name string
	entries            []store.Entry
	err                error
	saved              bool
}

func (m *Model) workflowOperation(save bool) tea.Cmd {
	a := &m.composer.workflows
	m.workflowRequest++
	a.request, a.pending, a.saving, a.err = m.workflowRequest, true, save, nil
	request, target, path, name, template := a.request, m.composer.paneID, a.path, a.name, m.composer.draft
	ops := m.workflowIO
	if ops.path == nil {
		ops.path = store.DefaultPath
	}
	if ops.load == nil {
		ops.load = store.Load
	}
	if ops.save == nil {
		ops.save = store.Save
	}
	return func() tea.Msg {
		result := workflowResultMsg{request: request, paneID: target, path: path, name: name}
		if result.path == "" {
			result.path, result.err = ops.path()
		}
		if result.err == nil && save {
			result.err = ops.save(result.path, name, template)
			result.saved = result.err == nil
		}
		if result.err == nil {
			result.entries, result.err = ops.load(result.path)
			if result.saved && result.err != nil {
				result.err = fmt.Errorf("workflow saved but reload failed: %w", result.err)
			}
		}
		return result
	}
}

func (m *Model) workflowKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	c := &m.composer
	a := &c.workflows
	if !a.active {
		if c.kind != monitor.SessionShell || msg.String() != "ctrl+w" {
			return false, nil
		}
		if !m.composerTargetAvailable() {
			c.err = fmt.Errorf("target shell changed or disappeared; draft retained")
			m.updateViewport()
			return true, nil
		}
		*a = workflowModal{active: true}
		cmd := m.workflowOperation(false)
		m.updateViewport()
		return true, cmd
	}
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return true, tea.Quit
	}
	if a.pending {
		if !a.saving && msg.String() == "esc" {
			*a = workflowModal{}
			m.updateViewport()
		}
		return true, nil
	}
	if msg.String() == "pgup" || msg.String() == "pgdown" {
		step := m.viewport.Height
		if msg.String() == "pgup" {
			step = -step
		}
		m.viewport.SetYOffset(m.viewport.YOffset + step)
		return true, nil
	}
	if msg.String() == "esc" {
		if a.mode == workflowBrowse {
			*a = workflowModal{}
		} else {
			a.mode = workflowBrowse
			a.names, a.values = nil, nil
			a.err = nil
		}
		m.updateViewport()
		return true, nil
	}
	switch a.mode {
	case workflowNaming:
		if msg.String() == "enter" {
			if !m.composerTargetAvailable() {
				a.err = fmt.Errorf("target shell changed or disappeared; draft retained")
			} else if a.path == "" {
				a.err = fmt.Errorf("workflow path unavailable; reopen after fixing data directory settings")
			} else {
				cmd := m.workflowOperation(true)
				m.updateViewport()
				return true, cmd
			}
		} else {
			a.name, a.position, a.err = workflowEdit(msg, a.name, a.position, 80, false)
		}
	case workflowParameters:
		switch msg.String() {
		case "enter":
			if a.parameter < len(a.names)-1 {
				a.parameter++
				a.position = len([]rune(a.values[a.parameter]))
			} else {
				m.insertWorkflow()
			}
		case "tab", "shift+tab":
			step := 1
			if msg.String() == "shift+tab" {
				step = -1
			}
			a.parameter = (a.parameter + step + len(a.names)) % len(a.names)
			a.position = len([]rune(a.values[a.parameter]))
		default:
			a.values[a.parameter], a.position, a.err = workflowEdit(msg, a.values[a.parameter], a.position, store.MaxTextBytes, true)
		}
	default:
		switch msg.String() {
		case "j", "down":
			a.selected = min(max(0, len(a.entries)-1), a.selected+1)
		case "k", "up":
			a.selected = max(0, a.selected-1)
		case "s":
			a.mode, a.name, a.position, a.err = workflowNaming, "", 0, nil
		case "ctrl+r":
			cmd := m.workflowOperation(false)
			m.updateViewport()
			return true, cmd
		case "enter":
			if !m.composerTargetAvailable() {
				a.err = fmt.Errorf("target shell changed or disappeared; draft retained")
				break
			}
			if a.selected >= len(a.entries) {
				break
			}
			entry := a.entries[a.selected]
			names, err := store.Parameters(entry.Template)
			if err != nil {
				a.err = err
				break
			}
			a.name, a.template, a.names, a.values, a.parameter, a.position, a.err = entry.Name, entry.Template, names, make([]string, len(names)), 0, 0, nil
			if len(names) == 0 {
				m.insertWorkflow()
			} else {
				a.mode = workflowParameters
			}
		}
	}
	m.updateViewport()
	return true, nil
}

func (m *Model) insertWorkflow() {
	c := &m.composer
	a := &c.workflows
	if !m.composerTargetAvailable() {
		a.err = fmt.Errorf("target shell changed or disappeared; draft and parameter values retained")
		return
	}
	values := map[string]string{}
	for i, name := range a.names {
		values[name] = a.values[i]
	}
	text, err := store.Render(a.template, values)
	if err != nil {
		a.err = err
		return
	}
	c.insert([]rune(text))
	*a = workflowModal{}
}

func (m *Model) workflowResult(msg workflowResultMsg) {
	c := &m.composer
	a := &c.workflows
	if !c.active || !a.active || !a.pending || a.request != msg.request || c.paneID != msg.paneID {
		return
	}
	a.pending, a.saving, a.path, a.err = false, false, msg.path, msg.err
	if msg.saved {
		a.mode = workflowBrowse
	}
	if msg.err == nil {
		a.entries = msg.entries
		a.selected = min(a.selected, max(0, len(a.entries)-1))
		if msg.saved {
			for i, entry := range a.entries {
				if entry.Name == msg.name {
					a.selected = i
					break
				}
			}
		}
	}
	m.updateViewport()
}

func workflowEdit(msg tea.KeyMsg, text string, position, limit int, parameter bool) (string, int, error) {
	runes := []rune(text)
	switch msg.String() {
	case "left":
		position = max(0, position-1)
	case "right":
		position = min(len(runes), position+1)
	case "home", "ctrl+a":
		position = 0
	case "end", "ctrl+e":
		position = len(runes)
	case "ctrl+u":
		text, position = "", 0
	case "backspace", "ctrl+h":
		if position > 0 {
			text = string(append(runes[:position-1], runes[position:]...))
			position--
		}
	case "delete":
		if position < len(runes) {
			text = string(append(runes[:position], runes[position+1:]...))
		}
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			input := msg.Runes
			if msg.Type == tea.KeySpace {
				input = []rune{' '}
			}
			clean := []rune{}
			for _, r := range input {
				if !unicode.IsControl(r) || (parameter && (r == '\n' || r == '\t')) {
					clean = append(clean, r)
				}
			}
			next := append([]rune{}, runes[:position]...)
			next = append(next, clean...)
			next = append(next, runes[position:]...)
			if (!parameter && len(next) > limit) || (parameter && len(string(next)) > limit) {
				return text, position, fmt.Errorf("input exceeds the field limit")
			}
			text, position = string(next), position+len(clean)
		}
	}
	return text, position, nil
}

func (m Model) workflowView() string {
	a := m.composer.workflows
	var text strings.Builder
	text.WriteString("Shell workflows - insert for review; nothing sends automatically\n\n")
	if a.pending {
		if a.saving {
			text.WriteString("Saving...\n")
		} else {
			text.WriteString("Loading...\n")
		}
	}
	if a.err != nil {
		fmt.Fprintf(&text, "Error: %q\n\n", a.err.Error())
	}
	if a.mode == workflowNaming {
		r := []rune(a.name)
		fmt.Fprintf(&text, "Save draft as: %s▏%s\n\nOnly the draft template is saved. Avoid secrets in saved text.\n\nTemplate:\n%s", string(r[:a.position]), string(r[a.position:]), m.composer.draft)
		return text.String()
	}
	if a.mode == workflowParameters {
		r := []rune(a.values[a.parameter])
		fmt.Fprintf(&text, "Workflow: %s\nParameter %d/%d: %s\nValue: %s▏%s\n\nValues are temporary and are not saved.\n\nTemplate:\n%s", a.name, a.parameter+1, len(a.names), a.names[a.parameter], string(r[:a.position]), string(r[a.position:]), a.template)
		return text.String()
	}
	if len(a.entries) == 0 {
		text.WriteString("No saved workflows. Press s to save the current draft.\n")
	}
	start := max(0, a.selected-4)
	for i := start; i < len(a.entries) && i < start+8; i++ {
		marker := "  "
		if i == a.selected {
			marker = "> "
		}
		fmt.Fprintf(&text, "%s%s\n", marker, a.entries[i].Name)
	}
	if a.selected < len(a.entries) {
		text.WriteString("\nSelected template:\n" + a.entries[a.selected].Template)
	}
	return text.String()
}

func (m Model) workflowFooter() string {
	a := m.composer.workflows
	if a.pending {
		if a.saving {
			return "Saving... [ctrl+c] Quit"
		}
		return "Loading... [esc] Back to draft"
	}
	switch a.mode {
	case workflowNaming:
		return "[enter] Save template  [esc] Back to list  [ctrl+u] Clear name"
	case workflowParameters:
		return "[enter] Next / insert for review  [tab/shift+tab] Parameter  [esc] Back to list"
	default:
		return "[↑↓/j/k] Select  [enter] Use  [s] Save draft  [ctrl+r] Reload  [pgup/pgdown] Scroll  [esc] Draft"
	}
}
