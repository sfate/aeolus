package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/blackholesun/aeolus/internal/monitor"
	"github.com/blackholesun/aeolus/internal/tmux"
)

// ---- Messages ----------------------------------------------------------------

type tickMsg time.Time

type refreshMsg struct {
	sessions []monitor.Session
	err      error
}

// ---- Styles ------------------------------------------------------------------

var (
	colorBorder     = lipgloss.Color("#444444")
	colorHeaderBg   = lipgloss.Color("#1A3A5C")
	colorPermission = lipgloss.Color("#FFB347")
	colorPermBg     = lipgloss.Color("#3D2E00")
	colorWorking    = lipgloss.Color("#00CFCF")
	colorIdle       = lipgloss.Color("#5AF78E")
	colorNoClaud    = lipgloss.Color("#666666")
	colorSelected   = lipgloss.Color("#1A1A2E")
	colorHelp       = lipgloss.Color("#555555")
	colorError      = lipgloss.Color("#FF5555")

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorHeaderBg).
			Padding(0, 2)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorHelp).
			Padding(0, 1)

	stylePermWarning = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPermission)

	stylePermBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPermission).
			Padding(1, 2)

	stylePermBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPermission)
)

// statusStyle returns a lipgloss style for the given status.
func statusStyle(s monitor.Status) lipgloss.Style {
	switch s {
	case monitor.StatusPermission:
		return lipgloss.NewStyle().Bold(true).Foreground(colorPermission)
	case monitor.StatusWorking:
		return lipgloss.NewStyle().Foreground(colorWorking)
	case monitor.StatusIdle:
		return lipgloss.NewStyle().Foreground(colorIdle)
	case monitor.StatusNoClaud:
		return lipgloss.NewStyle().Foreground(colorNoClaud)
	default:
		return lipgloss.NewStyle().Foreground(colorNoClaud)
	}
}

// statusIcon returns the icon for the given status.
func statusIcon(s monitor.Status) string {
	switch s {
	case monitor.StatusPermission:
		return "⚠ "
	case monitor.StatusWorking:
		return "● "
	case monitor.StatusIdle:
		return "○ "
	case monitor.StatusNoClaud:
		return "· "
	default:
		return "? "
	}
}

// sessionLabel builds the display label for a session.
func sessionLabel(s monitor.Session) string {
	name := fmt.Sprintf("%s/%s", s.Pane.SessionName, s.Pane.WindowName)
	if len(name) > 20 {
		name = name[:19] + "…"
	}
	return name
}

// ---- Model -------------------------------------------------------------------

// Model is the main Bubble Tea model for Aeolus.
type Model struct {
	sessions    []monitor.Session
	cursor      int
	width       int
	height      int
	lastRefresh time.Time
	err         error
	quitting    bool
	viewport    viewport.Model
	tmuxMissing bool
}

// NewModel creates a new Model with sensible defaults.
func NewModel() Model {
	vp := viewport.New(80, 20)
	return Model{
		viewport: vp,
	}
}

// Init returns the initial command — a tick to start polling.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tick(),
		doRefresh(),
	)
}

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func doRefresh() tea.Cmd {
	return func() tea.Msg {
		panes, err := tmux.ListAllPanes()
		if err != nil {
			return refreshMsg{err: err}
		}
		sessions := monitor.Refresh(panes)
		return refreshMsg{sessions: sessions}
	}
}

