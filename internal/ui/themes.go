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
	colorText, colorAccent                    lipgloss.Color
	colorPermission, colorPermBg              lipgloss.Color
	colorQuestion, colorQuestionBg            lipgloss.Color
	colorWorking, colorIdle, colorNoClaud     lipgloss.Color
	colorSelected, colorHelp, colorError      lipgloss.Color
	styleHeader, styleBorder, styleHelp       lipgloss.Style
	styleTitle, styleSection, styleMetadata   lipgloss.Style
	stylePrimary, styleSelected, styleError   lipgloss.Style
	stylePermWarning, stylePermBorder         lipgloss.Style
	styleQuestionBorder, styleQuestionWarning lipgloss.Style
}

func newDashboardTheme(name string) dashboardTheme {
	if name != "light" && name != "monochrome" {
		name = "dark"
	}
	t := dashboardTheme{
		name: name, renderer: lipgloss.NewRenderer(os.Stdout),
		colorBorder: "#475569", colorHeaderBg: "#1E293B", colorHeaderFg: "#F8FAFC",
		colorText: "#E2E8F0", colorAccent: "#7DD3FC",
		colorPermission: "#FFB347", colorPermBg: "#3D2E00",
		colorQuestion: "#C792EA", colorQuestionBg: "#2D1A3D",
		colorWorking: "#00CFCF", colorIdle: "#5AF78E", colorNoClaud: "#94A3B8",
		colorSelected: "#26364B", colorHelp: "#A0AEC0", colorError: "#FF5555",
	}
	if name == "light" {
		t.colorBorder, t.colorHeaderBg, t.colorHeaderFg = "#94A3B8", "#E2E8F0", "#0F172A"
		t.colorText, t.colorAccent = "#1E293B", "#0369A1"
		t.colorPermission, t.colorPermBg = "#8A4700", "#FFF1D5"
		t.colorQuestion, t.colorQuestionBg = "#6B2EA0", "#F3E8FF"
		t.colorWorking, t.colorIdle, t.colorNoClaud = "#006C79", "#176B35", "#475569"
		t.colorSelected, t.colorHelp, t.colorError = "#E2E8F0", "#475569", "#B42318"
	}
	if name == "monochrome" {
		t.renderer.SetColorProfile(termenv.Ascii)
	}
	t.styleHeader = t.renderer.NewStyle().Bold(true).Foreground(t.colorHeaderFg).Background(t.colorHeaderBg).Padding(0, 2)
	t.styleBorder = t.renderer.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.colorBorder)
	t.styleHelp = t.renderer.NewStyle().Foreground(t.colorHelp).Padding(0, 1)
	t.styleTitle = t.renderer.NewStyle().Bold(true).Foreground(t.colorAccent)
	t.styleSection = t.renderer.NewStyle().Bold(true).Foreground(t.colorAccent)
	t.styleMetadata = t.renderer.NewStyle().Foreground(t.colorHelp)
	t.stylePrimary = t.renderer.NewStyle().Bold(true).Foreground(t.colorAccent)
	t.styleSelected = t.renderer.NewStyle().Bold(true).Foreground(t.colorText).Background(t.colorSelected)
	t.styleError = t.renderer.NewStyle().Bold(true).Foreground(t.colorError)
	t.stylePermWarning = t.renderer.NewStyle().Bold(true).Foreground(t.colorPermission)
	t.stylePermBorder = t.renderer.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.colorPermission)
	t.styleQuestionBorder = t.renderer.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.colorQuestion)
	t.styleQuestionWarning = t.renderer.NewStyle().Bold(true).Foreground(t.colorQuestion)
	return t
}

func (t dashboardTheme) screenTitle(title string) string {
	return t.styleTitle.Render("◆ " + title)
}

func (t dashboardTheme) sectionLabel(label string) string {
	return t.styleSection.Render("── " + label + " ──")
}

func (t dashboardTheme) metadata(text string) string { return t.styleMetadata.Render(text) }

func (t dashboardTheme) primaryHint(text string) string { return t.stylePrimary.Render(text) }

func (t dashboardTheme) selectedRow(text string) string {
	return t.styleSelected.Render("▶ " + text)
}

func (t dashboardTheme) errorText(text string) string { return t.styleError.Render(text) }

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
