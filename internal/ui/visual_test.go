package ui

import (
	"github.com/sfate/aeolus/internal/monitor"
	"strings"
	"testing"
)

func TestThemeContrastAndSemanticStyles(t *testing.T) {
	for _, name := range []string{"dark", "light", "monochrome"} {
		theme := newDashboardTheme(name)
		if theme.colorHelp == "#555555" || theme.colorNoClaud == "#666666" {
			t.Errorf("%s theme keeps low-contrast secondary colors", name)
		}
		for _, style := range []struct {
			name string
			text string
			got  string
		}{
			{"title", "Command palette", theme.screenTitle("Command palette")},
			{"section", "Targets", theme.sectionLabel("Targets")},
			{"metadata", "session-1", theme.metadata("session-1")},
			{"primary", "[enter] Open", theme.primaryHint("[enter] Open")},
		} {
			if !strings.Contains(style.got, style.text) {
				t.Errorf("%s theme %s style lost label %q: %q", name, style.name, style.text, style.got)
			}
		}
	}
}

func TestFitScreenPreservesTitleFooterAndShowsClipping(t *testing.T) {
	got := fitScreenWithFooter("Palette\nFilter\nResult one\nResult two\nResult three\n[enter] Execute  [esc] Cancel", 32, 5)
	for _, want := range []string{"Palette", "[enter] Execute", "lines hidden"} {
		if !strings.Contains(got, want) {
			t.Errorf("fitScreen omitted %q: %q", want, got)
		}
	}
	if lines := strings.Count(got, "\n") + 1; lines > 5 {
		t.Fatalf("fitScreen returned %d lines for height 5", lines)
	}
}

func TestDashboardSummaryKeepsFiltersCompact(t *testing.T) {
	m := NewModel()
	s := testSession("%1")
	s.Pane.CurrentPath = "/Users/developer/projects/aeolus"
	m.sessions = []monitor.Session{s}
	m.allSessions = append([]monitor.Session{}, m.sessions...)
	summary := m.filterSummary()
	if strings.Contains(summary, `Working dir contains: ""`) || strings.Contains(summary, s.Pane.CurrentPath) || !strings.Contains(summary, "aeolus") || !strings.Contains(summary, "Sessions: 1/1") {
		t.Fatalf("filter summary is not compact: %q", summary)
	}
}
