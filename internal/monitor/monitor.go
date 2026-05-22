package monitor

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/blackholesun/aeolus/internal/tmux"
)

// Status represents the current state of a Claude Code session.
type Status int

const (
	StatusUnknown    Status = iota
	StatusNoClaud           // pane exists but no claude running
	StatusIdle              // claude running, idle
	StatusWorking           // claude actively processing/outputting
	StatusPermission        // waiting for permission approval
)

// String returns a human-readable label for the status.
func (s Status) String() string {
	switch s {
	case StatusNoClaud:
		return "NO CLAUDE"
	case StatusIdle:
		return "IDLE"
	case StatusWorking:
		return "WORKING"
	case StatusPermission:
		return "PERMISSION"
	default:
		return "UNKNOWN"
	}
}

// PermissionRequest holds details about a pending permission request.
type PermissionRequest struct {
	Tool    string // e.g. "Bash", "Edit", "Write"
	Command string // the command/path being requested
	RawText string // full captured context
}

// Session represents a tmux pane and its detected Claude Code state.
type Session struct {
	Pane      tmux.Pane
	Status    Status
	Request   *PermissionRequest
	Content   string    // last N lines of pane content
	UpdatedAt time.Time
}

// processInfo holds parsed process information.
type processInfo struct {
	PID  int
	PPID int
	Args string
}

// parseProcessList parses the output of `ps -eo pid,ppid,args`.
func parseProcessList() ([]processInfo, error) {
	cmd := exec.Command("ps", "-eo", "pid,ppid,args")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run ps: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	procs := make([]processInfo, 0, len(lines))

	for _, line := range lines[1:] { // skip header
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		args := strings.Join(fields[2:], " ")
		procs = append(procs, processInfo{PID: pid, PPID: ppid, Args: args})
	}

	return procs, nil
}

// buildChildMap creates a map from parent PID to child PIDs.
func buildChildMap(procs []processInfo) map[int][]int {
	children := make(map[int][]int)
	for _, p := range procs {
		children[p.PPID] = append(children[p.PPID], p.PID)
	}
	return children
}

// isClaudeProcess returns true if the process args look like a Claude Code process.
func isClaudeProcess(args string) bool {
	lower := strings.ToLower(args)
	// Match common claude binary patterns
	if strings.Contains(lower, "/claude") ||
		strings.Contains(lower, "claude-code") ||
		strings.HasSuffix(lower, " claude") ||
		strings.Contains(lower, " claude ") ||
		strings.HasPrefix(lower, "claude") {
		return true
	}
	return false
}

// hasClaudeDescendant checks if any descendant of rootPID is a Claude process.
func hasClaudeDescendant(rootPID int, children map[int][]int, procMap map[int]processInfo, visited map[int]bool) bool {
	if visited[rootPID] {
		return false
	}
	visited[rootPID] = true

	for _, childPID := range children[rootPID] {
		if proc, ok := procMap[childPID]; ok {
			if isClaudeProcess(proc.Args) {
				return true
			}
		}
		if hasClaudeDescendant(childPID, children, procMap, visited) {
			return true
		}
	}
	return false
}

// tailLines returns the last n lines of text.
func tailLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// parsePermissionRequest extracts permission request details from pane content.
func parsePermissionRequest(content string) *PermissionRequest {
	lines := strings.Split(content, "\n")
	req := &PermissionRequest{RawText: content}

	for i, line := range lines {
		stripped := strings.TrimSpace(line)

		// Look for tool name in permission box lines
		// Patterns: "│ Claude wants to use Bash", "│ Bash", "Tool: Bash"
		if strings.Contains(stripped, "Claude wants to use ") {
			after := strings.TrimPrefix(stripped, "│")
			after = strings.TrimSpace(after)
			parts := strings.SplitN(after, "Claude wants to use ", 2)
			if len(parts) == 2 {
				req.Tool = strings.TrimSpace(parts[1])
				req.Tool = strings.Trim(req.Tool, "│ ")
			}
		} else if strings.HasPrefix(stripped, "Tool:") {
			req.Tool = strings.TrimSpace(strings.TrimPrefix(stripped, "Tool:"))
		}

		// Try to parse command from the line after tool name
		if req.Tool != "" && req.Command == "" && i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			next = strings.Trim(next, "│ ")
			if next != "" && !strings.Contains(next, "Allow") && !strings.Contains(next, "Deny") && !strings.Contains(next, "╭") && !strings.Contains(next, "╰") {
				req.Command = next
			}
		}

		// Also try "Command:" prefix
		if strings.HasPrefix(stripped, "Command:") {
			req.Command = strings.TrimSpace(strings.TrimPrefix(stripped, "Command:"))
		}
	}

	// Fallback: try to detect tool from common patterns
	if req.Tool == "" {
		for _, line := range lines {
			stripped := strings.TrimSpace(line)
			stripped = strings.Trim(stripped, "│")
			stripped = strings.TrimSpace(stripped)
			for _, tool := range []string{"Bash", "Edit", "Write", "Read", "WebFetch", "WebSearch", "TodoWrite"} {
				if stripped == tool || strings.HasPrefix(stripped, tool+" ") {
					req.Tool = tool
					break
				}
			}
			if req.Tool != "" {
				break
			}
		}
	}

	return req
}

