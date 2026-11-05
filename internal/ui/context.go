package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

const contextFileLimit = 64 * 1024

type contextInput struct {
	active, pending bool
	path            string
	position        int
	request         uint64
	err             error
}
type contextLoadedMsg struct {
	request            uint64
	paneID, path, text string
	err                error
}

func loadContextFile(base, path string) (string, string, error) {
	if strings.TrimSpace(path) == "" {
		return "", "", fmt.Errorf("enter a file path")
	}
	if !filepath.IsAbs(path) {
		if base == "" || !filepath.IsAbs(base) {
			return "", "", fmt.Errorf("target directory is unavailable; enter an absolute path")
		}
		path = filepath.Join(base, path)
	}
	path = filepath.Clean(path)
	// Nonblocking open prevents a FIFO substituted during path resolution from hanging.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", "", fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", "", fmt.Errorf("inspect file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("select a regular text file")
	}
	if info.Size() > contextFileLimit {
		return "", "", fmt.Errorf("file exceeds the 64 KiB limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, contextFileLimit+1))
	if err != nil {
		return "", "", fmt.Errorf("read file: %w", err)
	}
	if len(data) > contextFileLimit {
		return "", "", fmt.Errorf("file exceeds the 64 KiB limit")
	}
	if !utf8.Valid(data) {
		return "", "", fmt.Errorf("file is not valid UTF-8")
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", "", fmt.Errorf("file contains unsupported terminal controls")
		}
	}
	return path, text, nil
}

func (m *Model) contextKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	c := &m.composer
	if !c.context.active {
		if msg.String() != "ctrl+o" {
			return false, nil
		}
		c.context = contextInput{active: true, request: c.context.request + 1}
		m.updateViewport()
		return true, nil
	}
	a := &c.context
	if msg.String() == "esc" {
		a.active = false
		a.pending = false
		m.updateViewport()
		return true, nil
	}
	if a.pending {
		return true, nil
	}
	r := []rune(a.path)
	switch msg.String() {
	case "enter":
		if !m.composerTargetAvailable() {
			a.err = fmt.Errorf("target is unavailable; draft retained")
			break
		}
		base := m.composerDirectory()
		a.pending = true
		a.err = nil
		m.contextRequest++
		a.request = m.contextRequest
		token, target, path := a.request, c.paneID, a.path
		m.updateViewport()
		return true, func() tea.Msg {
			label, text, err := loadContextFile(base, path)
			return contextLoadedMsg{request: token, paneID: target, path: label, text: text, err: err}
		}
	case "left":
		if a.position > 0 {
			a.position--
		}
	case "right":
		if a.position < len(r) {
			a.position++
		}
	case "home", "ctrl+a":
		a.position = 0
	case "end", "ctrl+e":
		a.position = len(r)
	case "backspace", "ctrl+h":
		if a.position > 0 {
			a.path = string(append(r[:a.position-1], r[a.position:]...))
			a.position--
		}
	case "delete":
		if a.position < len(r) {
			a.path = string(append(r[:a.position], r[a.position+1:]...))
		}
	case "ctrl+u":
		a.path = ""
		a.position = 0
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			input := msg.Runes
			if msg.Type == tea.KeySpace {
				input = []rune{' '}
			}
			clean := make([]rune, 0, len(input))
			for _, v := range input {
				if !unicode.IsControl(v) {
					clean = append(clean, v)
				}
			}
			next := append([]rune{}, r[:a.position]...)
			next = append(next, clean...)
			next = append(next, r[a.position:]...)
			a.path = string(next)
			a.position += len(clean)
		}
	}
	m.updateViewport()
	return true, nil
}
func (m *Model) contextLoaded(msg contextLoadedMsg) {
	c := &m.composer
	a := &c.context
	if !c.active || !a.active || !a.pending || msg.request != a.request || msg.paneID != c.paneID {
		return
	}
	a.pending = false
	if msg.err != nil {
		a.err = msg.err
	} else if !m.composerTargetAvailable() {
		a.err = fmt.Errorf("target is unavailable; draft retained")
	} else {
		c.insert([]rune(fmt.Sprintf("\n[Attached file: %q]\n%s\n[End attached file]\n", msg.path, msg.text)))
		a.active = false
	}
	m.updateViewport()
}
func (m Model) contextView() string {
	a := m.composer.context
	view := fmt.Sprintf("Attach text file to pane %s (64 KiB maximum)\nPath: %s\n", m.composer.paneID, focusInput(a.path, a.position, max(1, m.viewport.Width-6)))
	if a.pending {
		view += "Reading...\n"
	}
	if a.err != nil {
		view += fmt.Sprintf("Error: %q\n", a.err.Error())
	}
	return view + "\nDraft (not sent):\n" + m.composer.draft
}

func (m Model) composerDirectory() string {
	source := m.allSessions
	if source == nil {
		source = m.sessions
	}
	for _, s := range source {
		if s.Pane.PaneID == m.composer.paneID && s.Provider == m.composer.provider {
			return s.Pane.CurrentPath
		}
	}
	return ""
}
