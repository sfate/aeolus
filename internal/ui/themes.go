package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/sfate/aeolus/internal/monitor"
)

// dashboardTheme owns its renderer and styles; constructing another model cannot alter this model's appearance or the process-wide Lip Gloss renderer.
type dashboardTheme struct {
	name                                      string
	renderer                                  *lipgloss.Renderer
	colorBorder, colorHeaderBg, colorHeaderFg lipgloss.Color
	colorPermission, colorPermBg              lipgloss.Color
	colorQuestion, colorQuestionBg            lipgloss.Color
	colorWorking, colorIdle, colorNoClaud     lipgloss.Color
	colorSelected, colorHelp, colorError      lipgloss.Color
	styleHeader, styleBorder, styleHelp       lipgloss.Style
	stylePermWarning, stylePermBorder         lipgloss.Style
	styleQuestionBorder, styleQuestionWarning lipgloss.Style
}

func newDashboardTheme(name string) dashboardTheme {
	if name != "light" && name != "monochrome" {
		name = "dark"
	}
	t := dashboardTheme{
		name: name, renderer: lipgloss.NewRenderer(os.Stdout),
		colorBorder: "#444444", colorHeaderBg: "#1A3A5C", colorHeaderFg: "#FFFFFF",
		colorPermission: "#FFB347", colorPermBg: "#3D2E00",
		colorQuestion: "#C792EA", colorQuestionBg: "#2D1A3D",
		colorWorking: "#00CFCF", colorIdle: "#5AF78E", colorNoClaud: "#666666",
		colorSelected: "#1A1A2E", colorHelp: "#555555", colorError: "#FF5555",
	}
	if name == "light" {
		t.colorBorder, t.colorHeaderBg, t.colorHeaderFg = "#697586", "#DCEBFA", "#152536"
		t.colorPermission, t.colorPermBg = "#8A4700", "#FFF1D5"
		t.colorQuestion, t.colorQuestionBg = "#6B2EA0", "#F3E8FF"
		t.colorWorking, t.colorIdle, t.colorNoClaud = "#006C79", "#176B35", "#5E6772"
		t.colorSelected, t.colorHelp, t.colorError = "#E5ECF4", "#526173", "#B42318"
	}
	if name == "monochrome" {
		t.renderer.SetColorProfile(termenv.Ascii)
	}
	t.styleHeader = t.renderer.NewStyle().Bold(true).Foreground(t.colorHeaderFg).Background(t.colorHeaderBg).Padding(0, 2)
	t.styleBorder = t.renderer.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.colorBorder)
	t.styleHelp = t.renderer.NewStyle().Foreground(t.colorHelp).Padding(0, 1)
	t.stylePermWarning = t.renderer.NewStyle().Bold(true).Foreground(t.colorPermission)
	t.stylePermBorder = t.renderer.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.colorPermission)
	t.styleQuestionBorder = t.renderer.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.colorQuestion)
	t.styleQuestionWarning = t.renderer.NewStyle().Bold(true).Foreground(t.colorQuestion)
	return t
}

func (m Model) statusStyle(status monitor.Status) lipgloss.Style {
	style := m.theme.renderer.NewStyle()
	switch status {
	case monitor.StatusPermission:
		return style.Bold(true).Foreground(m.theme.colorPermission)
	case monitor.StatusQuestion:
		return style.Bold(true).Foreground(m.theme.colorQuestion)
	case monitor.StatusWorking:
		return style.Foreground(m.theme.colorWorking)
	case monitor.StatusIdle:
		return style.Foreground(m.theme.colorIdle)
	default:
		return style.Foreground(m.theme.colorNoClaud)
	}
}
