package tmux

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/sfate/aeolus/internal/layouts"
)

// LayoutLifecycle creates a complete local layout while preserving any partial result.
type LayoutLifecycle interface {
	CreateLayout(layouts.Layout) (LayoutResult, error)
}

type LayoutResult struct {
	SessionName string
	SessionID   string
	Panes       []Pane
}

// LayoutLaunchError identifies the location where creation stopped. Created panes
// remain available in LayoutResult and are never removed automatically.
type LayoutLaunchError struct {
	Window    string
	Pane      int
	Err       error
	Uncertain bool
}

func (e *LayoutLaunchError) Error() string {
	location := fmt.Sprintf("window %q", e.Window)
	if e.Pane > 0 {
		location += fmt.Sprintf(" pane %d", e.Pane)
	}
	if e.Uncertain {
		return fmt.Sprintf("layout creation at %s may have partially succeeded; inspect tmux before retrying: %v", location, e.Err)
	}
	return fmt.Sprintf("layout creation failed at %s: %v", location, e.Err)
}
func (e *LayoutLaunchError) Unwrap() error { return e.Err }

type layoutPaneCommand struct {
	command string
	args    []string
}

func preflightLayout(layout layouts.Layout, providerArgs map[string][]string) ([][]layoutPaneCommand, string, error) {
	if err := layouts.Validate(layout); err != nil {
		return nil, "", err
	}
	info, err := os.Stat(layout.Directory)
	if err != nil {
		return nil, "", fmt.Errorf("project directory %q: %w", layout.Directory, err)
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("project path %q is not a directory", layout.Directory)
	}
	env, err := exec.LookPath("env")
	if err != nil {
		return nil, "", fmt.Errorf("env executable is unavailable: %w", err)
	}
	env, err = filepath.Abs(env)
	if err != nil {
		return nil, "", fmt.Errorf("resolve env executable: %w", err)
	}
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return nil, "", fmt.Errorf("tmux executable is unavailable: %w", err)
	}
	tmuxPath, err = filepath.Abs(tmuxPath)
	if err != nil {
		return nil, "", fmt.Errorf("resolve tmux executable: %w", err)
	}
	commands := make([][]layoutPaneCommand, len(layout.Windows))
	resolved := map[string]string{}
	for wi, window := range layout.Windows {
		commands[wi] = make([]layoutPaneCommand, len(window.Panes))
		for pi, pane := range window.Panes {
			command := "zsh"
			var args []string
			if pane.Type == layouts.Agent {
				command = pane.Provider
				args = append([]string(nil), providerArgs[command]...)
				if len(args) > 64 {
					return nil, "", fmt.Errorf("provider_args.%s exceeds 64 arguments", command)
				}
				total := 0
				for _, arg := range args {
					if !utf8.ValidString(arg) {
						return nil, "", fmt.Errorf("provider_args.%s contains invalid UTF-8", command)
					}
					if len(arg) > 4096 {
						return nil, "", fmt.Errorf("provider_args.%s contains an argument longer than 4096 bytes", command)
					}
					for _, r := range arg {
						if unicode.IsControl(r) {
							return nil, "", fmt.Errorf("provider_args.%s contains a control character", command)
						}
					}
					total += len(arg)
				}
				if total > 16384 {
					return nil, "", fmt.Errorf("provider_args.%s exceeds 16 KiB", command)
				}
			}
			path, ok := resolved[command]
			if !ok {
				path, err = exec.LookPath(command)
				if err != nil {
					return nil, "", fmt.Errorf("%s executable is unavailable: %w", command, err)
				}
				path, err = filepath.Abs(path)
				if err != nil {
					return nil, "", fmt.Errorf("resolve %s executable: %w", command, err)
				}
				resolved[command] = path
			}
			if pane.Type == layouts.Shell {
				args = []string{"-i"}
			}
			commands[wi][pi] = layoutPaneCommand{command: path, args: append([]string{env, path}, args...)}
		}
	}
	return commands, tmuxPath, nil
}

