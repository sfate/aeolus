package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sfate/aeolus/internal/config"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/preferences"
	"github.com/sfate/aeolus/internal/tmux"
)

// ---- Messages ----------------------------------------------------------------

type tickMsg time.Time
type refreshRequestedMsg struct{}

type refreshMsg struct {
	sessions []monitor.Session
	err      error
}

type killPaneMsg struct {
	paneID string
	err    error
}

// statusIcon returns the icon for the given status.
func statusIcon(s monitor.Status) string {
	switch s {
	case monitor.StatusPermission:
		return "⚠ "
	case monitor.StatusQuestion:
		return "? "
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
	name := fmt.Sprintf("[%s] %s/%s", s.Provider, s.Pane.SessionName, s.Pane.WindowName)
	if s.Kind == monitor.SessionShell {
		name = fmt.Sprintf("[Shell] %s/%s", s.Pane.SessionName, s.Pane.WindowName)
	}
	return ansi.Truncate(name, 20, "…")
}

// ---- Model -------------------------------------------------------------------

// SessionMonitor supplies detected sessions to the TUI.
type SessionMonitor interface {
	Refresh([]tmux.Pane) []monitor.Session
}

// Model is the main Bubble Tea model for Aeolus.
type Model struct {
	theme             dashboardTheme
	keybindings       map[string]string
	preferencesSaving bool
	preferenceNotice  string
	preferenceSave    func(preferences.Snapshot) error
	interval          time.Duration
	projects          []config.Project
	defaultProvider   string
	backend           tmux.Backend
	monitor           SessionMonitor
	sessions          []monitor.Session
	cursor            int
	width             int
	height            int
	lastRefresh       time.Time
	err               error
	quitting          bool
	viewport          viewport.Model
	tmuxMissing       bool
	confirmKill       string
	search            outputSearch
	lifecycle         lifecycleForm
	composer          promptComposer
	contextRequest    uint64
	filters           sessionFilters
	allSessions       []monitor.Session
	review            gitReviewState
	promptRequest     uint64
	help              keyboardHelp
	refreshing        bool
	refreshQueued     bool
	refreshObsolete   bool
	response          *pendingResponse
	responseID        uint64
	worktree          worktreeForm
	palette           commandPalette
	attention         map[string]sessionAttention
}

// NewModel creates a new Model with sensible defaults.
func NewModel() Model { return NewModelWithRefreshInterval(config.DefaultRefreshInterval) }

// NewModelWithRefreshInterval creates a model with a custom polling interval.
func NewModelWithRefreshInterval(interval time.Duration) Model {
	return NewModelWithConfig(config.Config{RefreshInterval: interval})
}

// NewModelWithConfig creates a model with startup settings and copied project presets.
func NewModelWithConfig(settings config.Config) Model {
	arguments := make(map[string][]string, len(settings.ProviderArgs))
	for provider, args := range settings.ProviderArgs {
		arguments[provider] = append([]string{}, args...)
	}
	backend := tmux.Client{ProviderArgs: arguments}
	m := NewModelWithBackend(backend, monitor.New(backend))
	if settings.RefreshInterval >= 100*time.Millisecond && settings.RefreshInterval <= time.Minute {
		m.interval = settings.RefreshInterval
	}
	m.defaultProvider = string(launchProviders[launchProviderIndex(settings.DefaultProvider)])
	m.projects = append([]config.Project{}, settings.Projects...)
	m.theme = newDashboardTheme(settings.Theme)
	m.viewport.Style = m.theme.renderer.NewStyle()
	m.keybindings = make(map[string]string, len(settings.Keybindings))
	for action, key := range settings.Keybindings {
		m.keybindings[action] = key
	}
	return m
}

// NewModelWithBackend creates a TUI with injectable transport and state detection.
func NewModelWithBackend(backend tmux.Backend, sessionMonitor SessionMonitor) Model {
	theme := newDashboardTheme("dark")
	vp := viewport.New(80, 20)
	// Disable lipgloss word-wrap so raw ANSI escape sequences pass through intact.
	vp.Style = theme.renderer.NewStyle()
	return Model{
		theme:           theme,
		viewport:        vp,
		defaultProvider: "claude",
		interval:        config.DefaultRefreshInterval,
		backend:         backend,
		monitor:         sessionMonitor,
	}
}

// Init schedules polling and requests the first refresh through Update.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.tick(),
		func() tea.Msg { return refreshRequestedMsg{} },
	)
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *Model) doRefresh() tea.Cmd {
	// Keep monitor state single-threaded and coalesce repeated requests.
	if m.refreshing {
		m.refreshQueued = true
		return nil
	}
	m.refreshing = true
	return func() tea.Msg {
		panes, err := m.backend.ListAllPanes()
		if err != nil {
			return refreshMsg{err: err}
		}
		sessions := m.monitor.Refresh(panes)
		return refreshMsg{sessions: sessions}
	}
}

