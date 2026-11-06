package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/sfate/aeolus/internal/config"
	"github.com/sfate/aeolus/internal/monitor"
)

func themedModel(t *testing.T, theme string) Model {
	t.Helper()
	m := NewModelWithConfig(config.Config{Theme: theme})
	m.theme.renderer.SetColorProfile(termenv.TrueColor)
	m.sessions = []monitor.Session{testSession("%1")}
	m.sessions[0].Status = monitor.StatusPermission
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	return m
}

func TestThemesRenderIndependently(t *testing.T) {
	dark := themedModel(t, "dark")
	before := dark.View()
	light := themedModel(t, "light")
	if before == ansi.Strip(before) || light.View() == before {
		t.Fatal("dark and light themes did not render different colors")
	}
	if ansi.Strip(before) != ansi.Strip(light.View()) {
		t.Fatal("theme changed visible content or layout")
	}
	mono := themedModel(t, "monochrome")
	mono.sessions[0].Content = "\x1b[31mcolored captured output\x1b[0m"
	mono.updateViewport()
	view := mono.View()
	if view != ansi.Strip(view) || !strings.Contains(view, "colored captured output") {
		t.Fatal("monochrome retained ANSI escapes or lost output")
	}
	if after := dark.View(); after != before {
		t.Fatal("another model changed dark theme styling")
	}
	if NewModel().theme.name != "dark" || NewModelWithConfig(config.Config{}).theme.name != "dark" {
		t.Fatal("default theme changed")
	}
}
