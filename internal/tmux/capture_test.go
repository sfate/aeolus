package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sfate/aeolus/internal/shellblocks"
)

func TestCapturedShellWithBuiltHelper(t *testing.T) {
	for _, name := range []string{"tmux", "zsh", "base64", "tr"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " unavailable")
		}
	}
	dir, err := os.MkdirTemp("/tmp", "aeolus-capture-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	binary := filepath.Join(dir, "aeolus")
	if out, err := exec.Command("go", "build", "-o", binary, "../..").CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v %s", err, out)
	}
	user := t.TempDir()
	userRC := t.TempDir()
	envText := "sleep 0.1\nexport AEOLUS_ENV_OK=env\nexport ZDOTDIR=" + quoteShellArgument(userRC) + "\n"
	rcText := "export AEOLUS_RC_OK=rc\nbuiltin printf 'STARTUP_READY\\n'\n"
	if err := os.WriteFile(filepath.Join(user, ".zshenv"), []byte(envText), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userRC, ".zshrc"), []byte(rcText), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZDOTDIR", user)
	socket := filepath.Join(dir, "s")
	if out, err := exec.Command("tmux", "-S", socket, "-f", "/dev/null", "new-session", "-d", "--", "sleep", "30").CombinedOutput(); err != nil {
		t.Fatalf("start isolated server: %v %s", err, out)
	}
	project := t.TempDir()
	var captureIDs []string
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socket, "kill-server").Run()
		for _, id := range captureIDs {
			_ = shellblocks.DiscardCapture(id)
		}
	})
	t.Setenv("TMUX", socket+",0,0")
	pane, err := createCapturedShellSession(project, binary)
	if err != nil {
		t.Fatal(err)
	}
	captureIDs = append(captureIDs, pane.CaptureID)
	if !pane.Shell || !shellblocks.IsValidCaptureID(pane.CaptureID) {
		t.Fatalf("capture metadata: %#v", pane)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		out, err := CapturePane(pane.PaneID, 50)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "STARTUP_READY") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("startup was not preserved: %q", out)
		}
		time.Sleep(20 * time.Millisecond)
	}
	command := `[[ $AEOLUS_ENV_OK == env && $AEOLUS_RC_OK == rc && -t 0 && -t 1 ]] && printf 'STARTUP_%s\n' ok; false`
	if err := (Client{}).SendText(pane.PaneID, command); err != nil {
		t.Fatal(err)
	}
	for {
		blocks, err := shellblocks.ReadCapture(pane.CaptureID)
		if err == nil && len(blocks) > 0 && blocks[0].ExitCode != nil {
			if blocks[0].Command != command || *blocks[0].ExitCode != 1 || !strings.Contains(string(blocks[0].Output), "STARTUP_ok") {
				t.Fatalf("captured block: %#v", blocks[0])
			}
			break
		}
		if time.Now().After(deadline) {
			out, _ := CapturePane(pane.PaneID, 50)
			t.Fatalf("block missing: %#v %v output=%q", blocks, err, out)
		}
		time.Sleep(20 * time.Millisecond)
	}
	for path, want := range map[string]string{filepath.Join(user, ".zshenv"): envText, filepath.Join(userRC, ".zshrc"): rcText} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatal("user startup file changed")
		}
	}
	panes, err := ListAllPanes()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, got := range panes {
		if got.PaneID == pane.PaneID && got.CaptureID == pane.CaptureID {
			found = true
		}
	}
	if !found {
		t.Fatal("capture ID missing from metadata")
	}
	existingStream := filepath.Join(dir, "existing-stream")
	hook := "pipe-pane -O " + quoteShellArgument("cat > "+quoteShellArgument(existingStream))
	if out, err := exec.Command("tmux", "-S", socket, "set-hook", "-g", "after-new-session", hook).CombinedOutput(); err != nil {
		t.Fatalf("existing pipe hook: %v %s", err, out)
	}
	if _, err := createCapturedShellSession(project, binary); err == nil || !strings.Contains(err.Error(), "already has an output pipe") {
		t.Fatalf("existing pipe was replaced: %v", err)
	} else {
		_, remainder, ok := strings.Cut(err.Error(), "private bootstrap ")
		retained, _, okEnd := strings.Cut(remainder, " is retained")
		if !ok || !okEnd {
			t.Fatalf("retained bootstrap not identified: %v", err)
		}
		captureIDs = append(captureIDs, strings.TrimPrefix(filepath.Base(retained), "aeolus-blocks-"))
		if info, err := os.Stat(retained); err != nil || !info.IsDir() {
			t.Fatal("live bootstrap removed after partial failure")
		}
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		data, _ := os.ReadFile(existingStream)
		if strings.Contains(string(data), "STARTUP_READY") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("partial capture failure interrupted startup or existing pipe: %q", data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := KillPane(pane.PaneID); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := shellblocks.ReadCapture(pane.CaptureID); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("capture runtime not cleaned after shell exit")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
