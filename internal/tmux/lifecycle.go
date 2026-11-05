package tmux

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var paneIdentifier = regexp.MustCompile(`^%[0-9]+$`)
var sessionIdentifier = regexp.MustCompile(`^\$[0-9]+$`)

// Lifecycle is optional transport support for launching and renaming sessions.
type Lifecycle interface {
	CreateSession(directory, command string) (Pane, error)
	RenameWindow(paneID, name string) error
}

// LaunchUncertainError means tmux may have created a session; inspect before retrying.
type LaunchUncertainError struct{ Err error }

func (e *LaunchUncertainError) Error() string {
	return e.Err.Error() + "; inspect tmux sessions before retrying"
}
func (e *LaunchUncertainError) Unwrap() error { return e.Err }

// ShellLifecycle launches explicitly managed interactive zsh sessions.
type ShellLifecycle interface {
	CreateShellSession(directory string) (Pane, error)
}

// literalFormatArgument protects values from tmux format expansion and its
// command separator parsing. Shell quoting is unnecessary because argv is used.
func literalFormatArgument(value string) string {
	value = strings.ReplaceAll(value, "#", "##")
	if strings.HasSuffix(value, ";") {
		value = value[:len(value)-1] + `\;`
	}
	return value
}

func lifecycleOutput(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tmux lifecycle command failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// CreateSession starts an agent in a new detached, uniquely named tmux session.
// Multiple command arguments make tmux execute env directly instead of a shell.
func (c Client) CreateSession(directory, command string) (Pane, error) {
	switch command {
	case "claude", "codex", "gemini":
	default:
		return Pane{}, fmt.Errorf("unsupported agent %q: choose claude, codex, or gemini", command)
	}
	return createSession(directory, command, false)
}

func (Client) CreateShellSession(directory string) (Pane, error) {
	return createSession(directory, "zsh", true)
}

func createSession(directory, command string, shell bool) (Pane, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return Pane{}, fmt.Errorf("resolve project directory: %w", err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		return Pane{}, fmt.Errorf("open project directory: %w", err)
	}
	if !info.IsDir() {
		return Pane{}, fmt.Errorf("project path must be a directory")
	}
	executable, err := exec.LookPath(command)
	if err != nil {
		return Pane{}, fmt.Errorf("%s is not available on PATH: %w", command, err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return Pane{}, fmt.Errorf("resolve agent executable: %w", err)
	}
	env, err := exec.LookPath("env")
	if err != nil {
		return Pane{}, fmt.Errorf("env is not available on PATH: %w", err)
	}
	env, err = filepath.Abs(env)
	if err != nil {
		return Pane{}, fmt.Errorf("resolve env executable: %w", err)
	}
	var nonce [6]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return Pane{}, fmt.Errorf("generate session name: %w", err)
	}
	name := fmt.Sprintf("aeolus-%s-%x", command, nonce)
	format := "#{pane_id}\t#{session_id}\t#{session_name}\t#{window_index}"
	args := []string{"new-session", "-d", "-P", "-F", format, "-s", name, "-n", command, "-c", literalFormatArgument(directory), "--", env, executable}
	if shell {
		args = append(args, "-i")
	} else {
		for _, arg := range c.ProviderArgs[command] {
			for _, r := range arg {
				if unicode.IsControl(r) {
					return Pane{}, fmt.Errorf("provider arguments cannot contain control characters")
				}
			}
			if strings.HasSuffix(arg, ";") {
				arg = arg[:len(arg)-1] + `\;`
			}
			args = append(args, arg)
		}
	}
	out, err := lifecycleOutput(args...)
	if err != nil {
		return Pane{}, &LaunchUncertainError{Err: err}
	}
	fields := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(fields) != 4 || !paneIdentifier.MatchString(fields[0]) || !sessionIdentifier.MatchString(fields[1]) {
		return Pane{}, &LaunchUncertainError{Err: fmt.Errorf("session %s was created but tmux returned invalid metadata", name)}
	}
	index, err := strconv.Atoi(fields[3])
	if err != nil {
		return Pane{}, &LaunchUncertainError{Err: fmt.Errorf("session %s was created but tmux returned an invalid window index", name)}
	}
	if shell {
		if _, err := lifecycleOutput("set-option", "-p", "-t", fields[0], "@aeolus-shell", "v1"); err != nil {
			return Pane{}, fmt.Errorf("shell session %s was created but marking it failed; inspect it with tmux list-sessions: %w", name, err)
		}
	}
	return Pane{PaneID: fields[0], SessionID: fields[1], SessionName: fields[2], WindowIndex: index, WindowName: command, CurrentCommand: command, CurrentPath: directory, Shell: shell}, nil
}

// RenameWindow changes the window containing an exact pane ID.
func (Client) RenameWindow(paneID, name string) error {
	if !paneIdentifier.MatchString(paneID) {
		return fmt.Errorf("invalid pane identifier %q", paneID)
	}
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 80 {
		return fmt.Errorf("window name must contain 1 to 80 characters")
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == '|' {
			return fmt.Errorf("window name cannot contain control characters or |")
		}
	}
	_, err := lifecycleOutput("rename-window", "-t", paneID, "--", literalFormatArgument(name))
	return err
}

var _ Lifecycle = Client{}
