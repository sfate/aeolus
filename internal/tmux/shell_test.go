package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateShellSessionWithRealTmux(t *testing.T) {
	binary, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh unavailable")
	}
	socketDir, err := os.MkdirTemp("/tmp", "aeolus-shell-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socket := filepath.Join(socketDir, "tmux.sock")
	t.Setenv("ZDOTDIR", t.TempDir())
	if out, err := exec.Command(binary, "-S", socket, "-f", "/dev/null", "new-session", "-d", "--", "sleep", "30").CombinedOutput(); err != nil {
		t.Fatalf("isolated server: %v %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command(binary, "-S", socket, "kill-server").Run() })
	t.Setenv("TMUX", socket+",0,0")
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pane, err := (Client{}).CreateShellSession(directory)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	ready := false
	for {
		panes, err := ListAllPanes()
		if err != nil {
			t.Fatal(err)
		}
		for _, got := range panes {
			if got.PaneID == pane.PaneID && got.Shell && got.CurrentPath == directory && got.CurrentCommand == "zsh" {
				ready = true
			}
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("managed shell not discovered: %#v", panes)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := (Client{}).SendText(pane.PaneID, `[[ -t 0 && -t 1 ]] && printf 'AEOLUS_%s\n' TTY_yes`); err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(deadline) {
		out, err := CapturePane(pane.PaneID, 50)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "AEOLUS_TTY_yes") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	out, _ := CapturePane(pane.PaneID, 50)
	t.Fatalf("interactive shell did not execute command with terminal stdin/stdout: %q", out)
}
