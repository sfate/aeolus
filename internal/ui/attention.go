package ui

import (
	"fmt"

	"github.com/sfate/aeolus/internal/monitor"
)

type sessionAttention struct {
	pid      int
	provider monitor.Provider
	status   monitor.Status
	unread   string
}

func (a sessionAttention) matches(s monitor.Session) bool {
	return a.pid == s.Pane.PID && a.provider == s.Provider
}

// Observe only successful snapshots. New identities establish a baseline; removed panes are pruned rather than treated as completed work.
func (m *Model) observeAttention(sessions []monitor.Session) {
	next := make(map[string]sessionAttention, len(sessions))
	for _, s := range sessions {
		entry := sessionAttention{pid: s.Pane.PID, provider: s.Provider, status: s.Status}
		previous, found := m.attention[s.Pane.PaneID]
		if found && previous.matches(s) {
			entry.unread = previous.unread
			if s.Provider == monitor.ProviderClaude && s.Status != previous.status {
				switch {
				case s.Status == monitor.StatusPermission:
					entry.unread = "Permission requested"
				case s.Status == monitor.StatusQuestion:
					entry.unread = "Question asked"
				case previous.status == monitor.StatusWorking && s.Status == monitor.StatusIdle:
					entry.unread = "Became idle after working"
				}
			}
		}
		next[s.Pane.PaneID] = entry
	}
	m.attention = next
}

func (m Model) unreadAttention(s monitor.Session) string {
	entry, ok := m.attention[s.Pane.PaneID]
	if !ok || !entry.matches(s) {
		return ""
	}
	return entry.unread
}

func (m *Model) acknowledgeAttention() {
	if m.cursor < 0 || m.cursor >= len(m.sessions) {
		return
	}
	s := m.sessions[m.cursor]
	if m.unreadAttention(s) == "" {
		return
	}
	entry := m.attention[s.Pane.PaneID]
	entry.unread = ""
	m.attention[s.Pane.PaneID] = entry
	m.updateViewport()
}

func (m Model) attentionSummary() string {
	total, visible := 0, 0
	for _, entry := range m.attention {
		if entry.unread != "" {
			total++
		}
	}
	if total == 0 {
		return ""
	}
	for _, s := range m.sessions {
		if m.unreadAttention(s) != "" {
			visible++
		}
	}
	return fmt.Sprintf("!%d unread (%d hidden)  ", total, total-visible)
}
