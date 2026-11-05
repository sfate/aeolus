package tmux

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fakeLifecycleTmux(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\000' \"$@\" > \"$AEOLUS_TMUX_ARG_LOG\"\nprintf '%s\\n' '%7\t$4\taeolus-test\t2'\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEOLUS_TMUX_ARG_LOG", log)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir, log
}
func recordedArgs(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
}

func TestCreateSessionUsesLiteralArguments(t *testing.T) {
	bin, log := fakeLifecycleTmux(t)
	directory := filepath.Join(t.TempDir(), "project #{session_name} $(touch owned);")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	pane, err := (Client{}).CreateSession(directory, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if pane.PaneID != "%7" || pane.SessionID != "$4" || pane.WindowIndex != 2 || pane.CurrentCommand != "claude" {
		t.Fatalf("metadata %#v", pane)
	}
	args := recordedArgs(t, log)
	if args[0] != "new-session" || args[1] != "-d" {
		t.Fatalf("unexpected args %#v", args)
	}
	var cwd string
	for i, arg := range args {
		if arg == "-c" {
			cwd = args[i+1]
		}
	}
	if cwd != strings.ReplaceAll(directory[:len(directory)-1], "#", "##")+`\;` {
		t.Fatalf("unsafe directory argument %q", cwd)
	}
	if args[len(args)-1] != filepath.Join(bin, "claude") || filepath.Base(args[len(args)-2]) != "env" || args[len(args)-3] != "--" {
		t.Fatalf("agent must use direct argv execution: %#v", args)
	}
}

func TestLifecycleValidation(t *testing.T) {
	_, log := fakeLifecycleTmux(t)
	client := Client{}
	for _, command := range []string{"claude; touch owned", "bash", "codex --dangerously-bypass-approvals-and-sandbox"} {
		if _, err := client.CreateSession(t.TempDir(), command); err == nil {
			t.Fatalf("accepted command %q", command)
		}
	}
	if _, err := client.CreateSession(filepath.Join(t.TempDir(), "missing"), "claude"); err == nil {
		t.Fatal("accepted missing directory")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateSession(file, "claude"); err == nil {
		t.Fatal("accepted file as directory")
	}
	for _, name := range []string{"", "  ", "bad\nname", "bad|name", strings.Repeat("a", 81)} {
		if err := client.RenameWindow("%7", name); err == nil {
			t.Fatalf("accepted name %q", name)
		}
	}
	if err := client.RenameWindow("session:window", "valid"); err == nil {
		t.Fatal("accepted non-pane target")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("invalid input reached tmux: %v", err)
	}
}

func TestRenameWindowUsesLiteralArguments(t *testing.T) {
	_, log := fakeLifecycleTmux(t)
	if err := (Client{}).RenameWindow("%7", "review #{session_name} $(x);"); err != nil {
		t.Fatal(err)
	}
	want := []string{"rename-window", "-t", "%7", "--", `review ##{session_name} $(x)\;`}
	if got := recordedArgs(t, log); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
