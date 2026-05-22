package monitor

import (
	"testing"
)

// ---- tailLines ---------------------------------------------------------------

func TestTailLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{"fewer lines than n", "a\nb\nc", 10, "a\nb\nc"},
		{"exact n", "a\nb\nc", 3, "a\nb\nc"},
		{"more than n", "a\nb\nc\nd\ne", 3, "c\nd\ne"},
		{"trailing newline stripped", "a\nb\nc\n", 2, "b\nc"},
		{"n=1", "a\nb\nc", 1, "c"},
		{"empty string", "", 5, ""},
		{"single line", "hello", 3, "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tailLines(tt.input, tt.n); got != tt.want {
				t.Errorf("tailLines(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

// ---- isClaudeProcess ---------------------------------------------------------

func TestIsClaudeProcess(t *testing.T) {
	yes := []string{
		"claude",
		"claude --version",
		"/usr/local/bin/claude",
		"/home/user/.npm/bin/claude-code",
		"node /usr/local/lib/node_modules/@anthropic-ai/claude-code/cli.js",
		"/opt/homebrew/bin/claude arg1 arg2",
		"CLAUDE",        // case-insensitive
		"/path/CLAUDE",  // case-insensitive path
	}
	for _, args := range yes {
		t.Run("match:"+args, func(t *testing.T) {
			if !isClaudeProcess(args) {
				t.Errorf("isClaudeProcess(%q) = false, want true", args)
			}
		})
	}

	no := []string{
		"bash",
		"zsh",
		"node server.js",
		"vim",
		"git commit -m 'add claudefile'", // contains "claud" but not as a binary
		"python main.py",
		"",
	}
	for _, args := range no {
		t.Run("no-match:"+args, func(t *testing.T) {
			if isClaudeProcess(args) {
				t.Errorf("isClaudeProcess(%q) = true, want false", args)
			}
		})
	}
}

// ---- hasPermissionPatterns ---------------------------------------------------

// realPermissionBox simulates what Claude Code prints when asking for permission.
const realPermissionBox = `
╭─────────────────────────────────────────────────────────────────────╮
│ Claude wants to use Bash                                            │
│                                                                     │
│ $ rm -rf /tmp/build                                                 │
│                                                                     │
│ Allow? (y/n)                                                        │
╰─────────────────────────────────────────────────────────────────────╯
`

const allowOnceUI = `
╭──────────────────────────────────────╮
│ Bash                                 │
│ ls -la                               │
╰──────────────────────────────────────╯
❯ Allow once
  Allow for this session
  Deny
`

func TestHasPermissionPatterns(t *testing.T) {
	t.Run("real permission box with y/n", func(t *testing.T) {
		if !hasPermissionPatterns(realPermissionBox) {
			t.Error("expected true for real permission box")
		}
	})

	t.Run("allow once UI", func(t *testing.T) {
		if !hasPermissionPatterns(allowOnceUI) {
			t.Error("expected true for allow-once UI")
		}
	})

	t.Run("explicit Allow? (y/n)", func(t *testing.T) {
		if !hasPermissionPatterns("some output\nAllow? (y/n)\n") {
			t.Error("expected true")
		}
	})

	t.Run("explicit Allow for this session", func(t *testing.T) {
		if !hasPermissionPatterns("Allow for this session") {
			t.Error("expected true")
		}
	})

	t.Run("idle claude output — no permission", func(t *testing.T) {
		idle := "> Running tests\n> All tests passed\n> Done."
		if hasPermissionPatterns(idle) {
			t.Error("expected false for idle output")
		}
	})

	t.Run("old permission in history should not match tail", func(t *testing.T) {
		// Simulate scroll history: old answered permission box, then enough new output
		// to push the box fully out of the 10-line tail window.
		after := "> Running tests\n> Test 1 passed\n> Test 2 passed\n> Test 3 passed\n" +
			"> Test 4 passed\n> Test 5 passed\n> All tests passed\n> Build complete\n" +
			"> Deploying\n> Done\n$ "
		history := realPermissionBox + "\ny\n" + after
		tail := tailLines(history, 10)
		if hasPermissionPatterns(tail) {
			t.Errorf("tail of completed session should not match permission patterns, tail=%q", tail)
		}
	})

	t.Run("box without y/n is not a permission prompt", func(t *testing.T) {
		// A box alone (e.g. Claude's output boxes) must not trigger.
		noYN := "╭────────╮\n│ output │\n╰────────╯\n"
		if hasPermissionPatterns(noYN) {
			t.Error("expected false for box without y/n")
		}
	})
}

// ---- hasWorkingPatterns ------------------------------------------------------

func TestHasWorkingPatterns(t *testing.T) {
	spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	for _, ch := range spinners {
		t.Run("spinner:"+ch, func(t *testing.T) {
			if !hasWorkingPatterns("processing " + ch) {
				t.Errorf("expected true for spinner %q", ch)
			}
		})
	}

	t.Run("Working...", func(t *testing.T) {
		if !hasWorkingPatterns("Working...") {
			t.Error("expected true")
		}
	})

	t.Run("Thinking...", func(t *testing.T) {
		if !hasWorkingPatterns("Thinking...") {
			t.Error("expected true")
		}
	})

	t.Run("hourglass", func(t *testing.T) {
		if !hasWorkingPatterns("⏳ please wait") {
			t.Error("expected true")
		}
	})

	t.Run("idle output", func(t *testing.T) {
		if hasWorkingPatterns("> Done.\n$ ") {
			t.Error("expected false for idle output")
		}
	})
}

// ---- parsePermissionRequest --------------------------------------------------

func TestParsePermissionRequest(t *testing.T) {
	t.Run("Claude wants to use Bash", func(t *testing.T) {
		content := "│ Claude wants to use Bash\n│ rm -rf /tmp/foo\n│ Allow? (y/n)"
		req := parsePermissionRequest(content)
		if req.Tool != "Bash" {
			t.Errorf("Tool = %q, want %q", req.Tool, "Bash")
		}
	})

	t.Run("Tool: prefix", func(t *testing.T) {
		content := "Tool: Edit\nCommand: /src/main.go\nAllow? (y/n)"
		req := parsePermissionRequest(content)
		if req.Tool != "Edit" {
			t.Errorf("Tool = %q, want %q", req.Tool, "Edit")
		}
		if req.Command != "/src/main.go" {
			t.Errorf("Command = %q, want %q", req.Command, "/src/main.go")
		}
	})

	t.Run("Command: prefix", func(t *testing.T) {
		content := "Tool: Bash\nCommand: go test ./...\nAllow? (y/n)"
		req := parsePermissionRequest(content)
		if req.Command != "go test ./..." {
			t.Errorf("Command = %q, want %q", req.Command, "go test ./...")
		}
	})

	t.Run("fallback tool detection from bare tool name", func(t *testing.T) {
		content := "╭──────╮\n│ Write │\n│ /tmp/x │\n╰──────╯\nAllow? (y/n)"
		req := parsePermissionRequest(content)
		if req.Tool != "Write" {
			t.Errorf("Tool = %q, want %q", req.Tool, "Write")
		}
	})

	t.Run("raw text is always set", func(t *testing.T) {
		content := "some content"
		req := parsePermissionRequest(content)
		if req.RawText != content {
			t.Errorf("RawText = %q, want %q", req.RawText, content)
		}
	})

	t.Run("empty content returns empty req", func(t *testing.T) {
		req := parsePermissionRequest("")
		if req.Tool != "" || req.Command != "" {
			t.Errorf("expected empty req, got Tool=%q Command=%q", req.Tool, req.Command)
		}
	})
}

// ---- buildChildMap -----------------------------------------------------------

func TestBuildChildMap(t *testing.T) {
	procs := []processInfo{
		{PID: 1, PPID: 0, Args: "init"},
		{PID: 2, PPID: 1, Args: "bash"},
		{PID: 3, PPID: 1, Args: "zsh"},
		{PID: 4, PPID: 2, Args: "claude"},
	}
	children := buildChildMap(procs)

	if len(children[1]) != 2 {
		t.Errorf("PID 1 should have 2 children, got %d", len(children[1]))
	}
	if len(children[2]) != 1 || children[2][0] != 4 {
		t.Errorf("PID 2 should have child 4, got %v", children[2])
	}
	if len(children[4]) != 0 {
		t.Errorf("PID 4 (leaf) should have no children, got %v", children[4])
	}
}

// ---- hasClaudeDescendant -----------------------------------------------------

func TestHasClaudeDescendant(t *testing.T) {
	procs := []processInfo{
		{PID: 100, PPID: 1, Args: "zsh"},
		{PID: 101, PPID: 100, Args: "node"},
		{PID: 102, PPID: 101, Args: "/usr/local/bin/claude --interactive"},
	}
	procMap := map[int]processInfo{
		100: procs[0],
		101: procs[1],
		102: procs[2],
	}
	children := buildChildMap(procs)

	t.Run("claude is a grandchild", func(t *testing.T) {
		if !hasClaudeDescendant(100, children, procMap, map[int]bool{}) {
			t.Error("expected true: claude is grandchild of 100")
		}
	})

	t.Run("direct claude child", func(t *testing.T) {
		if !hasClaudeDescendant(101, children, procMap, map[int]bool{}) {
			t.Error("expected true: claude is direct child of 101")
		}
	})

	t.Run("no claude in subtree", func(t *testing.T) {
		otherProcs := []processInfo{
			{PID: 200, PPID: 1, Args: "bash"},
			{PID: 201, PPID: 200, Args: "vim"},
		}
		pm := map[int]processInfo{200: otherProcs[0], 201: otherProcs[1]}
		cm := buildChildMap(otherProcs)
		if hasClaudeDescendant(200, cm, pm, map[int]bool{}) {
			t.Error("expected false: no claude in subtree")
		}
	})

	t.Run("cycle guard prevents infinite loop", func(t *testing.T) {
		// Artificial cycle
		cycleProcs := []processInfo{
			{PID: 300, PPID: 301, Args: "bash"},
			{PID: 301, PPID: 300, Args: "zsh"},
		}
		pm := map[int]processInfo{300: cycleProcs[0], 301: cycleProcs[1]}
		cm := buildChildMap(cycleProcs)
		// Should not hang
		result := hasClaudeDescendant(300, cm, pm, map[int]bool{})
		if result {
			t.Error("expected false: no claude in cycle")
		}
	})
}

// ---- Status.String -----------------------------------------------------------

func TestStatusString(t *testing.T) {
	tests := []struct {
		s    Status
		want string
	}{
		{StatusUnknown, "UNKNOWN"},
		{StatusNoClaud, "NO CLAUDE"},
		{StatusIdle, "IDLE"},
		{StatusWorking, "WORKING"},
		{StatusPermission, "PERMISSION"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Status(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}
