// Package gitreview reads bounded repository snapshots without changing Git state.
package gitreview

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sfate/aeolus/internal/gitsafe"
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

// Read inspects the repository containing directory. Output and total runtime are
// bounded. Untracked file paths are listed; their contents are not read as diffs.
func Read(ctx context.Context, directory string) (Snapshot, error) {
	if strings.TrimSpace(directory) == "" {
		return Snapshot{}, fmt.Errorf("pane working directory is unavailable; refresh the dashboard and try again")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	root, truncated, err := gitsafe.Run(ctx, directory, nil, 32<<10, "rev-parse", "--show-toplevel")
	if err != nil {
		return Snapshot{}, fmt.Errorf("cannot review pane directory: %w", err)
	}
	if truncated {
		return Snapshot{}, fmt.Errorf("repository path exceeds review limit")
	}
	directory = strings.TrimSuffix(string(root), "\n")
	config, err := gitsafe.FilterOverrides(ctx, directory)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Directory: directory}
	status, short, err := gitsafe.Run(ctx, directory, config, maxStatusBytes, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=all", "--renames")
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
	data, short, err := gitsafe.Run(ctx, directory, config, maxOutputBytes, append(diffArgs, "--cached", "--")...)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.StagedDiff = string(data)
	snapshot.Truncated = short
	data, short, err = gitsafe.Run(ctx, directory, config, maxOutputBytes, append(diffArgs, "--")...)
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
