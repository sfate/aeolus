package shellblocks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestZshHooksWithRealTmux(t *testing.T) {
	paths := map[string]string{}
	for _, name := range []string{"tmux", "zsh", "base64", "tr"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Skip(name + " unavailable")
		}
		paths[name] = path
	}
	dir, err := os.MkdirTemp("/tmp", "aeolus-events-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")
	run := func(args ...string) []byte {
		t.Helper()
		out, err := exec.Command(paths["tmux"], append([]string{"-S", socket}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v %s", args, err, out)
		}
		return out
	}
	run("-f", "/dev/null", "new-session", "-d", "-s", "test", "--", paths["zsh"], "-f", "-i")
	t.Cleanup(func() { _ = exec.Command(paths["tmux"], "-S", socket, "kill-server").Run() })
	stream := filepath.Join(dir, "stream")
	run("pipe-pane", "-O", "-t", "test:0.0", "cat > "+shellQuote(stream))
	hooks, err := ZshHooks(testToken, paths["base64"], paths["tr"])
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "hooks.zsh")
	if err := os.WriteFile(source, []byte(hooks), 0600); err != nil {
		t.Fatal(err)
	}
	send := func(command string) {
		t.Helper()
		run("send-keys", "-t", "test:0.0", "-l", "--", command)
		run("send-keys", "-t", "test:0.0", "Enter")
	}
	send("_old_preexec() { builtin printf 'OLD_PREEXEC\\n'; }; _old_precmd() { builtin printf 'OLD_PRECMD\\n'; }; preexec_functions=(_old_preexec); precmd_functions=(_old_precmd); source " + shellQuote(source))
	// Wait for setup so zsh can consume the next commands through normal line editing.
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, _ := os.ReadFile(stream)
		if strings.Contains(string(data), "OLD_PRECMD\r\n") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("hooks did not load: %q", data)
		}
		time.Sleep(10 * time.Millisecond)
	}
	send("false")
	send(`printf 'EXIT_%s\n' $?`)
	send(`[[ -t 0 && -t 1 ]] && printf 'TTY_%s\n' yes`)
	for {
		data, _ := os.ReadFile(stream)
		p, _ := NewParser(testToken)
		_, _ = p.Write(data)
		blocks := p.Snapshot()
		if len(blocks) >= 3 && blocks[2].ExitCode != nil {
			if blocks[0].Command != "false" || *blocks[0].ExitCode != 1 || !strings.Contains(string(blocks[1].Output), "EXIT_1") || !strings.Contains(string(blocks[2].Output), "TTY_yes") || !strings.Contains(string(data), "OLD_PREEXEC\r\n") {
				t.Fatalf("hook semantics changed: %#v stream=%q", blocks, data)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("explicit blocks not completed: %#v stream=%q", blocks, data)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHookValidation(t *testing.T) {
	if _, err := ZshHooks("bad", "/usr/bin/base64", "/usr/bin/tr"); err == nil {
		t.Fatal("bad token accepted")
	}
	if _, err := ZshHooks(testToken, "base64", "/usr/bin/tr"); err == nil {
		t.Fatal("relative tool accepted")
	}
}
