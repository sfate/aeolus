package monitor

import (
	"regexp"
	"strings"
)

// Codex renders this status row while a turn is running. Other Codex states
// remain unknown because the TUI does not expose them through a stable signal.
var codexWorkingStatus = regexp.MustCompile(`^\s*(?:\S+\s+)?Working \((?:\d+s|\d+m \d{2}s|\d+h \d{2}m \d{2}s) • esc to interrupt\)\s*$`)

func detectCodexStatus(content string) (Status, *PermissionRequest, *QuestionRequest) {
	for _, line := range strings.Split(tailLines(stripANSI(content), 8), "\n") {
		if codexWorkingStatus.MatchString(line) {
			return StatusWorking, nil, nil
		}
	}
	return StatusUnknown, nil, nil
}
