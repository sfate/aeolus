package monitor

import (
	"errors"
	"github.com/sfate/aeolus/internal/tmux"
	"testing"
)

func TestProviderExecutables(t *testing.T) {
	cases := []struct {
		args string
		want Provider
	}{
		{"claude --resume", ProviderClaude},
		{"/opt/bin/codex --resume", ProviderCodex},
		{"gemini", ProviderGemini},
		{"node /opt/bin/gemini", ProviderGemini},
		{"node /opt/bin/codex", ProviderCodex},
		{"node /opt/bin/claude", ProviderClaude},
		{"node /usr/lib/node_modules/@anthropic-ai/claude-code/cli.js", ProviderClaude},
		{"node /usr/lib/node_modules/@openai/codex/bin/codex.js", ProviderCodex},
		{"node /usr/lib/node_modules/@google/gemini-cli/dist/index.js", ProviderGemini},
		{"/opt/claude-tools/server", ProviderUnknown},
		{"echo claude", ProviderUnknown},
		{"cat /tmp/codex", ProviderUnknown},
		{"vim /usr/lib/node_modules/@google/gemini-cli/dist/index.js", ProviderUnknown},
		{"node -e 'codex'", ProviderUnknown},
		{"claude-notes", ProviderUnknown},
		{"git commit -m claude", ProviderUnknown},
	}
	for _, tt := range cases {
		t.Run(tt.args, func(t *testing.T) {
			if got := providerForProcess(tt.args); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderRefresh(t *testing.T) {
	m := New(fakePaneSource{content: map[string]string{"%1": "Allow once", "%2": "Allow once", "%3": "Allow once"}})
	m.processes = func() ([]processInfo, error) {
		return []processInfo{{PID: 11, PPID: 10, Args: "node /usr/lib/node_modules/@google/gemini-cli/dist/index.js"}}, nil
	}
	got := m.Refresh([]tmux.Pane{{PaneID: "%1", CurrentCommand: "claude"}, {PaneID: "%2", CurrentCommand: "codex"}, {PaneID: "%3", PID: 10, CurrentCommand: "node"}, {PaneID: "%4", CurrentCommand: "bash"}})
	if len(got) != 3 {
		t.Fatalf("got %d sessions: %#v", len(got), got)
	}
	for i, want := range []Provider{ProviderClaude, ProviderCodex, ProviderGemini} {
		if got[i].Provider != want {
			t.Fatalf("session %d: %#v", i, got[i])
		}
	}
	if got[0].Status != StatusPermission || !got[0].CanRespond() {
		t.Fatal("Claude behavior lost")
	}
	for _, s := range got[1:] {
		if s.Status != StatusUnknown || s.CanRespond() || s.Request != nil {
			t.Fatalf("unsupported interpretation: %#v", s)
		}
	}
}

type brokenPaneSource struct{}

func (brokenPaneSource) CapturePane(string, int) (string, error) {
	return "", errors.New("capture failed")
}

func TestDetectedProviderSurvivesCaptureError(t *testing.T) {
	m := New(brokenPaneSource{})
	m.processes = func() ([]processInfo, error) { return nil, nil }
	got := m.Refresh([]tmux.Pane{{PaneID: "%1", CurrentCommand: "codex"}})
	if len(got) != 1 || got[0].Provider != ProviderCodex || got[0].Status != StatusUnknown {
		t.Fatalf("got %#v", got)
	}
}

func TestProviderDescendantsPreferParentAgent(t *testing.T) {
	m := New(fakePaneSource{})
	m.processes = func() ([]processInfo, error) {
		return []processInfo{{PID: 2, PPID: 1, Args: "codex"}, {PID: 3, PPID: 2, Args: "claude"}, {PID: 1, PPID: 3, Args: "bash"}}, nil
	}
	got := m.Refresh([]tmux.Pane{{PaneID: "%1", PID: 1}})
	if len(got) != 1 || got[0].Provider != ProviderCodex {
		t.Fatalf("got %#v", got)
	}
}
