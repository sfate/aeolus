package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfate/aeolus/internal/history"
	"github.com/sfate/aeolus/internal/shellblocks"
)

type blockBrowser struct {
	active, loading   bool
	request           uint64
	paneID, captureID string
	blocks            []shellblocks.Block
	selection         int
	err               error
}

type blocksLoadedMsg struct {
	request uint64
	blocks  []shellblocks.Block
	err     error
}

func (m Model) captureAvailable(paneID, captureID string) bool {
	if !shellblocks.IsValidCaptureID(captureID) {
		return false
	}
	for _, session := range m.allSessions {
		if session.Pane.PaneID == paneID && session.Pane.CaptureID == captureID {
			return true
		}
	}
	return false
}

func (m *Model) openBlockBrowser() tea.Cmd {
	if m.cursor >= len(m.sessions) {
		return nil
	}
	pane := m.sessions[m.cursor].Pane
	if !shellblocks.IsValidCaptureID(pane.CaptureID) {
		return nil
	}
	m.blocks = blockBrowser{active: true, paneID: pane.PaneID, captureID: pane.CaptureID}
	m.updateViewport()
	return m.loadBlocks()
}

func (m *Model) loadBlocks() tea.Cmd {
	if !m.blocks.active || m.blocks.loading || !m.captureAvailable(m.blocks.paneID, m.blocks.captureID) {
		m.blocks.err = fmt.Errorf("captured pane is no longer available")
		m.updateViewport()
		return nil
	}
	m.blocks.request++
	m.blocks.loading = true
	m.blocks.err = nil
	request, id := m.blocks.request, m.blocks.captureID
	read := m.blockRead
	if read == nil {
		read = shellblocks.ReadCapture
	}
	return func() tea.Msg {
		blocks, err := read(id)
		return blocksLoadedMsg{request: request, blocks: blocks, err: err}
	}
}

func (m *Model) finishBlocksLoad(msg blocksLoadedMsg) {
	if !m.blocks.active || msg.request != m.blocks.request || !m.captureAvailable(m.blocks.paneID, m.blocks.captureID) {
		return
	}
	m.blocks.loading = false
	if msg.err != nil {
		m.blocks.err = msg.err
		m.updateViewport()
		return
	}
	selectedID := uint64(0)
	if m.blocks.selection < len(m.blocks.blocks) {
		selectedID = m.blocks.blocks[m.blocks.selection].ID
	}
	m.blocks.blocks = msg.blocks
	m.blocks.selection = 0
	if len(msg.blocks) > 0 {
		m.blocks.selection = len(msg.blocks) - 1
		for i, block := range msg.blocks {
			if block.ID == selectedID {
				m.blocks.selection = i
				break
			}
		}
	}
	m.updateViewport()
}

func (m *Model) blockBrowserKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.blocks.active {
		return false, nil
	}
	switch msg.String() {
	case "esc":
		m.blocks = blockBrowser{}
		m.updateViewport()
	case "ctrl+c":
		m.quitting = true
		return true, tea.Quit
	case "r":
		if m.blocks.loading {
			return true, nil
		}
		return true, m.loadBlocks()
	case "s":
		if m.blocks.selection >= len(m.blocks.blocks) || m.blocks.loading || m.historySaving {
			return true, nil
		}
		block := m.blocks.blocks[m.blocks.selection]
		if block.ExitCode == nil || block.Incomplete {
			m.blocks.err = fmt.Errorf("only completed commands can be saved")
			m.updateViewport()
			return true, nil
		}
		m.blocks.err = nil
		return true, m.saveHistoryEntry(history.Entry{
			Command:   block.Command,
			Directory: block.Directory,
			ExitCode:  block.ExitCode,
			Duration:  block.Duration,
			SavedAt:   time.Now(),
		})
	case "up", "k":
		if m.blocks.selection > 0 {
			m.blocks.selection--
			m.updateViewport()
		}
	case "down", "j":
		if m.blocks.selection+1 < len(m.blocks.blocks) {
			m.blocks.selection++
			m.updateViewport()
		}
	default:
		return true, nil
	}
	return true, nil
}

func (m Model) blocksView() string {
	b := m.blocks
	var text strings.Builder
	fmt.Fprintf(&text, "Captured command blocks - pane %s\n\n", b.paneID)
	text.WriteString("Capture records commands and terminal output in private temporary storage. Output may contain secrets.\n")
	if b.loading {
		text.WriteString("\nLoading capture...\n")
	}
	if b.err != nil {
		fmt.Fprintf(&text, "\nCannot read capture: %s\n", safeReviewText(b.err.Error()))
	}
	if len(b.blocks) == 0 && !b.loading {
		text.WriteString("\nNo command blocks captured yet.\n")
	}
	for i, block := range b.blocks {
		marker := "  "
		if i == b.selection {
			marker = "> "
		}
		command := strings.ReplaceAll(block.Command, "\n", " ↵ ")
		width := m.width
		if width < 20 {
			width = 80
		}
		fmt.Fprintf(&text, "%s#%d %s\n", marker, block.ID, ansi.Truncate(safeReviewText(command), width-8, "…"))
	}
	if b.selection < len(b.blocks) {
		block := b.blocks[b.selection]
		status := "running or incomplete"
		if block.ExitCode != nil {
			status = fmt.Sprintf("exit %d", *block.ExitCode)
		}
		flags := make([]string, 0, 2)
		if block.Incomplete {
			flags = append(flags, "incomplete")
		}
		if block.Truncated {
			flags = append(flags, "output truncated")
		}
		fmt.Fprintf(&text, "\nDirectory: %s\nStarted: %s\nDuration: %s  Status: %s", strconv.Quote(safeReviewText(block.Directory)), block.StartedAt.Local().Format(time.RFC3339), block.Duration.Round(time.Millisecond), status)
		if len(flags) > 0 {
			fmt.Fprintf(&text, "  (%s)", strings.Join(flags, ", "))
		}
		text.WriteString("\n\nOutput:\n")
		output := safeReviewText(string(block.Output))
		if output == "" {
			output = "(no captured output)"
		}
		text.WriteString(output)
	}
	text.WriteString("\n\n[↑↓/j/k] Select  [r] Refresh")
	if b.selection < len(b.blocks) && b.blocks[b.selection].ExitCode != nil && !b.blocks[b.selection].Incomplete {
		text.WriteString("  [s] Save command to history")
	}
	text.WriteString("  [esc] Close")
	return text.String()
}
