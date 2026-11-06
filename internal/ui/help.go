package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfate/aeolus/internal/monitor"
)

type keyboardHelp struct {
	active   bool
	viewport viewport.Model
}

func (m Model) helpAvailable() bool {
	return m.confirmKill == "" && !m.review.active && !m.composer.active &&
		!m.filters.editing && m.lifecycle.kind == lifecycleNone && !m.search.editing
}

func (m *Model) resizeHelp() {
	m.help.viewport.Width = max(1, m.width)
	m.help.viewport.Height = max(1, m.height-4)
	m.help.viewport.SetContent(ansi.Hardwrap(m.helpText(), m.help.viewport.Width, true))
}

func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.help.active {
		m.help = keyboardHelp{active: true, viewport: viewport.New(1, 1)}
		m.resizeHelp()
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.help = keyboardHelp{}
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "home":
		m.help.viewport.GotoTop()
	case "end":
		m.help.viewport.GotoBottom()
	default:
		var cmd tea.Cmd
		m.help.viewport, cmd = m.help.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) helpText() string {
	context := "No session selected. Session actions require a visible selected session."
	if m.cursor >= 0 && m.cursor < len(m.sessions) {
		s := m.sessions[m.cursor]
		context = fmt.Sprintf("Selected provider: %s. Status: %s.\n", s.Provider, s.Status)
		switch {
		case s.CanRespond() && s.Status == monitor.StatusPermission:
			context += "[y] Approve  [a] Approve for session  [n] Deny"
		case s.CanRespond() && s.Status == monitor.StatusQuestion:
			context += "[y] Answer yes  [n] Answer no"
		default:
			context += "No approval shortcuts available. Open the pane to inspect or respond."
		}
	}
	return m.shortcutHelp(context + `

Dashboard
[j/down] Next session  [k/up] Previous session
[enter] Switch tmux client to the selected pane
[c] Launch agent  [R] Rename selected window
[x] Request pane termination; confirm with y, cancel with n/Esc
[r] Refresh sessions  [g] Read-only Git review
[f] Filter sessions  [/] Search captured output
[i] Compose a prompt  [?] Open this help
[ctrl+k] Search dashboard actions in the command palette
[q/ctrl+c] Quit from dashboard
[pgup/pgdown/ctrl+b/ctrl+f] Scroll output by page
[ctrl+u/ctrl+d] Scroll output by half page

Prompt composer
[enter] New line  [tab] Insert tab  [ctrl+s] Send explicitly
[ctrl+o] Attach a local UTF-8 text file for review
[esc] Discard draft; attachment Esc returns to draft
[arrows] Move cursor  [home/ctrl+a] Start of line
[end/ctrl+e] End of line  [backspace/ctrl+h/delete] Delete text
Attachment path: Enter reads, ctrl+u clears, Home/End move
Draft edits and actions are blocked while sending

Launch and rename forms
[tab/shift+tab] Change launch provider
[enter] Submit  [esc] Cancel before submission  [ctrl+c] Quit
[left/right] Move cursor  [home/ctrl+a] Start  [end/ctrl+e] End
[backspace/ctrl+h/delete] Delete text  [ctrl+u] Clear field
Submitted operations remain locked until completion
Rename affects every pane sharing the selected tmux window

Session filters
[p] Cycle provider  [s] Cycle status  [c] Clear all
[tab] Toggle controls / working-directory substring editing
[enter] Apply  [esc] Discard changes  [ctrl+c] Quit
Directory text: left/right, Home/End, Backspace/Delete, ctrl+u

Captured-output search
[enter] Keep query  [esc] Cancel edit or clear kept query
[ctrl+n/ctrl+p] Next/previous matching line after keeping query
[left/right] Move cursor  [home/ctrl+a] Start  [end/ctrl+e] End
[backspace/ctrl+h/delete] Delete text  [ctrl+u] Clear query
Search reads the latest captured output, not unlimited history

Git review
[up/down/j/k/pgup/pgdown] Scroll  [r] Reload snapshot
[ctrl+b/ctrl+f] Page  [ctrl+u/ctrl+d] Half page
[esc] Close  [ctrl+c] Quit

Command palette
[text] Filter actions  [up/down/ctrl+p/ctrl+n] Select action
[enter] Execute selected action  [esc] Cancel  [ctrl+c] Quit
[left/right/home/end] Edit query  [ctrl+u] Clear query
[backspace/ctrl+h/delete] Delete query text

Help
[up/down/j/k/pgup/pgdown] Scroll  [home/end] First/last page
[ctrl+b/ctrl+f] Page  [ctrl+u/ctrl+d] Half page
[esc] Close  [ctrl+c] Quit
Session shortcuts are disabled while help is open
In text inputs, ? remains ordinary text
`)
}

func (m Model) helpView() string {
	return "Keyboard help\n\n" + m.help.viewport.View() + "\n\n[↑↓/j/k/pgup/pgdown/home/end] Scroll  [esc] Close"
}
