// Package gitreview reads bounded repository snapshots without changing Git state.
package gitreview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const maxOutputBytes = 1 << 20
const maxStatusBytes = 256 << 10

// File contains a NUL-delimited porcelain status record and unescaped paths.
type File struct{ Status, Path, OriginalPath string }

// Snapshot is an aggregate review, not an atomic filesystem transaction.
type Snapshot struct {
	Directory                string
	Files                    []File
	StagedDiff, UnstagedDiff string
	Truncated                bool
}

type boundedOutput struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	size := len(p)
	remaining := b.limit - b.buffer.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, err := b.buffer.Write(p)
	return size, err
}

func environment() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1", "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat", "GIT_ATTR_NOSYSTEM=1")
}

func run(ctx context.Context, directory string, config []string, limit int, args ...string) ([]byte, bool, error) {
	prefix := []string{"--no-pager", "--no-optional-locks", "-C", directory, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "core.untrackedCache=false", "-c", "color.ui=false", "-c", "protocol.allow=never"}
	prefix = append(prefix, config...)
	cmd := exec.CommandContext(ctx, "git", append(prefix, args...)...)
	cmd.Env = environment()
	output := &boundedOutput{limit: limit}
	stderr := &boundedOutput{limit: 8 << 10}
	cmd.Stdout = output
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return nil, false, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.buffer.String()))
	}
	return output.buffer.Bytes(), output.truncated, nil
}

var filterKey = regexp.MustCompile(`^filter\.[A-Za-z0-9_.-]+\.(clean|smudge|process|required)$`)

// filterOverrides prevents status and diff from invoking configured clean filters.
func filterOverrides(ctx context.Context, directory string) ([]string, error) {
	data, truncated, err := run(ctx, directory, nil, maxStatusBytes, "config", "--null", "--name-only", "--get-regexp", `^filter\..*\.(clean|smudge|process|required)$`)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	if truncated {
		return nil, fmt.Errorf("filter configuration exceeds review limit")
	}
	var config []string
	for _, key := range strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00") {
		if key == "" {
			continue
		}
		if !filterKey.MatchString(key) {
			return nil, fmt.Errorf("unsupported filter configuration key; review disabled rather than executing it")
		}
		value := ""
		if strings.HasSuffix(key, ".required") {
			value = "false"
		}
		config = append(config, "-c", key+"="+value)
	}
	return config, nil
}

// Read inspects the repository containing directory. Output and total runtime are
// bounded. Untracked file paths are listed; their contents are not read as diffs.
func Read(ctx context.Context, directory string) (Snapshot, error) {
	if strings.TrimSpace(directory) == "" {
		return Snapshot{}, fmt.Errorf("pane working directory is unavailable; refresh the dashboard and try again")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	root, truncated, err := run(ctx, directory, nil, 32<<10, "rev-parse", "--show-toplevel")
	if err != nil {
		return Snapshot{}, fmt.Errorf("cannot review pane directory: %w", err)
	}
	if truncated {
		return Snapshot{}, fmt.Errorf("repository path exceeds review limit")
	}
	directory = strings.TrimSuffix(string(root), "\n")
	config, err := filterOverrides(ctx, directory)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Directory: directory}
	status, short, err := run(ctx, directory, config, maxStatusBytes, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=all", "--renames")
	if err != nil {
		return Snapshot{}, err
	}
	if short {
		return Snapshot{}, fmt.Errorf("changed-path list exceeds 256 KiB; narrow the repository before reviewing")
	}
	snapshot.Files, err = parseStatus(status)
	if err != nil {
		return Snapshot{}, err
	}
	diffArgs := []string{"diff", "--no-ext-diff", "--no-textconv", "--color=never", "--ignore-submodules=all", "--no-renames", "--no-relative"}
	data, short, err := run(ctx, directory, config, maxOutputBytes, append(diffArgs, "--cached", "--")...)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.StagedDiff = string(data)
	snapshot.Truncated = short
	data, short, err = run(ctx, directory, config, maxOutputBytes, append(diffArgs, "--")...)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.UnstagedDiff = string(data)
	snapshot.Truncated = snapshot.Truncated || short
	return snapshot, nil
}

func parseStatus(data []byte) ([]File, error) {
	var files []File
	for len(data) > 0 {
		end := bytes.IndexByte(data, 0)
		if end < 3 || data[2] != ' ' {
			return nil, fmt.Errorf("git returned an incomplete status record")
		}
		record := data[:end]
		data = data[end+1:]
		file := File{Status: string(record[:2]), Path: string(record[3:])}
		if strings.ContainsAny(file.Status, "RC") {
			end = bytes.IndexByte(data, 0)
			if end < 0 {
				return nil, fmt.Errorf("git returned an incomplete rename record")
			}
			file.OriginalPath = string(data[:end])
			data = data[end+1:]
		}
		files = append(files, file)
	}
	return files, nil
}
