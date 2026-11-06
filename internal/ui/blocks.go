package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfate/aeolus/internal/bookmarks"
	"github.com/sfate/aeolus/internal/history"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/shellblocks"
)

type blockBrowser struct {
	active, loading, bookmarkOnly, bookmarkSaving bool
	request, bookmarkRequest                      uint64
	paneID, captureID                             string
	blocks                                        []shellblocks.Block
	bookmarked                                    map[uint64]bool
	selection                                     int
	err, bookmarkErr                              error
	notice                                        string
}

type blocksLoadedMsg struct {
	request     uint64
	blocks      []shellblocks.Block
	bookmarks   []bookmarks.Entry
	err         error
	bookmarkErr error
}

type blockBookmarkSavedMsg struct {
	request           uint64
	paneID, captureID string
	blockID           uint64
	bookmarked        bool
	err               error
}

type blockCopiedMsg struct {
	paneID, captureID string
	blockID           uint64
	kind              string
	err               error
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

func (m Model) visibleBlocks() []shellblocks.Block {
	if !m.blocks.bookmarkOnly {
		return m.blocks.blocks
	}
	visible := make([]shellblocks.Block, 0, len(m.blocks.blocks))
	for _, block := range m.blocks.blocks {
		if m.blocks.bookmarked[block.ID] {
			visible = append(visible, block)
		}
	}
	return visible
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
	m.blocks.err, m.blocks.bookmarkErr, m.blocks.notice = nil, nil, ""
	request, id := m.blocks.request, m.blocks.captureID
	read := m.blockRead
	if read == nil {
		read = shellblocks.ReadCapture
	}
	return func() tea.Msg {
		blocks, err := read(id)
		msg := blocksLoadedMsg{request: request, blocks: blocks, err: err}
		path, pathErr := bookmarks.DefaultPath()
		if pathErr != nil {
			msg.bookmarkErr = pathErr
		} else {
			entries, loadErr := bookmarks.Load(path)
			msg.bookmarkErr = loadErr
			if loadErr == nil {
				for _, entry := range entries {
					if entry.CaptureID == id {
						msg.bookmarks = append(msg.bookmarks, entry)
					}
				}
			}
		}
		return msg
	}
}

func (m *Model) finishBlocksLoad(msg blocksLoadedMsg) {
	if !m.blocks.active || msg.request != m.blocks.request || !m.captureAvailable(m.blocks.paneID, m.blocks.captureID) {
		return
	}
	m.blocks.loading = false
	m.blocks.err, m.blocks.bookmarkErr = msg.err, msg.bookmarkErr
	if msg.err != nil {
		m.blocks.blocks = nil
		m.blocks.bookmarked = nil
		m.updateViewport()
		return
	}
	selectedID := uint64(0)
	visible := m.visibleBlocks()
	if m.blocks.selection < len(visible) {
		selectedID = visible[m.blocks.selection].ID
	}
	m.blocks.blocks = msg.blocks
	m.blocks.bookmarked = make(map[uint64]bool, len(msg.bookmarks))
	for _, entry := range msg.bookmarks {
		m.blocks.bookmarked[entry.BlockID] = true
	}
	visible = m.visibleBlocks()
	m.blocks.selection = 0
	if len(visible) > 0 {
		m.blocks.selection = len(visible) - 1
		for i, block := range visible {
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
	visible := m.visibleBlocks()
	switch msg.String() {
	case "esc":
		m.blocks = blockBrowser{}
		m.updateViewport()
	case "ctrl+c":
		m.quitting = true
		return true, tea.Quit
	case "r":
		if m.blocks.loading || m.blocks.bookmarkSaving {
			return true, nil
		}
		return true, m.loadBlocks()
	case "b":
		selectedID := uint64(0)
		if m.blocks.selection < len(visible) {
			selectedID = visible[m.blocks.selection].ID
		}
		m.blocks.bookmarkOnly = !m.blocks.bookmarkOnly
		visible = m.visibleBlocks()
		m.blocks.selection = 0
		for i, block := range visible {
			if block.ID == selectedID {
				m.blocks.selection = i
				break
			}
		}
		m.updateViewport()
	case "m":
		if m.blocks.loading || m.blocks.bookmarkSaving || m.blocks.bookmarkErr != nil || m.blocks.selection >= len(visible) {
			return true, nil
		}
		return true, m.setBlockBookmark(visible[m.blocks.selection])
	case "c":
		if m.blocks.loading || m.blocks.selection >= len(visible) {
			return true, nil
		}
		return true, m.copyBlock(visible[m.blocks.selection], "command")
	case "o":
		if m.blocks.loading || m.blocks.selection >= len(visible) {
			return true, nil
		}
		return true, m.copyBlock(visible[m.blocks.selection], "output")
	case "i":
		if m.blocks.loading || m.blocks.selection >= len(visible) {
			return true, nil
		}
		m.insertCapturedBlock(visible[m.blocks.selection])
	case "s":
		if m.blocks.selection >= len(visible) || m.blocks.loading || m.historySaving {
			return true, nil
		}
		block := visible[m.blocks.selection]
		if block.ExitCode == nil || block.Incomplete {
			m.blocks.err = fmt.Errorf("only completed commands can be saved")
			m.updateViewport()
			return true, nil
		}
		m.blocks.err = nil
		return true, m.saveHistoryEntry(history.Entry{
			Command: block.Command, Directory: block.Directory, ExitCode: block.ExitCode,
			Duration: block.Duration, SavedAt: time.Now(),
		})
	case "up", "k":
		if m.blocks.selection > 0 {
			m.blocks.selection--
			m.updateViewport()
		}
	case "down", "j":
		if m.blocks.selection+1 < len(visible) {
			m.blocks.selection++
			m.updateViewport()
		}
	default:
		return true, nil
	}
	return true, nil
}

func (m *Model) setBlockBookmark(block shellblocks.Block) tea.Cmd {
	if !m.captureAvailable(m.blocks.paneID, m.blocks.captureID) {
		m.blocks.err = fmt.Errorf("captured pane is no longer available")
		m.updateViewport()
		return nil
	}
	if !shellblocks.IsValidCaptureID(m.blocks.captureID) || block.ID == 0 {
		m.blocks.err = fmt.Errorf("invalid capture or block ID")
		m.updateViewport()
		return nil
	}
	bookmarked := !m.blocks.bookmarked[block.ID]
	m.blocks.bookmarkSaving = true
	m.blocks.bookmarkRequest++
	request, paneID, captureID, blockID := m.blocks.bookmarkRequest, m.blocks.paneID, m.blocks.captureID, block.ID
	return func() tea.Msg {
		path, err := bookmarks.DefaultPath()
		if err == nil {
			err = bookmarks.Set(path, bookmarks.Entry{CaptureID: captureID, BlockID: blockID}, bookmarked)
		}
		return blockBookmarkSavedMsg{request: request, paneID: paneID, captureID: captureID, blockID: blockID, bookmarked: bookmarked, err: err}
	}
}

func (m *Model) finishBlockBookmark(msg blockBookmarkSavedMsg) {
	if !m.blocks.active || !m.captureAvailable(msg.paneID, msg.captureID) || m.blocks.paneID != msg.paneID || m.blocks.captureID != msg.captureID || m.blocks.bookmarkRequest != msg.request {
		return
	}
	m.blocks.bookmarkSaving = false
	m.blocks.bookmarkErr = msg.err
	if msg.err == nil {
		if m.blocks.bookmarked == nil {
			m.blocks.bookmarked = map[uint64]bool{}
		}
		if msg.bookmarked {
			m.blocks.bookmarked[msg.blockID] = true
			m.blocks.notice = "Block bookmarked."
		} else {
			delete(m.blocks.bookmarked, msg.blockID)
			m.blocks.notice = "Bookmark removed."
		}
		if visible := m.visibleBlocks(); m.blocks.selection >= len(visible) {
			m.blocks.selection = max(0, len(visible)-1)
		}
	}
	m.updateViewport()
}

func (m *Model) copyBlock(block shellblocks.Block, kind string) tea.Cmd {
	if !m.captureAvailable(m.blocks.paneID, m.blocks.captureID) {
		m.blocks.err = fmt.Errorf("captured pane is no longer available")
		m.updateViewport()
		return nil
	}
	copyText := m.blockCopy
	if copyText == nil {
		copyText = copyToClipboard
	}
	text := block.Command
	if kind == "output" {
		text = safeReviewText(string(block.Output))
	}
	paneID, captureID, blockID := m.blocks.paneID, m.blocks.captureID, block.ID
	return func() tea.Msg {
		return blockCopiedMsg{paneID: paneID, captureID: captureID, blockID: blockID, kind: kind, err: copyText(text)}
	}
}

func (m *Model) finishBlockCopy(msg blockCopiedMsg) {
	if !m.blocks.active || !m.captureAvailable(msg.paneID, msg.captureID) || m.blocks.paneID != msg.paneID || m.blocks.captureID != msg.captureID {
		return
	}
	if msg.err != nil {
		m.blocks.notice = "Clipboard unavailable: " + safeReviewText(msg.err.Error())
	} else {
		m.blocks.notice = "Copied command to clipboard."
		if msg.kind == "output" {
			m.blocks.notice = "Copied output to clipboard."
		}
	}
	m.updateViewport()
}

func (m *Model) insertCapturedBlock(block shellblocks.Block) {
	if !m.captureAvailable(m.blocks.paneID, m.blocks.captureID) {
		m.blocks.err = fmt.Errorf("captured pane is no longer available")
		m.updateViewport()
		return
	}
	var target *monitor.Session
	for i := range m.allSessions {
		if m.allSessions[i].Pane.PaneID == m.blocks.paneID && m.allSessions[i].Pane.CaptureID == m.blocks.captureID {
			target = &m.allSessions[i]
			break
		}
	}
	if target == nil || target.Kind != monitor.SessionShell {
		m.blocks.err = fmt.Errorf("captured shell is no longer available")
		m.updateViewport()
		return
	}
	m.blocks = blockBrowser{}
	m.composer = promptComposer{active: true, paneID: target.Pane.PaneID, provider: target.Provider, kind: target.Kind, draft: block.Command, position: len([]rune(block.Command))}
	m.updateViewport()
}

func copyToClipboard(text string) error {
	type clipboardCommand struct {
		name string
		args []string
	}
	var commands []clipboardCommand
	switch runtime.GOOS {
	case "darwin":
		commands = []clipboardCommand{{"pbcopy", nil}}
	case "windows":
		commands = []clipboardCommand{{"clip", nil}}
	default:
		commands = []clipboardCommand{{"wl-copy", nil}, {"xclip", []string{"-selection", "clipboard"}}, {"xsel", []string{"--clipboard", "--input"}}}
	}
	var lastErr error
	available := false
	for _, candidate := range commands {
		path, err := exec.LookPath(candidate.name)
		if err != nil {
			continue
		}
		available = true
		command := exec.Command(path, candidate.args...)
		command.Stdin = strings.NewReader(text)
		if err := command.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if !available {
		return fmt.Errorf("no supported clipboard utility found")
	}
	return fmt.Errorf("clipboard utility failed: %w", lastErr)
}

func (m Model) blocksView() string {
	b := m.blocks
	visible := m.visibleBlocks()
	var text strings.Builder
	fmt.Fprintf(&text, "Captured command blocks - pane %s\n\n", b.paneID)
	text.WriteString("Capture records commands and terminal output in private temporary storage. Output may contain secrets.\n")
	if b.bookmarkOnly {
		text.WriteString("Showing bookmarked blocks only.\n")
	}
	if b.loading {
		text.WriteString("\nLoading capture...\n")
	}
	if b.err != nil {
		fmt.Fprintf(&text, "\nCannot read capture: %s\n", safeReviewText(b.err.Error()))
	}
	if b.bookmarkErr != nil {
		fmt.Fprintf(&text, "\nBookmarks unavailable: %s\n", safeReviewText(b.bookmarkErr.Error()))
	}
	if b.notice != "" {
		fmt.Fprintf(&text, "\n%s\n", safeReviewText(b.notice))
	}
	if len(visible) == 0 && !b.loading {
		if b.bookmarkOnly {
			text.WriteString("\nNo bookmarked blocks in this capture.\n")
		} else {
			text.WriteString("\nNo command blocks captured yet.\n")
		}
	}
	for i, block := range visible {
		marker := "  "
		if i == b.selection {
			marker = "> "
		}
		star := "  "
		if b.bookmarked[block.ID] {
			star = "★ "
		}
		command := strings.ReplaceAll(block.Command, "\n", " ↵ ")
		width := m.width
		if width < 20 {
			width = 80
		}
		fmt.Fprintf(&text, "%s%s#%d %s\n", marker, star, block.ID, ansi.Truncate(safeReviewText(command), width-11, "…"))
	}
	if b.selection < len(visible) {
		block := visible[b.selection]
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
	text.WriteString("\n\n[↑↓/j/k] Select  [r] Refresh  [b] Bookmarked/all  [m] Bookmark")
	text.WriteString("  [c] Copy command  [o] Copy output  [i] Insert draft")
	if b.selection < len(visible) && visible[b.selection].ExitCode != nil && !visible[b.selection].Incomplete {
		text.WriteString("  [s] Save command to history")
	}
	text.WriteString("  [esc] Close")
	return text.String()
}
