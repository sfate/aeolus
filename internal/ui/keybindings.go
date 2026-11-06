package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/config"
)

func (m Model) dashboardKey(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	if len(m.keybindings) == 0 {
		return msg, true
	}
	pressed := msg.String()
	for action, canonical := range config.DashboardKeys {
		binding := canonical
		if custom, ok := m.keybindings[action]; ok {
			binding = custom
		}
		if pressed == binding {
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(canonical)}, true
		}
	}
	for _, canonical := range config.DashboardKeys {
		if pressed == canonical {
			return msg, false
		}
	}
	return msg, true
}

func (m Model) shortcutHelp(text string) string {
	pairs := []string{"[↑↓/jk]", "[↑/" + m.dashboardBinding("previous") + " ↓/" + m.dashboardBinding("next") + "]", "[j/down]", "[" + m.dashboardBinding("next") + "/down]", "[k/up]", "[" + m.dashboardBinding("previous") + "/up]"}
	for action, custom := range m.keybindings {
		canonical := config.DashboardKeys[action]
		pairs = append(pairs, "["+canonical+"]", "["+custom+"]")
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

func (m Model) dashboardBinding(action string) string {
	if custom, ok := m.keybindings[action]; ok {
		return custom
	}
	return config.DashboardKeys[action]
}

func (m Model) dashboardBindingForCanonical(key string) string {
	for action, canonical := range config.DashboardKeys {
		if canonical == key {
			return m.dashboardBinding(action)
		}
	}
	return key
}