// hasPermissionPatterns checks if the content contains permission request indicators.
func hasPermissionPatterns(content string) bool {
	patterns := []string{
		"Allow? (y/n)",
		"(Y/n)",
		"(y/N)",
		"Do you want to",
		"Allow this",
		"allow this",
		"Deny",
		"Allow",
		// Claude Code permission box patterns
		"╭─",
		"Allow once",
		"Allow for this session",
		"Deny once",
	}

	// Check that at least permission-related patterns appear together
	hasBox := strings.Contains(content, "╭") && strings.Contains(content, "╰")
	hasYN := strings.Contains(content, "(y/n)") ||
		strings.Contains(content, "(Y/n)") ||
		strings.Contains(content, "(y/N)") ||
		strings.Contains(content, "Allow? ")

	if hasBox && hasYN {
		return true
	}

	// Check for explicit patterns
	for _, p := range patterns {
		if strings.Contains(content, p) {
			return true
		}
	}

	// Additional check: look for the characteristic permission UI
	if strings.Contains(content, "Allow once") || strings.Contains(content, "Allow for this session") {
		return true
	}

	return false
}

// hasWorkingPatterns checks if the content shows Claude is actively working.
func hasWorkingPatterns(content string) bool {
	spinnerChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	for _, ch := range spinnerChars {
		if strings.Contains(content, ch) {
			return true
		}
	}
	return strings.Contains(content, "⏳") ||
		strings.Contains(content, "Working...") ||
		strings.Contains(content, "Thinking...")
}

// DetectSession captures pane content and determines the Claude Code status.
func DetectSession(pane tmux.Pane, procs []processInfo) Session {
	session := Session{
		Pane:      pane,
		Status:    StatusUnknown,
		UpdatedAt: time.Now(),
	}

	// Capture last 50 lines of pane content
	content, err := tmux.CapturePane(pane.PaneID, 50)
	if err != nil {
		session.Status = StatusUnknown
		return session
	}
	session.Content = content

	// Build process maps for tree traversal
	procMap := make(map[int]processInfo, len(procs))
	for _, p := range procs {
		procMap[p.PID] = p
	}
	children := buildChildMap(procs)

	// Check if any descendant of the pane PID is a Claude process
	visited := make(map[int]bool)
	claudeRunning := hasClaudeDescendant(pane.PID, children, procMap, visited)

	// Also check if the current command itself is claude
	if !claudeRunning && isClaudeProcess(pane.CurrentCommand) {
		claudeRunning = true
	}

	if !claudeRunning {
		session.Status = StatusNoClaud
		return session
	}

	// Claude is running — determine its state from content.
	// Permission check uses only the last 10 lines: active prompts are always
	// at the bottom; older handled prompts must not trigger false positives.
	tail := tailLines(content, 10)
	if hasPermissionPatterns(tail) {
		session.Status = StatusPermission
		session.Request = parsePermissionRequest(content)
	} else if hasWorkingPatterns(content) {
		session.Status = StatusWorking
	} else {
		session.Status = StatusIdle
	}

	return session
}

// Refresh builds a Session list from the current set of panes.
func Refresh(panes []tmux.Pane) []Session {
	// Fetch process list once for all panes
	procs, _ := parseProcessList()

	sessions := make([]Session, 0, len(panes))
	for _, pane := range panes {
		sessions = append(sessions, DetectSession(pane, procs))
	}

	// Only include panes where Claude is actually running, sorted by urgency.
	sorted := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if s.Status == StatusPermission {
			sorted = append(sorted, s)
		}
	}
	for _, s := range sessions {
		if s.Status == StatusWorking {
			sorted = append(sorted, s)
		}
	}
	for _, s := range sessions {
		if s.Status == StatusIdle {
			sorted = append(sorted, s)
		}
	}

	return sorted
}
