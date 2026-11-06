package tmux

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sfate/aeolus/internal/layouts"
)

func fakeLayoutTmux(t *testing.T) (string, string) {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	count := filepath.Join(bin, "count")
	script := `#!/bin/sh
printf '%s\000' "$@" >> "$AEOLUS_LAYOUT_LOG"
printf '\n' >> "$AEOLUS_LAYOUT_LOG"
case "$1" in
  new-session|new-window|split-window)
    if [ "$AEOLUS_FAIL_COMMAND" = "$1" ]; then echo synthetic-failure >&2; exit 1; fi
    n=0; [ -f "$AEOLUS_LAYOUT_COUNT" ] && n=$(/bin/cat "$AEOLUS_LAYOUT_COUNT"); n=$((n + 1)); printf '%s' "$n" > "$AEOLUS_LAYOUT_COUNT"
    wi=0; pi=0; [ "$1" = "new-window" ] && wi=1; [ "$1" = "split-window" ] && pi=1
    printf '%%%s\t$1\taeolus-test\t%s\t%s\n' "$n" "$wi" "$pi"
    ;;
  set-option) ;;
  *) echo "unexpected tmux command $1" >&2; exit 2 ;;
esac
`
	envPath, err := exec.LookPath("env")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(envPath, filepath.Join(bin, "env")); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"tmux": script, "codex": "#!/bin/sh\nexit 0\n", "zsh": "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("AEOLUS_LAYOUT_LOG", log)
	t.Setenv("AEOLUS_LAYOUT_COUNT", count)
	t.Setenv("PATH", bin)
	return bin, log
}

func testLayout(directory string) layouts.Layout {
	return layouts.Layout{Name: "Review", Directory: directory, Windows: []layouts.Window{
		{Name: "Code", Panes: []layouts.Pane{{Type: layouts.Agent, Provider: "codex"}, {Type: layouts.Shell, Split: layouts.Vertical, SizePercent: 35}}},
		{Name: "Notes", Panes: []layouts.Pane{{Type: layouts.Shell}}},
	}}
}

func calls(t *testing.T, log string) [][]string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	out := make([][]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.Split(strings.TrimSuffix(line, "\x00"), "\x00"))
	}
	return out
}

func TestCreateLayoutPreflightsAndBuildsWindowsAndPanes(t *testing.T) {
	bin, log := fakeLayoutTmux(t)
	directory := filepath.Join(t.TempDir(), "project #{session_name} $(touch owned);")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	client := Client{ProviderArgs: map[string][]string{"codex": {"--profile", "two words", "$(touch owned)", "Enter"}}}
	result, err := client.CreateLayout(testLayout(directory))
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "$1" || len(result.Panes) != 3 || result.Panes[0].WindowName != "Code" || result.Panes[1].PaneIndex != 1 || result.Panes[2].WindowName != "Notes" || !result.Panes[1].Shell {
		t.Fatalf("layout result %#v", result)
	}
	got := calls(t, log)
	if len(got) != 5 {
		t.Fatalf("expected three pane creations and two shell markers, got %#v", got)
	}
	if got[0][0] != "new-session" || got[1][0] != "split-window" || got[2][0] != "set-option" || got[3][0] != "new-window" || got[4][0] != "set-option" {
		t.Fatalf("unexpected tmux operation sequence: %#v", got)
	}
	if !reflect.DeepEqual(got[0][len(got[0])-5:], []string{filepath.Join(bin, "codex"), "--profile", "two words", "$(touch owned)", "Enter"}) {
		t.Fatalf("agent args missing or command unsafe: %#v", got[0])
	}
	if !containsArg(got[1], "-v") || !containsArg(got[1], "-p") || !containsArg(got[1], "35") {
		t.Fatalf("split shape or size missing: %#v", got[1])
	}
	if !containsArg(got[3], "Notes") {
		t.Fatalf("second window name missing: %#v", got[3])
	}
	if !containsArg(got[0], strings.ReplaceAll(directory[:len(directory)-1], "#", "##")+`\;`) {
		t.Fatalf("directory was not passed literally: %#v", got[0])
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestCreateLayoutPreflightFailureRunsNoTmuxCommands(t *testing.T) {
	bin, log := fakeLayoutTmux(t)
	layout := testLayout(t.TempDir())
	if err := os.Remove(filepath.Join(bin, "codex")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Client{}).CreateLayout(layout); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("provider preflight error %v", err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("tmux ran before preflight completed: %v", err)
	}
	layout = testLayout(t.TempDir())
	_, log = fakeLayoutTmux(t)
	client := Client{ProviderArgs: map[string][]string{"codex": {"bad\x00argument"}}}
	if _, err := client.CreateLayout(layout); err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("invalid argument preflight error %v", err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("tmux ran before argument validation: %v", err)
	}
	layout = testLayout(filepath.Join(t.TempDir(), "missing"))
	if _, err := (Client{}).CreateLayout(layout); err == nil {
		t.Fatal("missing directory passed preflight")
	}
}

func TestCreateLayoutReportsPartialResultsAndDoesNotRollback(t *testing.T) {
	_, log := fakeLayoutTmux(t)
	t.Setenv("AEOLUS_FAIL_COMMAND", "split-window")
	result, err := (Client{}).CreateLayout(testLayout(t.TempDir()))
	var launchErr *LayoutLaunchError
	if !errors.As(err, &launchErr) || launchErr.Window != "Code" || launchErr.Pane != 2 || len(result.Panes) != 1 {
		t.Fatalf("partial creation not reported: result=%#v err=%v", result, err)
	}
	for _, call := range calls(t, log) {
		if call[0] == "kill-session" || call[0] == "kill-pane" {
			t.Fatalf("partial layout was rolled back: %#v", call)
		}
	}
}

func TestCreateLayoutWithRealTmux(t *testing.T) {
	real, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux unavailable")
	}
	bin := t.TempDir()
	socketDir, err := os.MkdirTemp("/tmp", "aeolus-layout-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socket := filepath.Join(socketDir, "s")
	t.Setenv("AEOLUS_REAL_TMUX", real)
	t.Setenv("AEOLUS_TEST_SOCKET", socket)
	wrapper := "#!/bin/sh\nexec \"$AEOLUS_REAL_TMUX\" -S \"$AEOLUS_TEST_SOCKET\" -f /dev/null \"$@\"\n"
	for name, script := range map[string]string{"tmux": wrapper, "codex": "#!/bin/sh\nexec /bin/sleep 30\n", "zsh": "#!/bin/sh\nexec /bin/sleep 30\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	envPath, err := exec.LookPath("env")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(envPath, filepath.Join(bin, "env")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Cleanup(func() { _ = exec.Command(real, "-S", socket, "kill-server").Run() })
	result, err := (Client{}).CreateLayout(testLayout(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Panes) != 3 {
		t.Fatalf("created panes %#v", result.Panes)
	}
	windows, err := exec.Command(real, "-S", socket, "-f", "/dev/null", "list-windows", "-t", result.SessionID, "-F", "#{window_name}").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSpace(string(windows)), "\n"); len(got) != 2 || got[0] != "Code" || got[1] != "Notes" {
		t.Fatalf("windows %q", windows)
	}
	panes, err := exec.Command(real, "-S", socket, "-f", "/dev/null", "list-panes", "-a", "-t", result.SessionID, "-F", "#{pane_id}\t#{@aeolus-shell}").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSpace(string(panes)), "\n"); len(got) != 3 || !strings.Contains(got[1], "v1") || !strings.Contains(got[2], "v1") {
		t.Fatalf("pane markers %q", panes)
	}
}