func (m Model) doKillPane(paneID string) tea.Cmd {
	return func() tea.Msg {
		return killPaneMsg{
			paneID: paneID,
			err:    m.backend.KillPane(paneID),
		}
	}
}

// Update handles incoming messages and user input.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case responseSentMsg:
		m.finishResponse(msg)
		return m, nil
	case worktreeCreatedMsg:
		return m.finishWorktreeCreate(msg)

	case worktreeLaunchedMsg:
		return m.finishWorktreeLaunch(msg)
	case preferencesSavedMsg:
		m.preferencesSaving = false
		if msg.err != nil {
			m.preferenceNotice = fmt.Sprintf("Cannot save dashboard filters: %v", msg.err)
		} else {
			m.preferenceNotice = "Dashboard filters saved. Clear filters and press S to reset startup choices."
		}
		return m, nil

	case gitReviewMsg:
		m.finishGitReview(msg)
		return m, nil

	case lifecycleMsg:
		return m.finishLifecycle(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		if m.review.active {
			m.review.viewport.Width = max(1, m.width)
			m.review.viewport.Height = max(1, m.height-7)
			m.review.viewport.SetYOffset(m.review.viewport.YOffset)
		}
		m.updateViewport()
		return m, nil

	case tickMsg:
		cmd := m.doRefresh()
		return m, tea.Batch(m.tick(), cmd)

	case refreshRequestedMsg:
		cmd := m.doRefresh()
		return m, cmd

	case refreshMsg:
		m.refreshing = false
		if m.refreshObsolete {
			m.refreshObsolete = false
			m.refreshQueued = false
			cmd := m.doRefresh()
			return m, cmd
		}
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
			selected := m.selectedPaneID()
			m.observeAttention(msg.sessions)
			m.allSessions = append([]monitor.Session{}, msg.sessions...)
			m.rebuildFilteredSessions(selected)
			m.lastRefresh = time.Now()
			// Clamp cursor
			if m.cursor >= len(m.sessions) && len(m.sessions) > 0 {
				m.cursor = len(m.sessions) - 1
			}
			if m.confirmKill != "" && !hasPane(m.sessions, m.confirmKill) {
				m.confirmKill = ""
			}
			m.updateViewport()
		}
		if m.refreshQueued {
			m.refreshQueued = false
			cmd := m.doRefresh()
			return m, cmd
		}
		return m, nil

	case killPaneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.confirmKill = ""
			m.updateViewport()
			return m, nil
		}
		m.confirmKill = ""
		m.refreshObsolete = m.refreshing
		selected := m.selectedPaneID()
		m.syncFilterSource()
		m.allSessions = removePane(m.allSessions, msg.paneID)
		delete(m.attention, msg.paneID)
		m.rebuildFilteredSessions(selected)
		if m.cursor >= len(m.sessions) && len(m.sessions) > 0 {
			m.cursor = len(m.sessions) - 1
		}
		m.updateViewport()
		cmd := m.doRefresh()
		return m, cmd

	case libraryResultMsg:
		m.libraryResult(msg)
		return m, nil

	case contextLoadedMsg:
		m.contextLoaded(msg)
		return m, nil

	case promptSentMsg:
		m.promptSent(msg)
		return m, nil

	case tea.MouseMsg:
		if m.palette.active {
			return m, nil
		}
		if m.help.active {
			var cmd tea.Cmd
			m.help.viewport, cmd = m.help.viewport.Update(msg)
			return m, cmd
		}
		if m.review.active {
			var cmd tea.Cmd
			m.review.viewport, cmd = m.review.viewport.Update(msg)
			return m, cmd
		}

	case tea.KeyMsg:
		if m.palette.active || (msg.String() == "ctrl+k" && !m.help.active && m.helpAvailable()) {
			return m.updatePalette(msg)
		}
		if m.help.active || (msg.String() == "?" && m.helpAvailable()) {
			return m.updateHelp(msg)
		}
		if m.confirmKill == "" && !m.review.active && !m.composer.active && !m.filters.editing && m.lifecycle.kind == lifecycleNone && !m.search.editing {
			mapped, ok := m.dashboardKey(msg)
			if !ok {
				return m, nil
			}
			msg = mapped
		}
		if m.confirmKill != "" {
			switch msg.String() {
			case "y":
				paneID := m.confirmKill
				m.confirmKill = ""
				m.updateViewport()
				return m, m.doKillPane(paneID)
			case "n", "esc":
				m.confirmKill = ""
				m.updateViewport()
				return m, nil
			case "q", "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}

		if m.worktree.active || (!m.composer.active && !m.filters.editing && m.lifecycle.kind == lifecycleNone && !m.search.editing && !m.review.active) {
			if handled, cmd := m.worktreeKey(msg); handled {
				return m, cmd
			}
		}

		if m.review.active || (!m.composer.active && !m.filters.editing && m.lifecycle.kind == lifecycleNone && !m.search.editing) {
			if handled, cmd := m.gitReviewKey(msg); handled {
				return m, cmd
			}
		}

		if !m.composer.active && !m.filters.editing {
			if handled, cmd := m.lifecycleKey(msg); handled {
				return m, cmd
			}
		}

		if !m.search.editing && !m.filters.editing {
			if handled, cmd := m.composerKey(msg); handled {
				return m, cmd
			}
		}

		if m.filters.editing || (!m.search.editing && msg.String() == "f") {
			if m.filterKey(msg) {
				if m.quitting {
					return m, tea.Quit
				}
				return m, nil
			}
		}

		if m.searchKey(msg) {
			if m.quitting {
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "!":
			m.acknowledgeAttention()
			return m, nil
		case "S":
			return m, m.savePreferences()
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.confirmKill = ""
				m.updateViewport()
			}
		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
				m.confirmKill = ""
				m.updateViewport()
			}
		case "y", "a", "n":
			return m.startResponse(msg.String())
		case "enter":
			if m.cursor < len(m.sessions) {
				s := m.sessions[m.cursor]
				if err := m.backend.SwitchToPane(s.Pane); err != nil {
					m.err = err
				}
			}
		case "x":
			if m.cursor < len(m.sessions) {
				m.search = outputSearch{}
				m.confirmKill = m.sessions[m.cursor].Pane.PaneID
				m.updateViewport()
			}
		case "r":
			cmd := m.doRefresh()
			return m, cmd
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

func hasPane(sessions []monitor.Session, paneID string) bool {
	for _, s := range sessions {
		if s.Pane.PaneID == paneID {
			return true
		}
	}
	return false
}

func removePane(sessions []monitor.Session, paneID string) []monitor.Session {
	filtered := sessions[:0]
	for _, s := range sessions {
		if s.Pane.PaneID != paneID {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// ---- Layout helpers ----------------------------------------------------------

func listPaneWidth(total int) int {
	if total < 80 {
		return max(1, total-2)
	}
	return max(1, total*2/5-2)
}

func contentPaneWidth(total int) int {
	if total < 80 {
		return max(1, total-2)
	}
	return max(1, total-listPaneWidth(total)-5)
}

func contentPaneHeight(total int) int { return max(1, total-7) }

// ---- Content helpers ---------------------------------------------------------

// updateViewport refreshes viewport content and positions the scroll:
// - permission sessions: top (so the parsed summary is immediately visible)
// - others: bottom (so the most recent terminal output is visible)
func (m *Model) updateViewport() {
	if m.help.active {
		m.resizeHelp()
	}
	m.resizeViewport()
	if m.composer.active {
		m.updateComposerViewport()
		return
	}
	if m.filters.editing {
		m.viewport.SetContent(m.filterModalText())
		m.viewport.GotoTop()
		return
	}
	if m.search.active() {
		m.updateSearchViewport()
		if m.search.active() {
			return
		}
	}
	m.viewport.SetContent(m.contentPaneText())
	s := monitor.Status(-1)
	if m.cursor < len(m.sessions) {
		s = m.sessions[m.cursor].Status
	}
	if s == monitor.StatusPermission || s == monitor.StatusQuestion {
		m.viewport.GotoTop()
	} else {
		m.viewport.GotoBottom()
	}
}

// tailLines returns the last n non-blank lines of text.
// tmux pads every line to terminal width with spaces, so trailing "blank"
// rows are actually space-filled strings. Strip them before taking the tail
// so GotoBottom() lands on actual content, not blank terminal padding.
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

// contentPaneText builds the text shown in the right-hand pane.
// For permission sessions: parsed summary on top + last 15 lines of raw pane for context.
// For others: last N lines so the most recent output is visible.
func (m *Model) contentPaneText() string {
	content := m.sessionContentPaneText()
	if m.cursor >= 0 && m.cursor < len(m.sessions) && m.confirmKill == "" {
		if unread := m.unreadAttention(m.sessions[m.cursor]); unread != "" {
			banner := "! Unread: " + unread + "  [!] Acknowledge"
			if m.sessions[m.cursor].Status == monitor.StatusPermission || m.sessions[m.cursor].Status == monitor.StatusQuestion {
				return banner + "\n\n" + content
			}
			return content + "\n\n" + banner
		}
	}
	return content
}

func (m *Model) sessionContentPaneText() string {
	if len(m.sessions) == 0 || m.cursor >= len(m.sessions) {
		return ""
	}
	s := m.sessions[m.cursor]

	if m.confirmKill == s.Pane.PaneID {
		var sb strings.Builder
		sb.WriteString(m.theme.renderer.NewStyle().Bold(true).Foreground(m.theme.colorError).Render("KILL PANE?") + "\n")
		sb.WriteString(strings.Repeat("─", 36) + "\n")
		sb.WriteString(fmt.Sprintf("  Pane:    %s\n", s.Pane.PaneID))
		sb.WriteString(fmt.Sprintf("  Session: %s\n", s.Pane.SessionName))
		sb.WriteString(fmt.Sprintf("  Window:  %s\n", s.Pane.WindowName))
		sb.WriteString(strings.Repeat("─", 36) + "\n")
		sb.WriteString(
			m.theme.renderer.NewStyle().Bold(true).Foreground(m.theme.colorError).Render("  [y] Kill pane") +
				"     " +
				m.theme.renderer.NewStyle().Bold(true).Foreground(m.theme.colorIdle).Render("[n/esc] Cancel") + "\n",
		)
		sb.WriteString("\n")
		sb.WriteString(m.theme.renderer.NewStyle().Foreground(m.theme.colorHelp).Render("─── context ───") + "\n")
		sb.WriteString(tailLines(s.Content, 15))
		return sb.String()
	}

	if s.CanRespond() && s.Status == monitor.StatusPermission {
		var sb strings.Builder
		req := s.Request

		sb.WriteString(m.theme.stylePermWarning.Render("⚠  PERMISSION REQUEST") + "\n")
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
			m.theme.renderer.NewStyle().Bold(true).Foreground(m.theme.colorIdle).Render("  [y] Approve") +
				"     " +
				m.theme.renderer.NewStyle().Bold(true).Foreground(m.theme.colorIdle).Render("[a] Approve all") +
				"     " +
				m.theme.renderer.NewStyle().Bold(true).Foreground(m.theme.colorError).Render("[n] Deny") + "\n",
		)
		sb.WriteString("\n")
		sb.WriteString(m.theme.renderer.NewStyle().Foreground(m.theme.colorHelp).Render("─── context ───") + "\n")
		sb.WriteString(tailLines(s.Content, 15))
		return sb.String()
	}

	if s.CanRespond() && s.Status == monitor.StatusQuestion {
		var sb strings.Builder
		sb.WriteString(m.theme.styleQuestionWarning.Render("?  QUESTION") + "\n")
		sb.WriteString(strings.Repeat("─", 36) + "\n")
		if s.Question != nil && s.Question.Text != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", s.Question.Text))
		}
		sb.WriteString(strings.Repeat("─", 36) + "\n")
		sb.WriteString(
			m.theme.renderer.NewStyle().Bold(true).Foreground(m.theme.colorIdle).Render("  [y] Yes") +
				"     " +
				m.theme.renderer.NewStyle().Bold(true).Foreground(m.theme.colorError).Render("[n] No") + "\n",
		)
		sb.WriteString("\n")
		sb.WriteString(m.theme.renderer.NewStyle().Foreground(m.theme.colorHelp).Render("─── context ───") + "\n")
		sb.WriteString(tailLines(s.Content, 15))
		return sb.String()
	}

	if s.Status == monitor.StatusUnknown {
		if s.Kind == monitor.SessionShell {
			return "Managed shell. Command status is unavailable without explicit shell integration.\n\n" + tailLines(s.Content, 50)
		}
		return fmt.Sprintf("%s activity is unknown. Open the pane to inspect or respond.\n\n%s", s.Provider, tailLines(s.Content, 50))
	}

	// For working/idle: show tail so newest output is at the bottom.
	return tailLines(s.Content, 50)
}

// ---- View --------------------------------------------------------------------

// View renders the full TUI.
func (m Model) View() string {
	view := m.view()
	if m.theme.name == "monochrome" {
		return ansi.Strip(view)
	}
	return view
}

func (m Model) view() string {
	if m.quitting {
		return ""
	}
	if m.width <= 0 || m.height <= 0 {
		return "Loading…"
	}
	if m.width < 24 || m.height < 12 {
		return fitScreen("Resize to at least 24x12.\n[q] Quit", m.width, m.height)
	}
	if m.palette.active {
		return fitScreen(m.paletteView(), m.width, m.height)
	}
	if m.help.active {
		return fitScreen(m.helpView(), m.width, m.height)
	}
	return fitScreen(m.screenView(), m.width, m.height)
}

func (m Model) screenView() string {
	// Header
	title := m.theme.styleHeader.Render(" Aeolus - Agent Session Manager ")
	ts := ""
	if !m.lastRefresh.IsZero() {
		ts = m.theme.renderer.NewStyle().Foreground(m.theme.colorHelp).Render(
			fmt.Sprintf(" last refresh: %s", m.lastRefresh.Format("15:04:05")),
		)
	}
	header := fitLine(lipgloss.JoinHorizontal(lipgloss.Top, title, ts), m.width)
	header += "\n" + fitLine(m.theme.styleHelp.Render(m.filterSummary()), m.width)
	if m.preferenceNotice != "" {
		header += "\n" + fitLine(safeReviewText(m.preferenceNotice), m.width)
	}

	if m.worktree.active {
		return lipgloss.JoinVertical(lipgloss.Left, header, "", m.worktreeView())
	}
	if m.review.active {
		return lipgloss.JoinVertical(lipgloss.Left, header, "", m.gitReviewView())
	}

	if m.lifecycle.kind != lifecycleNone {
		return lipgloss.JoinVertical(lipgloss.Left, header, "", m.lifecycleView())
	}

	// Error state
	if m.tmuxMissing {
		errMsg := m.theme.renderer.NewStyle().
			Bold(true).
			Foreground(m.theme.colorError).
			Padding(2, 4).
			Render("tmux server is not running.\n\nStart a tmux session and relaunch Aeolus.")
		return lipgloss.JoinVertical(lipgloss.Left, header, errMsg, m.helpBar())
	}
	if m.err != nil {
		errMsg := m.theme.renderer.NewStyle().
			Foreground(m.theme.colorError).
			Padding(1, 2).
			Render("Error: " + m.err.Error())
		return lipgloss.JoinVertical(lipgloss.Left, header, errMsg, m.helpBar())
	}

	lw, cw, lh, ph := m.paneDimensions()

	// Session list
	listContent := m.renderSessionList(lw, lh)
	listPane := m.theme.styleBorder.
		Width(lw).
		Height(lh).
		Render(listContent)

	// Content pane — dimensions/content are managed in Update(); View() just renders.
	var contentBorderStyle lipgloss.Style
	if m.cursor < len(m.sessions) {
		switch m.confirmKill {
		case m.sessions[m.cursor].Pane.PaneID:
			contentBorderStyle = m.theme.renderer.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(m.theme.colorError)
		default:
			switch m.sessions[m.cursor].Status {
			case monitor.StatusPermission:
				contentBorderStyle = m.theme.stylePermBorder
			case monitor.StatusQuestion:
				contentBorderStyle = m.theme.styleQuestionBorder
			default:
				contentBorderStyle = m.theme.styleBorder
			}
		}
	} else {
		contentBorderStyle = m.theme.styleBorder
	}
	contentPane := contentBorderStyle.
		Width(cw).
		Height(ph).
		Render(m.viewport.View())

	// Wide terminals keep the list beside output. Narrow terminals stack them.
	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		m.theme.renderer.NewStyle().MarginRight(1).Render(listPane),
		contentPane,
	)

	if m.width < 80 {
		panes = lipgloss.JoinVertical(lipgloss.Left, listPane, contentPane)
	}
	if m.composer.active || m.filters.editing {
		panes = contentPane
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		panes,
		"",
		fitLine(m.layoutFooter(), m.width),
	)
}

// renderSessionList builds the session list content.
func (m Model) renderSessionList(width, height int) string {
	if len(m.sessions) == 0 {
		return m.theme.renderer.NewStyle().
			Foreground(m.theme.colorNoClaud).
			Italic(true).
			Render(ansi.Truncate("No sessions match the current filters", max(0, width), "…"))
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

		icon := m.statusStyle(s.Status).Render(statusIcon(s.Status))
		name := sessionLabel(s)
		if m.unreadAttention(s) != "" {
			name = "! " + name
		}
		statusTag := fmt.Sprintf("[%s]", s.Status.String())

		// Measure terminal cells, preserving complete Unicode grapheme clusters.
		nameWidth := max(0, width-ansi.StringWidth(statusIcon(s.Status))-ansi.StringWidth(statusTag)-4)
		name = ansi.Truncate(name, nameWidth, "…")
		name += strings.Repeat(" ", max(0, nameWidth-ansi.StringWidth(name)))

		statusColored := m.statusStyle(s.Status).Render(statusTag)
		row := fmt.Sprintf("%s%s  %s", icon, name, statusColored)

		if selected && s.Status == monitor.StatusPermission {
			row = m.theme.renderer.NewStyle().Background(m.theme.colorPermBg).Bold(true).Render(row)
		} else if selected && s.Status == monitor.StatusQuestion {
			row = m.theme.renderer.NewStyle().Background(m.theme.colorQuestionBg).Bold(true).Render(row)
		} else if selected {
			row = m.theme.renderer.NewStyle().Background(m.theme.colorSelected).Bold(true).Render(row)
		} else if s.CanRespond() && s.Status == monitor.StatusPermission {
			row = m.theme.renderer.NewStyle().Background(m.theme.colorPermBg).Render(row)
		} else if s.CanRespond() && s.Status == monitor.StatusQuestion {
			row = m.theme.renderer.NewStyle().Background(m.theme.colorQuestionBg).Render(row)
		}

		// Cursor arrow
		cursor := "  "
		if selected {
			cursor = "▶ "
		}
		sb.WriteString(ansi.Truncate(cursor+row, max(0, width), "…") + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

// helpBar renders the bottom keybinding help.
func (m Model) helpBar() string {
	responses := ""
	if m.response != nil {
		return m.theme.styleHelp.Render("Sending response to " + m.response.session.Pane.PaneID + " - response keys disabled until delivery completes")
	}
	if m.cursor >= 0 && m.cursor < len(m.sessions) && m.unreadAttention(m.sessions[m.cursor]) != "" {
		responses = "  [!] Acknowledge"
	}
	if m.cursor < len(m.sessions) && m.sessions[m.cursor].CanRespond() &&
		(m.sessions[m.cursor].Status == monitor.StatusPermission || m.sessions[m.cursor].Status == monitor.StatusQuestion) {
		responses += "  [y] Approve/Yes  [a] Approve for session  [n] Deny/No"
	}
	return m.theme.styleHelp.Render(m.shortcutHelp("[↑↓/jk] Navigate  [enter] Go to pane" + responses + "  [c] Launch  [R] Rename  [x] Kill pane  [r] Refresh  [S] Save filters  [g] Git review  [W] Worktree  [f] Filter  [/] Search  [i] Compose  [?] Help  [ctrl+k] Palette  [q] Quit"))
}

func (m Model) footer() string {
	if m.composer.active {
		if m.composer.library.active {
			if m.composer.library.naming {
				return m.theme.styleHelp.Render("[enter] Save unique name  [esc] Back to library")
			}
			return m.theme.styleHelp.Render("[j/k] Select  [enter] Insert  [s] Save draft  [ctrl+r] Reload  [pgup/pgdown] Preview  [esc] Back")
		}
		if m.composer.context.active {
			return m.theme.styleHelp.Render("[enter] Attach for review  [esc] Back to draft")
		}
		return m.theme.styleHelp.Render("[enter] New line  [ctrl+s] Send  [ctrl+o] Attach file  [ctrl+p] Prompts  [esc] Cancel")
	}
	if m.filters.editing {
		return m.theme.styleHelp.Render("[tab] Controls/directory text  [enter] Apply  [esc] Cancel")
	}
	if m.search.active() {
		return m.searchHelp()
	}
	return m.helpBar()
}
