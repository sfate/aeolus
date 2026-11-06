package ui

import "fmt"

func launchProviderIndex(provider string) int {
	for i, p := range launchProviders {
		if string(p) == provider {
			return i
		}
	}
	return 0
}

func (m *Model) cycleProjectPreset() {
	if len(m.projects) == 0 {
		return
	}
	form := &m.lifecycle
	form.shell = false
	form.preset = (form.preset + 1) % len(m.projects)
	project := m.projects[form.preset]
	form.text = project.Directory
	form.position = len([]rune(form.text))
	form.provider = launchProviderIndex(project.Provider)
	form.err = nil
}

func (m Model) projectPresetLabel() string {
	if m.lifecycle.preset < 0 || m.lifecycle.preset >= len(m.projects) {
		return "Manual"
	}
	return fmt.Sprintf("%q", m.projects[m.lifecycle.preset].Name)
}
