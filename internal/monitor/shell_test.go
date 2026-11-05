package monitor

import (
	"testing"

	"github.com/sfate/aeolus/internal/tmux"
)

func TestManagedShellRequiresExplicitMarker(t *testing.T) {
	m := New(fakePaneSource{})
	m.processes = func() ([]processInfo, error) { return nil, nil }
	got := m.Refresh([]tmux.Pane{{PaneID: "%1", CurrentCommand: "zsh"}, {PaneID: "%2", CurrentCommand: "sleep", Shell: true}})
	if len(got) != 1 || got[0].Pane.PaneID != "%2" || got[0].Kind != SessionShell || got[0].Provider != ProviderUnknown || got[0].CanRespond() {
		t.Fatalf("managed shell detection: %#v", got)
	}
}

func TestAgentDetectionWinsInManagedShell(t *testing.T) {
	m := New(fakePaneSource{})
	m.processes = func() ([]processInfo, error) { return nil, nil }
	got := m.Refresh([]tmux.Pane{{PaneID: "%1", CurrentCommand: "claude", Shell: true}})
	if len(got) != 1 || got[0].Kind != SessionAgent || got[0].Provider != ProviderClaude {
		t.Fatalf("agent detection lost inside shell: %#v", got)
	}
}
