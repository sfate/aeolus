package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfate/aeolus/internal/gitreview"
)

type gitReviewState struct {
	active, loading   bool
	paneID, directory string
	request           uint64
	viewport          viewport.Model
	cancel            context.CancelFunc
	read              func(context.Context, string) (gitreview.Snapshot, error)
}

type gitReviewMsg struct {
	request  uint64
	snapshot gitreview.Snapshot
	err      error
}

func (m *Model) gitReviewKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.review.active {
		if msg.String() != "g" {
			return false, nil
		}
		if m.cursor >= len(m.sessions) {
			return true, nil
		}
		pane := m.sessions[m.cursor].Pane
		m.review.active = true
		m.review.paneID = pane.PaneID
		m.review.directory = pane.CurrentPath
		m.review.viewport = viewport.New(max(1, m.width), max(1, m.height-7))
		return true, m.startGitReview()
	}
	switch msg.String() {
	case "esc":
		if m.review.cancel != nil {
			m.review.cancel()
		}
		m.review.active = false
		m.review.loading = false
		m.review.viewport.SetContent("")
	case "ctrl+c":
		if m.review.cancel != nil {
			m.review.cancel()
		}
		m.quitting = true
		return true, tea.Quit
	case "r":
		if !m.review.loading {
			return true, m.startGitReview()
		}
	default:
		var cmd tea.Cmd
		m.review.viewport, cmd = m.review.viewport.Update(msg)
		return true, cmd
	}
	return true, nil
}

func (m *Model) startGitReview() tea.Cmd {
	m.review.request++
	m.review.loading = true
	m.review.viewport.SetContent("Loading Git snapshot...")
	m.review.viewport.GotoTop()
	ctx, cancel := context.WithCancel(context.Background())
	m.review.cancel = cancel
	read := m.review.read
	if read == nil {
		read = gitreview.Read
	}
	directory, request := m.review.directory, m.review.request
	return func() tea.Msg {
		snapshot, err := read(ctx, directory)
		return gitReviewMsg{request: request, snapshot: snapshot, err: err}
	}
}

func (m *Model) finishGitReview(msg gitReviewMsg) {
	if !m.review.active || msg.request != m.review.request {
		return
	}
	if m.review.cancel != nil {
		m.review.cancel()
	}
	m.review.loading = false
	content := ""
	if msg.err != nil {
		content = "Cannot review this directory:\n\n" + safeReviewText(msg.err.Error()) + "\n\nUse r to retry, or Esc to close."
	} else {
		content = reviewSnapshotText(msg.snapshot)
	}
	m.review.viewport.SetContent(content)
	m.review.viewport.GotoTop()
}

func safeReviewText(text string) string {
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, ansi.Strip(text))
}

func reviewSnapshotText(snapshot gitreview.Snapshot) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Repository: %s\n\nChanged paths (index/worktree status):\n", strconv.Quote(snapshot.Directory))
	if len(snapshot.Files) == 0 {
		text.WriteString("(clean)\n")
	}
	for _, file := range snapshot.Files {
		fmt.Fprintf(&text, "%s %s", file.Status, strconv.Quote(file.Path))
		if file.OriginalPath != "" {
			fmt.Fprintf(&text, " <- %s", strconv.Quote(file.OriginalPath))
		}
		text.WriteByte('\n')
	}
	text.WriteString("\nUntracked paths are listed; their contents are not included in diffs.\n")
	if snapshot.Truncated {
		text.WriteString("\nOutput truncated: each diff is limited to 1 MiB.\n")
	}
	text.WriteString("\nStaged diff:\n")
	if snapshot.StagedDiff == "" {
		text.WriteString("(none)\n")
	} else {
		text.WriteString(snapshot.StagedDiff)
	}
	text.WriteString("\nUnstaged diff:\n")
	if snapshot.UnstagedDiff == "" {
		text.WriteString("(none)\n")
	} else {
		text.WriteString(snapshot.UnstagedDiff)
	}
	text.WriteString("\nSnapshot only. External filters, text conversion, and submodule diffs are disabled.\n")
	return safeReviewText(text.String())
}

func (m Model) gitReviewView() string {
	title := fmt.Sprintf("Git review - pane %s - directory %s\n", m.review.paneID, strconv.Quote(m.review.directory))
	return fitLine(safeReviewText(strings.TrimSuffix(title, "\n")), m.width) + "\n\n" + m.review.viewport.View() + "\n\n[r] Reload snapshot  [↑↓/j/k/pgup/pgdown] Scroll  [esc] Close"
}
