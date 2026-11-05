package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// fitLine clips by terminal cells without cutting Unicode clusters or ANSI codes.
func fitLine(text string, width int) string { return ansi.Truncate(text, max(0, width), "…") }

func fitScreen(text string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = fitLine(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func (m Model) paneDimensions() (listWidth, contentWidth, listHeight, contentHeight int) {
	listWidth = listPaneWidth(m.width)
	contentWidth = contentPaneWidth(m.width)
	listHeight = contentPaneHeight(m.height)
	contentHeight = listHeight
	if m.width < 80 {
		listHeight = min(3, max(1, (m.height-10)/4))
		contentHeight = max(1, m.height-9-listHeight)
	}
	if m.composer.active || m.filters.editing {
		contentWidth = max(1, m.width-2)
		contentHeight = contentPaneHeight(m.height)
	}
	return
}

func (m *Model) resizeViewport() {
	_, width, _, height := m.paneDimensions()
	m.viewport.Width = width
	m.viewport.Height = height
	m.viewport.SetYOffset(m.viewport.YOffset)
}

func (m Model) layoutFooter() string {
	if m.width >= 80 {
		return m.footer()
	}
	if m.composer.active {
		if m.composer.context.active {
			return "enter attach | esc back"
		}
		return "ctrl+s send | esc back"
	}
	if m.filters.editing {
		return "enter apply | esc back"
	}
	if m.search.active() {
		return m.searchHelp()
	}
	return "[j/k] [enter] [c] [q]"
}

// focusInput keeps the insertion cursor visible while retaining the original value.
func focusInput(text string, position, width int) string {
	if width <= 1 {
		return "▏"
	}
	runes := []rune(text)
	position = max(0, min(position, len(runes)))
	before := ansi.Hardwrap(string(runes[:position]), max(1, width-1), true)
	lines := strings.Split(before, "\n")
	return ansi.Truncate(lines[len(lines)-1]+"▏"+string(runes[position:]), max(1, width), "")
}
