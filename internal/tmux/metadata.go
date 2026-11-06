package tmux

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

var paneMetadataFields = []string{"pane_id", "pane_index", "window_index", "session_name", "session_id", "pane_current_command", "pane_pid", "pane_width", "pane_height", "window_name", "pane_current_path", "@aeolus-shell", "@aeolus-capture"}

// tmux's n modifier measures bytes. Length framing preserves delimiters, Unicode,
// control bytes, and trailing spaces without interpreting any field as shell text.
func paneMetadataFormat() string {
	var b strings.Builder
	for _, field := range paneMetadataFields {
		fmt.Fprintf(&b, "#{n:%s}:#{%s}", field, field)
	}
	return b.String()
}

func parsePanes(data []byte) ([]Pane, error) {
	panes := make([]Pane, 0)
	for len(data) > 0 {
		fields := make([]string, len(paneMetadataFields))
		for i := range fields {
			colon := bytes.IndexByte(data, ':')
			if colon < 1 {
				return nil, fmt.Errorf("invalid tmux pane metadata length for %s", paneMetadataFields[i])
			}
			n, err := strconv.Atoi(string(data[:colon]))
			data = data[colon+1:]
			if err != nil || n < 0 || n > len(data) {
				return nil, fmt.Errorf("invalid tmux pane metadata size for %s", paneMetadataFields[i])
			}
			fields[i] = string(data[:n])
			data = data[n:]
		}
		if len(data) == 0 || data[0] != '\n' {
			return nil, fmt.Errorf("invalid tmux pane metadata record boundary")
		}
		data = data[1:]
		numbers := make([]int, 5)
		for i, index := range []int{1, 2, 6, 7, 8} {
			n, err := strconv.Atoi(fields[index])
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid tmux pane metadata %s", paneMetadataFields[index])
			}
			numbers[i] = n
		}
		if !paneIdentifier.MatchString(fields[0]) || !sessionIdentifier.MatchString(fields[4]) {
			return nil, fmt.Errorf("invalid tmux pane metadata identifier")
		}
		panes = append(panes, Pane{PaneID: fields[0], PaneIndex: numbers[0], WindowIndex: numbers[1], SessionName: fields[3], SessionID: fields[4], CurrentCommand: fields[5], PID: numbers[2], Width: numbers[3], Height: numbers[4], WindowName: fields[9], CurrentPath: fields[10], Shell: fields[11] == "v1", CaptureID: fields[12]})
	}
	return panes, nil
}