// Update handles incoming messages and user input.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = contentPaneWidth(m.width)
		m.viewport.Height = contentPaneHeight(m.height)
		m.updateViewport()
		return m, nil

	case tickMsg:
		return m, tea.Batch(tick(), doRefresh())

	case refreshMsg:
		if msg.err != nil {
			m.err = msg.err
			// Detect tmux-not-running specifically
			if strings.Contains(msg.err.Error(), "not running") ||
				strings.Contains(msg.err.Error(), "no server") {
				m.tmuxMissing = true
			}
		} else {
			m.err = nil
			m.tmuxMissing = false
			m.sessions = msg.sessions
			m.lastRefresh = time.Now()
			// Clamp cursor
			if m.cursor >= len(m.sessions) && len(m.sessions) > 0 {
				m.cursor = len(m.sessions) - 1
			}
			m.updateViewport()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.updateViewport()
			}
		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
				m.updateViewport()
			}
		case "y":
			if m.cursor < len(m.sessions) {
				s := m.sessions[m.cursor]
				if s.Status == monitor.StatusPermission {
					_ = tmux.SendKeys(s.Pane.PaneID, "y")
					// Optimistic update
					m.sessions[m.cursor].Status = monitor.StatusWorking
					m.sessions[m.cursor].Request = nil
					m.updateViewport()
				}
			}
		case "n":
			if m.cursor < len(m.sessions) {
				s := m.sessions[m.cursor]
				if s.Status == monitor.StatusPermission {
					_ = tmux.SendKeys(s.Pane.PaneID, "n")
					m.sessions[m.cursor].Status = monitor.StatusIdle
					m.sessions[m.cursor].Request = nil
					m.updateViewport()
				}
			}
		case "r":
			return m, doRefresh()
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}

	// Let viewport handle scroll keys if focus is there
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// ---- Layout helpers ----------------------------------------------------------

func listPaneWidth(total int) int {
	w := total*2/5 - 4
	if w < 24 {
		w = 24
	}
	return w
}

func contentPaneWidth(total int) int {
	w := total - listPaneWidth(total) - 8
	if w < 20 {
		w = 20
	}
	return w
}

func contentPaneHeight(total int) int {
	h := total - 7 // header + help bar + borders
	if h < 5 {
		h = 5
	}
	return h
}

// ---- Content helpers ---------------------------------------------------------

// updateViewport refreshes viewport content and positions the scroll:
// - permission sessions: top (so the parsed summary is immediately visible)
// - others: bottom (so the most recent terminal output is visible)
func (m *Model) updateViewport() {
	m.viewport.SetContent(m.contentPaneText())
	if m.cursor < len(m.sessions) && m.sessions[m.cursor].Status == monitor.StatusPermission {
		m.viewport.GotoTop()
	} else {
		m.viewport.GotoBottom()
	}
}

// tailLines returns the last n lines of text.
func tailLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// contentPaneText builds the text shown in the right-hand pane.
// For permission sessions: parsed summary on top + last 15 lines of raw pane for context.
// For others: last N lines so the most recent output is visible.
func (m *Model) contentPaneText() string {
	if len(m.sessions) == 0 || m.cursor >= len(m.sessions) {
		return ""
	}
	s := m.sessions[m.cursor]

	if s.Status == monitor.StatusPermission {
		var sb strings.Builder
		req := s.Request

		sb.WriteString(stylePermWarning.Render("⚠  PERMISSION REQUEST") + "\n")
		sb.WriteString(strings.Repeat("─", 36) + "\n")
		if req != nil {
			if req.Tool != "" {
				sb.WriteString(fmt.Sprintf("  Tool:    %s\n", req.Tool))
			}
			if req.Command != "" {
				sb.WriteString(fmt.Sprintf("  Command: %s\n", req.Command))
			}
		}
		sb.WriteString(strings.Repeat("─", 36) + "\n")
		sb.WriteString(
			lipgloss.NewStyle().Bold(true).Foreground(colorIdle).Render("  [y] Approve") +
				"     " +
				lipgloss.NewStyle().Bold(true).Foreground(colorError).Render("[n] Deny") + "\n",
		)
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(colorHelp).Render("─── context ───") + "\n")
		sb.WriteString(tailLines(s.Content, 15))
		return sb.String()
	}

	// For working/idle: show tail so newest output is at the bottom.
	// Viewport will be scrolled to bottom after this.
	return tailLines(s.Content, 50)
}

// ---- View --------------------------------------------------------------------