func (c Client) CreateLayout(layout layouts.Layout) (LayoutResult, error) {
	commands, tmuxPath, err := preflightLayout(layout, c.ProviderArgs)
	if err != nil {
		return LayoutResult{}, err
	}
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return LayoutResult{}, fmt.Errorf("generate layout session name: %w", err)
	}
	result := LayoutResult{SessionName: fmt.Sprintf("aeolus-layout-%x", nonce)}
	format := "#{pane_id}\t#{session_id}\t#{session_name}\t#{window_index}\t#{pane_index}"
	for wi, window := range layout.Windows {
		for pi, pane := range window.Panes {
			command := commands[wi][pi]
			args := []string{}
			if wi == 0 && pi == 0 {
				args = append(args, "new-session", "-d", "-P", "-F", format, "-s", result.SessionName, "-n", literalFormatArgument(window.Name), "-c", literalFormatArgument(layout.Directory))
			} else if pi == 0 {
				args = append(args, "new-window", "-d", "-P", "-F", format, "-t", result.SessionID, "-n", literalFormatArgument(window.Name), "-c", literalFormatArgument(layout.Directory))
			} else {
				previous := result.Panes[len(result.Panes)-1]
				splitFlag := "-h"
				if pane.Split == layouts.Vertical {
					splitFlag = "-v"
				}
				args = append(args, "split-window", "-d", "-P", "-F", format, "-t", previous.PaneID, splitFlag)
				if pane.SizePercent > 0 {
					args = append(args, "-p", strconv.Itoa(pane.SizePercent))
				}
				args = append(args, "-c", literalFormatArgument(layout.Directory))
			}
			args = append(args, "--")
			args = append(args, command.args...)
			for i := len(args) - len(command.args); i < len(args); i++ {
				args[i] = literalProviderArgument(args[i])
			}
			out, runErr := lifecycleOutputAt(tmuxPath, args...)
			if runErr != nil {
				return result, &LayoutLaunchError{Window: window.Name, Pane: pi + 1, Err: runErr, Uncertain: true}
			}
			created, parseErr := parseLayoutPane(out)
			if parseErr != nil {
				return result, &LayoutLaunchError{Window: window.Name, Pane: pi + 1, Err: parseErr, Uncertain: true}
			}
			created.WindowName = window.Name
			created.CurrentPath = layout.Directory
			created.CurrentCommand = command.command
			created.Shell = pane.Type == layouts.Shell
			if len(result.Panes) == 0 {
				result.SessionID = created.SessionID
				result.SessionName = created.SessionName
			}
			result.Panes = append(result.Panes, created)
			if created.Shell {
				if _, markErr := lifecycleOutputAt(tmuxPath, "set-option", "-p", "-t", created.PaneID, "@aeolus-shell", "v1"); markErr != nil {
					return result, &LayoutLaunchError{Window: window.Name, Pane: pi + 1, Err: fmt.Errorf("mark created shell pane: %w", markErr), Uncertain: true}
				}
			}
		}
	}
	return result, nil
}

func literalProviderArgument(arg string) string {
	arg = strings.ReplaceAll(arg, "#", "##")
	if strings.HasSuffix(arg, ";") {
		arg = arg[:len(arg)-1] + `\;`
	}
	return arg
}

func parseLayoutPane(output []byte) (Pane, error) {
	fields := strings.Split(strings.TrimSpace(string(output)), "\t")
	if len(fields) != 5 || !paneIdentifier.MatchString(fields[0]) || !sessionIdentifier.MatchString(fields[1]) || fields[2] == "" {
		return Pane{}, fmt.Errorf("tmux returned invalid pane metadata")
	}
	windowIndex, err := strconv.Atoi(fields[3])
	if err != nil || windowIndex < 0 {
		return Pane{}, fmt.Errorf("tmux returned an invalid window index")
	}
	paneIndex, err := strconv.Atoi(fields[4])
	if err != nil || paneIndex < 0 {
		return Pane{}, fmt.Errorf("tmux returned an invalid pane index")
	}
	return Pane{PaneID: fields[0], SessionID: fields[1], SessionName: fields[2], WindowIndex: windowIndex, PaneIndex: paneIndex}, nil
}

func lifecycleOutputAt(executable string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, executable, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tmux lifecycle command failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

var _ LayoutLifecycle = Client{}
