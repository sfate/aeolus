package tmux

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sfate/aeolus/internal/shellblocks"
)

// CapturedShellLifecycle is optional transport support for explicitly opted-in
// terminal capture in a new managed shell. It never attaches to existing panes.
type CapturedShellLifecycle interface {
	CreateCapturedShellSession(directory string) (Pane, error)
}

func quoteShellArgument(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (Client) CreateCapturedShellSession(directory string) (Pane, error) {
	executable, err := os.Executable()
	if err != nil {
		return Pane{}, err
	}
	return createCapturedShellSession(directory, executable)
}

func createCapturedShellSession(directory, executable string) (Pane, error) {
	capture, err := shellblocks.PrepareCapture()
	if err != nil {
		return Pane{}, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = shellblocks.DiscardCapture(capture.ID)
		}
	}()
	// A failed tmux call may still have started a child. Its live ZDOTDIR must not
	// disappear while zsh is sourcing startup files.
	pane, err := createSessionWithCapture(directory, "zsh", true, capture.Directory, func() { keep = true })
	if err != nil {
		if keep {
			return Pane{}, fmt.Errorf("captured shell launch failed; bootstrap %s is retained until manual recovery: %w", capture.Directory, err)
		}
		return Pane{}, err
	}
	fail := func(err error) (Pane, error) {
		return Pane{}, fmt.Errorf("shell pane %s was created but capture setup failed; private bootstrap %s is retained, inspect the pane before retrying: %w", pane.PaneID, capture.Directory, err)
	}
	pipe, err := lifecycleOutput("display-message", "-p", "-t", pane.PaneID, "#{pane_pipe}")
	if err != nil {
		return fail(err)
	}
	if strings.TrimSpace(string(pipe)) != "0" {
		return fail(fmt.Errorf("pane already has an output pipe; it was not replaced"))
	}
	if _, err := lifecycleOutput("set-option", "-p", "-t", pane.PaneID, "@aeolus-capture", capture.ID); err != nil {
		return fail(err)
	}
	command := quoteShellArgument(executable) + " --internal-shell-collector " + quoteShellArgument(capture.ID)
	if _, err := lifecycleOutput("pipe-pane", "-o", "-O", "-t", pane.PaneID, command); err != nil {
		return fail(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := shellblocks.ReadCapture(capture.ID); err == nil && shellblocks.CaptureReady(capture.ID) {
			break
		}
		if time.Now().After(deadline) {
			return fail(fmt.Errorf("collector or zsh hooks did not become ready"))
		}
		time.Sleep(20 * time.Millisecond)
	}
	keep = true
	pane.CaptureID = capture.ID
	return pane, nil
}
