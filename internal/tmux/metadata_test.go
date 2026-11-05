package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func paneRecord(fields ...string) string {
	var b strings.Builder
	for _, v := range fields {
		fmt.Fprintf(&b, "%d:%s", len(v), v)
	}
	return b.String() + "\n"
}

func TestManagedShellMetadataMarker(t *testing.T) {
	for _, marker := range []string{"", "v1", "v2", "true"} {
		record := paneRecord("%2", "0", "1", "session", "$1", "zsh", "123", "80", "24", "shell", "/tmp", marker)
		panes, err := parsePanes([]byte(record))
		if err != nil || len(panes) != 1 || panes[0].Shell != (marker == "v1") {
			t.Fatalf("marker %q: %#v %v", marker, panes, err)
		}
	}
}

func TestParsePaneMetadata(t *testing.T) {
	path := "/tmp/雪 project|with\ttab\nand\\slashes# "
	record := paneRecord("%2", "0", "1", "session|雪", "$1", "claude", "123", "80", "24", "window|name", path, "")
	got, err := parsePanes([]byte(record + record))
	if err != nil || len(got) != 2 || got[0].CurrentPath != path || got[0].WindowName != "window|name" || got[0].SessionName != "session|雪" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	for _, bad := range []string{"bad", "999:short", "-1:x", record[:len(record)-2], strings.Replace(record, "3:123", "3:abc", 1)} {
		if _, err := parsePanes([]byte(bad)); err == nil {
			t.Fatalf("accepted malformed record %q", bad)
		}
	}
}

func TestListPanesWithRealTmux(t *testing.T) {
	binary, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	socket := filepath.Join(t.TempDir(), "tmux.sock")
	directory := filepath.Join(t.TempDir(), "雪|with space\ttab\nand#hash ")
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-S", socket, "-f", "/dev/null", "new-session", "-d", "-c", literalFormatArgument(directory), "--", "sleep", "30")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated tmux start: %v %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command(binary, "-S", socket, "kill-server").Run() })
	t.Setenv("TMUX", socket+",0,0")
	// new-session can return before its child has changed directory and executed.
	deadline := time.Now().Add(5 * time.Second)
	for {
		panes, err := ListAllPanes()
		if err != nil {
			t.Fatalf("list panes while waiting for startup: %v", err)
		}
		if len(panes) == 1 && panes[0].CurrentPath == directory {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane startup timed out: want directory %q; last panes=%#v", directory, panes)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