// View renders the full TUI.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.width == 0 {
		return "Loading…"
	}

	// Header
	title := styleHeader.Render(" Aeolus — Claude Code Session Manager ")
	ts := ""
	if !m.lastRefresh.IsZero() {
		ts = lipgloss.NewStyle().Foreground(colorHelp).Render(
			fmt.Sprintf(" last refresh: %s", m.lastRefresh.Format("15:04:05")),
		)
	}
	header := lipgloss.JoinHorizontal(lipgloss.Top, title, ts)

	// Error state
	if m.tmuxMissing {
		errMsg := lipgloss.NewStyle().
			Bold(true).
			Foreground(colorError).
			Padding(2, 4).
			Render("tmux server is not running.\n\nStart a tmux session and relaunch Aeolus.")
		return lipgloss.JoinVertical(lipgloss.Left, header, errMsg, helpBar())
	}
	if m.err != nil {
		errMsg := lipgloss.NewStyle().
			Foreground(colorError).
			Padding(1, 2).
			Render("Error: " + m.err.Error())
		return lipgloss.JoinVertical(lipgloss.Left, header, errMsg, helpBar())
	}

	lw := listPaneWidth(m.width)
	cw := contentPaneWidth(m.width)
	ph := contentPaneHeight(m.height)

	// Session list
	listContent := m.renderSessionList(lw, ph)
	listPane := styleBorder.
		Width(lw).
		Height(ph).
		Render(listContent)

	// Content pane
	m.viewport.Width = cw
	m.viewport.Height = ph
	m.updateViewport()

	var contentBorderStyle lipgloss.Style
	if m.cursor < len(m.sessions) && m.sessions[m.cursor].Status == monitor.StatusPermission {
		contentBorderStyle = stylePermBorder
	} else {
		contentBorderStyle = styleBorder
	}
	contentPane := contentBorderStyle.
		Width(cw).
		Height(ph).
		Render(m.viewport.View())

	// Join panes horizontally
	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().MarginRight(1).Render(listPane),
		contentPane,
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		panes,
		"",
		helpBar(),
	)
}

// renderSessionList builds the session list content.
func (m Model) renderSessionList(width, height int) string {
	if len(m.sessions) == 0 {
		return lipgloss.NewStyle().
			Foreground(colorNoClaud).
			Italic(true).
			Render("No Claude Code sessions found")
	}

	var sb strings.Builder
	maxVisible := height
	start := 0
	if m.cursor >= maxVisible {
		start = m.cursor - maxVisible + 1
	}

	for i := start; i < len(m.sessions) && i < start+maxVisible; i++ {
		s := m.sessions[i]
		selected := i == m.cursor

		icon := statusStyle(s.Status).Render(statusIcon(s.Status))
		name := sessionLabel(s)
		statusTag := fmt.Sprintf("[%s]", s.Status.String())

		// Pad name to fixed width for alignment
		nameWidth := width - 4 - len(statusTag) - 3
		if nameWidth < 1 {
			nameWidth = 1
		}
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		} else {
			name = fmt.Sprintf("%-*s", nameWidth, name)
		}

		statusColored := statusStyle(s.Status).Render(statusTag)
		row := fmt.Sprintf("%s%s  %s", icon, name, statusColored)

		if selected && s.Status == monitor.StatusPermission {
			row = lipgloss.NewStyle().
				Background(colorPermBg).
				Bold(true).
				Render(row)
		} else if selected {
			row = lipgloss.NewStyle().
				Background(colorSelected).
				Bold(true).
				Render(row)
		} else if s.Status == monitor.StatusPermission {
			row = lipgloss.NewStyle().
				Background(colorPermBg).
				Render(row)
		}

		// Cursor arrow
		cursor := "  "
		if selected {
			cursor = "▶ "
		}
		sb.WriteString(cursor + row + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

// helpBar renders the bottom keybinding help.
func helpBar() string {
	return styleHelp.Render(
		"[↑↓/jk] Navigate  [y] Approve  [n] Deny  [r] Refresh  [q] Quit",
	)
}
