package worktrees

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "core.fsmonitor=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	write(t, dir, "tracked.txt", "original\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "initial")
	return dir
}
func write(t *testing.T, dir, name, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCreatePreservesDirtySourceAndStartsFromHead(t *testing.T) {
	source := repo(t)
	write(t, source, "tracked.txt", "staged\n")
	git(t, source, "add", "tracked.txt")
	write(t, source, "tracked.txt", "dirty\n")
	write(t, source, "untracked.txt", "private\n")
	before := git(t, source, "status", "--porcelain=v1")
	target := filepath.Join(t.TempDir(), "new 空 $(command); worktree")
	result, err := (Git{}).Create(context.Background(), source, target, "new-topic")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready || result.Directory != target || result.Branch != "new-topic" {
		t.Fatalf("result %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(target, "tracked.txt"))
	if err != nil || string(content) != "original\n" {
		t.Fatalf("copied dirty edits %q %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(target, "untracked.txt")); !os.IsNotExist(err) {
		t.Fatal("copied untracked file")
	}
	if after := git(t, source, "status", "--porcelain=v1"); before != after {
		t.Fatalf("source changed: %q -> %q", before, after)
	}
	if got := strings.TrimSpace(git(t, target, "branch", "--show-current")); got != "new-topic" {
		t.Fatalf("branch %q", got)
	}
}

func TestCreateRejectsExistingPathsBranchesAndInvalidRefs(t *testing.T) {
	source := repo(t)
	existing := t.TempDir()
	creator := Git{}
	if _, err := creator.Create(context.Background(), source, existing, "new-topic"); err == nil {
		t.Fatal("accepted existing directory")
	}
	symlink := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(existing, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := creator.Create(context.Background(), source, symlink, "new-topic"); err == nil {
		t.Fatal("accepted symlink")
	}
	branch := strings.TrimSpace(git(t, source, "branch", "--show-current"))
	for _, ref := range []string{branch, "-bad", "@{-1}", "bad..name", "refs/heads/../bad", "bad branch"} {
		target := filepath.Join(t.TempDir(), "new")
		if _, err := creator.Create(context.Background(), source, target, ref); err == nil {
			t.Fatalf("accepted ref %q", ref)
		}
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("validation mutated destination for %q", ref)
		}
	}
	if _, err := creator.Create(context.Background(), source, "relative", "topic"); err == nil {
		t.Fatal("accepted relative destination")
	}
}

func TestCreateDisablesHooksAndCheckoutFilters(t *testing.T) {
	source := repo(t)
	write(t, source, ".gitattributes", "tracked.txt filter=evil\n")
	git(t, source, "add", ".")
	git(t, source, "commit", "-m", "attributes")
	marker := filepath.Join(source, "executed")
	hook := filepath.Join(source, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	command := "touch '" + marker + "'; cat"
	git(t, source, "config", "filter.evil.smudge", command)
	git(t, source, "config", "filter.evil.process", command)
	git(t, source, "config", "filter.evil.required", "true")
	git(t, source, "config", "core.fsmonitor", command)
	target := filepath.Join(t.TempDir(), "new")
	result, err := (Git{}).Create(context.Background(), source, target, "safe-topic")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready {
		t.Fatal("worktree not ready")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("repository command executed %v", err)
	}
}

func TestCreateRejectsUnbornRepositoryAndCancellation(t *testing.T) {
	source := t.TempDir()
	git(t, source, "init")
	if _, err := (Git{}).Create(context.Background(), source, filepath.Join(t.TempDir(), "new"), "topic"); err == nil {
		t.Fatal("accepted unborn repository")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Git{}).Create(ctx, repo(t), filepath.Join(t.TempDir(), "new"), "topic"); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestCreateRejectsGitMetadataDestinations(t *testing.T) {
	source := repo(t)
	metadata := filepath.Join(source, ".git")
	alias := filepath.Join(t.TempDir(), "metadata")
	if err := os.Symlink(metadata, alias); err != nil {
		t.Fatal(err)
	}
	for _, parent := range []string{metadata, alias} {
		target := filepath.Join(parent, "new-checkout")
		if _, err := (Git{}).Create(context.Background(), source, target, "topic"); err == nil {
			t.Fatalf("accepted metadata destination %q", target)
		}
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("created rejected destination %q: %v", target, err)
		}
	}
}

func TestCreateDisablesBranchConditionalFilters(t *testing.T) {
	source := repo(t)
	write(t, source, ".gitattributes", "tracked.txt filter=conditional\n")
	git(t, source, "add", ".")
	git(t, source, "commit", "-m", "attributes")
	marker := filepath.Join(t.TempDir(), "executed")
	configuration := filepath.Join(t.TempDir(), "conditional.gitconfig")
	content := "[filter \"conditional\"]\n\tsmudge = touch '" + marker + "'; cat\n\trequired = true\n"
	if err := os.WriteFile(configuration, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, source, "config", "includeIf.onbranch:conditional-topic.path", configuration)
	target := filepath.Join(t.TempDir(), "new")
	result, err := (Git{}).Create(context.Background(), source, target, "conditional-topic")
	if err != nil || !result.Ready {
		t.Fatalf("create: %#v %v", result, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("conditional filter executed: %v", err)
	}
}
