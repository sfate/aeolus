package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

// Pane represents a tmux pane with its metadata.
type Pane struct {
	PaneID         string
	PaneIndex      int
	WindowIndex    int
	SessionName    string
	SessionID      string
	CurrentCommand string
	CurrentPath    string
	PID            int
	Width          int
	Height         int
	WindowName     string
	Shell          bool
	CaptureID      string
}

// ListAllPanes returns all tmux panes across all sessions.
// Returns an error if tmux is not running or not available.
func ListAllPanes() ([]Pane, error) {
	format := paneMetadataFormat()
	cmd := exec.Command("tmux", "list-panes", "-a", "-F", format)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "no server running") || strings.Contains(stderr, "can't open terminal") {
				return nil, fmt.Errorf("tmux server is not running: %w", err)
			}
		}
		return nil, fmt.Errorf("failed to list tmux panes: %w", err)
	}

	return parsePanes(out)
}

// CapturePane returns the last N lines of content from a tmux pane.
func CapturePane(paneID string, lastNLines int) (string, error) {
	startLine := fmt.Sprintf("-%d", lastNLines)
	// -e preserves ANSI escape sequences (colors, bold, etc.)
	cmd := exec.Command("tmux", "capture-pane", "-t", paneID, "-p", "-e", "-S", startLine)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane %s: %w", paneID, err)
	}
	return string(out), nil
}

// SendKeys sends keystrokes to a tmux pane followed by Enter.
func SendKeys(paneID string, keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", paneID, keys, "Enter")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send keys to pane %s: %w", paneID, err)
	}
	return nil
}

// SendKeySequence sends raw tmux key names to a pane.
func SendKeySequence(paneID string, keys ...string) error {
	args := append([]string{"send-keys", "-t", paneID}, keys...)
	cmd := exec.Command("tmux", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send key sequence to pane %s: %w", paneID, err)
	}
	return nil
}

// SwitchToPane makes the pane's tmux session/window/pane active for the current client.
func SwitchToPane(pane Pane) error {
	if err := exec.Command("tmux", "switch-client", "-t", pane.SessionID).Run(); err != nil {
		return fmt.Errorf("failed to switch to session %s: %w", pane.SessionID, err)
	}
	windowTarget := fmt.Sprintf("%s:%d", pane.SessionID, pane.WindowIndex)
	if err := exec.Command("tmux", "select-window", "-t", windowTarget).Run(); err != nil {
		return fmt.Errorf("failed to select window %s: %w", windowTarget, err)
	}
	if err := exec.Command("tmux", "select-pane", "-t", pane.PaneID).Run(); err != nil {
		return fmt.Errorf("failed to select pane %s: %w", pane.PaneID, err)
	}
	return nil
}

// KillPane closes a tmux pane.
func KillPane(paneID string) error {
	cmd := exec.Command("tmux", "kill-pane", "-t", paneID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill pane %s: %w", paneID, err)
	}
	return nil
}

// Backend is the pane transport used by the monitor and TUI.
type Backend interface {
	ListAllPanes() ([]Pane, error)
	CapturePane(string, int) (string, error)
	SendKeys(string, string) error
	SendKeySequence(string, ...string) error
	SwitchToPane(Pane) error
	KillPane(string) error
}

// Client executes pane operations through the local tmux executable.
type Client struct{}

func (Client) ListAllPanes() ([]Pane, error)                    { return ListAllPanes() }
func (Client) CapturePane(id string, lines int) (string, error) { return CapturePane(id, lines) }
func (Client) SendKeys(id, keys string) error                   { return SendKeys(id, keys) }
func (Client) SendKeySequence(id string, keys ...string) error  { return SendKeySequence(id, keys...) }
func (Client) SwitchToPane(pane Pane) error                     { return SwitchToPane(pane) }
func (Client) KillPane(id string) error                         { return KillPane(id) }

var _ Backend = Client{}
