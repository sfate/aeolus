// Package worktrees creates new isolated checkouts without replacing existing work.
package worktrees

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sfate/aeolus/internal/gitsafe"
)

// Result identifies artifacts retained after creation, including partial failures.
type Result struct {
	Directory, Branch string
	Ready             bool
}

// Git creates a new branch and worktree from the source repository's HEAD.
type Git struct{}

func output(ctx context.Context, directory string, args ...string) (string, error) {
	data, truncated, err := gitsafe.Run(ctx, directory, nil, 64<<10, args...)
	if err != nil {
		return "", err
	}
	if truncated {
		return "", fmt.Errorf("git output exceeds worktree operation limit")
	}
	return strings.TrimSuffix(string(data), "\n"), nil
}

// Create reserves a nonexistent destination, adds a new branch without checkout,
// then materializes files with hooks and configured filters disabled. On failure,
// any created directory, branch, and worktree metadata are retained for inspection.
func (Git) Create(ctx context.Context, source, target, branch string) (Result, error) {
	result := Result{}
	if strings.TrimSpace(source) == "" {
		return result, fmt.Errorf("source pane directory is unavailable")
	}
	if !filepath.IsAbs(target) {
		return result, fmt.Errorf("worktree destination must be an absolute path")
	}
	target = filepath.Clean(target)
	if _, err := os.Lstat(target); err == nil {
		return result, fmt.Errorf("worktree destination already exists; choose a new path")
	} else if !os.IsNotExist(err) {
		return result, fmt.Errorf("inspect destination: %w", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return result, fmt.Errorf("destination parent must exist: %w", err)
	}
	if strings.HasPrefix(branch, "-") || strings.Contains(branch, "@{") || branch == "" {
		return result, fmt.Errorf("enter a literal new branch name without options or checkout shorthand")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	root, err := output(ctx, source, "rev-parse", "--show-toplevel")
	if err != nil {
		return result, fmt.Errorf("source must be a non-bare Git worktree: %w", err)
	}
	commit, err := output(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return result, fmt.Errorf("source repository must have an existing HEAD commit: %w", err)
	}
	literal, err := output(ctx, root, "check-ref-format", "--branch", branch)
	if err != nil || literal != branch {
		return result, fmt.Errorf("invalid literal branch name %q", branch)
	}
	_, _, err = gitsafe.Run(ctx, root, nil, 64<<10, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return result, fmt.Errorf("branch %q already exists; choose a new branch", branch)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		return result, err
	}
	common, err := output(ctx, root, "rev-parse", "--git-common-dir")
	if err != nil {
		return result, err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(root, common)
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return result, err
	}
	actualTarget := filepath.Join(parent, filepath.Base(target))
	relative, err := filepath.Rel(common, actualTarget)
	if err != nil || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))) {
		return result, fmt.Errorf("destination cannot be inside Git metadata")
	}
	if err := os.Mkdir(target, 0700); err != nil {
		return result, fmt.Errorf("reserve new worktree destination: %w", err)
	}
	result = Result{Directory: target, Branch: branch}
	retained := func(err error) (Result, error) {
		return result, fmt.Errorf("worktree creation failed; destination %q and any branch %q are retained for inspection: %w", target, branch, err)
	}
	config, err := gitsafe.FilterOverrides(ctx, root)
	if err != nil {
		return retained(err)
	}
	_, _, err = gitsafe.Run(ctx, root, config, 64<<10, "worktree", "add", "--no-checkout", "-b", branch, "--", target, commit)
	if err != nil {
		return retained(err)
	}
	// Read configuration in the new worktree before checkout so conditional config
	// keyed to its path cannot introduce unchecked smudge/process filters.
	config, err = gitsafe.FilterOverrides(ctx, target)
	if err != nil {
		return retained(err)
	}
	_, _, err = gitsafe.Run(ctx, target, config, 64<<10, "reset", "--hard", "HEAD", "--")
	if err != nil {
		return retained(err)
	}
	result.Ready = true
	return result, nil
}
