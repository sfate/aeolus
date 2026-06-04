package tmux

import (
	"fmt"
	"os/exec"
	"strconv"
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
	PID            int
	Width          int
	Height         int
	WindowName     string
}

// ListAllPanes returns all tmux panes across all sessions.
// Returns an error if tmux is not running or not available.
func ListAllPanes() ([]Pane, error) {
	format := "#{pane_id}|#{pane_index}|#{window_index}|#{session_name}|#{session_id}|#{pane_current_command}|#{pane_pid}|#{pane_width}|#{pane_height}|#{window_name}"
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

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	panes := make([]Pane, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 10 {
			continue
		}

		paneIndex, _ := strconv.Atoi(parts[1])
		windowIndex, _ := strconv.Atoi(parts[2])
		pid, _ := strconv.Atoi(parts[6])
		width, _ := strconv.Atoi(parts[7])
		height, _ := strconv.Atoi(parts[8])

		panes = append(panes, Pane{
			PaneID:         parts[0],
			PaneIndex:      paneIndex,
			WindowIndex:    windowIndex,
			SessionName:    parts[3],
			SessionID:      parts[4],
			CurrentCommand: parts[5],
			PID:            pid,
			Width:          width,
			Height:         height,
			WindowName:     parts[9],
		})
	}

	return panes, nil
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
