package gitreview

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
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
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
	write(t, dir, "tracked.txt", "before\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "initial")
	return dir
}
func write(t *testing.T, dir, path, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotShowsPathsAndBothDiffs(t *testing.T) {
	dir := repo(t)
	write(t, dir, "tracked.txt", "staged\n")
	git(t, dir, "add", "tracked.txt")
	write(t, dir, "tracked.txt", "unstaged\n")
	write(t, dir, "new 空 file.txt", "untracked\n")
	snapshot, err := Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 2 {
		t.Fatalf("files %#v", snapshot.Files)
	}
	if !strings.Contains(snapshot.StagedDiff, "+staged") || !strings.Contains(snapshot.UnstagedDiff, "+unstaged") {
		t.Fatalf("diffs: %#v", snapshot)
	}
	found := false
	for _, file := range snapshot.Files {
		if file.Path == "new 空 file.txt" && file.Status == "??" {
			found = true
		}
	}
	if !found {
		t.Fatalf("untracked path lost: %#v", snapshot.Files)
	}
}

func TestStatusParsesRenamesAndControlCharacters(t *testing.T) {
	files, err := parseStatus([]byte("R  new 空\nname\x00old name\x00?? --flag\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "new 空\nname" || files[0].OriginalPath != "old name" || files[1].Path != "--flag" {
		t.Fatalf("got %#v", files)
	}
	if _, err := parseStatus([]byte("R  missing-old\x00")); err == nil {
		t.Fatal("accepted partial rename")
	}
}

func TestSnapshotDoesNotRunRepositoryCommands(t *testing.T) {
	dir := repo(t)
	marker := filepath.Join(dir, "executed")
	command := "touch " + marker + "; cat"
	git(t, dir, "config", "filter.evil.clean", command)
	git(t, dir, "config", "filter.evil.process", command)
	git(t, dir, "config", "filter.evil.required", "true")
	git(t, dir, "config", "diff.evil.command", command)
	git(t, dir, "config", "diff.evil.textconv", command)
	git(t, dir, "config", "core.fsmonitor", command)
	write(t, dir, ".gitattributes", "tracked.txt filter=evil diff=evil\n")
	write(t, dir, "tracked.txt", "after\n")
	snapshot, err := Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot.UnstagedDiff, "+after") {
		t.Fatalf("missing diff: %q", snapshot.UnstagedDiff)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("repository command executed: %v", err)
	}
}

func TestSnapshotBoundsOutputAndIgnoresInheritedGitDirectory(t *testing.T) {
	dir := repo(t)
	other := repo(t)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	write(t, dir, "tracked.txt", strings.Repeat("large line\n", maxOutputBytes))
	snapshot, err := Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Truncated || len(snapshot.UnstagedDiff) > maxOutputBytes {
		t.Fatalf("unbounded output: %d truncated %t", len(snapshot.UnstagedDiff), snapshot.Truncated)
	}
	if snapshot.Directory == other {
		t.Fatal("inherited Git directory redirected review")
	}
}

func TestSnapshotErrorsAndCancellation(t *testing.T) {
	if _, err := Read(context.Background(), ""); err == nil {
		t.Fatal("accepted empty directory")
	}
	if _, err := Read(context.Background(), t.TempDir()); err == nil {
		t.Fatal("accepted non-repository")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Read(ctx, repo(t)); err == nil {
		t.Fatal("ignored cancellation")
	}
}
