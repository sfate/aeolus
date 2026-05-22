package monitor

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/blackholesun/aeolus/internal/tmux"
)

// reANSI matches ANSI/VT escape sequences including SGR colors and OSC8 hyperlinks.
var reANSI = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[a-zA-Z]|\][^\x1b]*(?:\x1b\\|\x07)|[@-Z\\-_])`)

func stripANSI(s string) string {
	return reANSI.ReplaceAllString(s, "")
}

// Status represents the current state of a Claude Code session.
type Status int

const (
	StatusUnknown    Status = iota
	StatusNoClaud           // pane exists but no claude running
	StatusIdle              // claude running, idle
	StatusWorking           // claude actively processing/outputting
	StatusPermission        // waiting for permission approval
	StatusQuestion          // claude asked a yes/no question
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
	case StatusQuestion:
		return "QUESTION"
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

// QuestionRequest holds details about a yes/no question Claude is asking.
type QuestionRequest struct {
	Text    string // the question line
	RawText string // full captured context
}

// Session represents a tmux pane and its detected Claude Code state.
type Session struct {
	Pane      tmux.Pane
	Status    Status
	Request   *PermissionRequest
	Question  *QuestionRequest
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

// tailLines returns the last n non-blank lines of text.
// tmux capture-pane pads every line to terminal width with spaces, so raw
// "blank" lines at the bottom are actually lines full of spaces — not empty.
// We strip those trailing whitespace-only lines before taking the tail so
// that detection patterns don't land in blank padding.
func tailLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
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
// Must be called on the tail of the pane only (last ~10 lines) to avoid false
// positives from already-answered prompts in the scroll history.
func hasPermissionPatterns(content string) bool {
	// Most reliable: box + y/n together.
	hasBox := strings.Contains(content, "╭") && strings.Contains(content, "╰")
	hasYN := strings.Contains(content, "(y/n)") ||
		strings.Contains(content, "(Y/n)") ||
		strings.Contains(content, "(y/N)") ||
		strings.Contains(content, "Allow? ")
	if hasBox && hasYN {
		return true
	}

	// Interactive selector UI (no y/n prompt, uses arrow keys).
	if strings.Contains(content, "Allow once") || strings.Contains(content, "Allow for this session") {
		return true
	}

	// Explicit patterns that are unambiguous on their own.
	// "Esc to cancel" appears in the newer numbered-selection permission UI
	// ("Do you want to proceed? / ❯ 1. Yes / 2. No / Esc to cancel").
	explicit := []string{
		"Allow? (y/n)",
		"Do you want to proceed",
		"Allow this action",
		"Esc to cancel",
	}
	for _, p := range explicit {
		if strings.Contains(content, p) {
			return true
		}
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
	if strings.Contains(content, "⏳") ||
		strings.Contains(content, "Working...") ||
		strings.Contains(content, "Thinking...") {
		return true
	}
	// Claude Code uses decorative Unicode symbols for active thinking:
	//   Misc Symbols U+2600–U+27FF: ✻ ✶ ✳ ✸ ✽ etc. → "✽ Wibbling…"
	//   Middle dot U+00B7: ·  → "· Booping…", "· Compacting context…"
	// Strip ANSI first so color codes don't hide the leading symbol.
	// Historical timing lines contain " for " ("✻ Crunched for 3s") — skip those.
	clean := stripANSI(content)
	for _, line := range strings.Split(clean, "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 || !strings.Contains(line, "…") || strings.Contains(line, " for ") {
			continue
		}
		first := []rune(line)[0]
		if (first > 0x2600 && first < 0x2800) || first == 0x00B7 {
			return true
		}
	}
	return false
}

// questionMetaLine returns true for lines that are UI chrome or user input rather than
// Claude's output: separators, timing (✻), recaps (※), status bar (🧬), accept-edits
// hint (⏵), prompt (❯), tool-output indicator (▎), and user-input continuation lines
// (indented with 2+ spaces, e.g. the second line of a multi-line ❯ block).
func questionMetaLine(line string) bool {
	if strings.HasPrefix(line, "  ") {
		return true
	}
	for _, prefix := range []string{"─", "✻", "※", "🧬", "⏵", "❯", "▎"} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// hasQuestionPatterns detects when Claude has asked a question and is waiting at the
// input prompt (❯). Claude Code does not use (y/n) suffixes for conversational questions;
// it ends the message with "?" and falls idle.
func hasQuestionPatterns(content string) bool {
	clean := stripANSI(content)
	lines := strings.Split(clean, "\n")

	// The empty ❯ input cursor must be visible — Claude is idle and waiting for input.
	// Historical user-input lines look like "❯ some text" (non-empty after ❯); skip those.
	hasPrompt := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "❯" {
			hasPrompt = true
			break
		}
	}
	if !hasPrompt {
		return false
	}

	// At least one non-meta line in this window must end with "?".
	// Pass the original line to questionMetaLine so the leading-spaces check
	// (user-input continuation) works before TrimSpace strips them.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || questionMetaLine(line) {
			continue
		}
		if strings.HasSuffix(trimmed, "?") {
			return true
		}
	}
	return false
}

// parseQuestionRequest extracts the question text from ANSI-stripped pane content.
func parseQuestionRequest(content string) *QuestionRequest {
	clean := stripANSI(content)
	lines := strings.Split(clean, "\n")
	// Walk backwards to find the most-recent line ending with "?".
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || questionMetaLine(trimmed) {
			continue
		}
		if strings.HasSuffix(trimmed, "?") {
			return &QuestionRequest{Text: trimmed, RawText: content}
		}
	}
	return &QuestionRequest{RawText: content}
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
	// Permission dialogs are ~10 lines tall; tail12 gives a small buffer without
	// catching previously-answered dialogs still in the scrollback.
	tail12 := tailLines(content, 12)
	tail30 := tailLines(content, 30)
	if hasPermissionPatterns(tail12) {
		session.Status = StatusPermission
		session.Request = parsePermissionRequest(content)
	} else if hasWorkingPatterns(content) {
		session.Status = StatusWorking
	} else if hasQuestionPatterns(tail30) {
		session.Status = StatusQuestion
		session.Question = parseQuestionRequest(tail30)
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

	// Only include panes where Claude is actually running, preserving pane order.
	sorted := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if s.Status != StatusNoClaud && s.Status != StatusUnknown {
			sorted = append(sorted, s)
		}
	}

	return sorted
}
