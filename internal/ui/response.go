package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/monitor"
)

type pendingResponse struct {
	id      uint64
	session monitor.Session
	key     string
}

type responseSentMsg struct {
	id  uint64
	err error
}

func (m Model) startResponse(key string) (tea.Model, tea.Cmd) {
	if m.response != nil || m.cursor < 0 || m.cursor >= len(m.sessions) {
		return m, nil
	}
	s := m.sessions[m.cursor]
	if !s.CanRespond() || (s.Status != monitor.StatusPermission && s.Status != monitor.StatusQuestion) || (key == "a" && s.Status != monitor.StatusPermission) {
		return m, nil
	}
	// Copy request values so later refreshes cannot change the completion target.
	if s.Request != nil {
		request := *s.Request
		s.Request = &request
	}
	if s.Question != nil {
		question := *s.Question
		s.Question = &question
	}
	m.responseID++
	response := pendingResponse{id: m.responseID, session: s, key: key}
	m.response = &response
	m.err = nil
	backend := m.backend
	return m, func() tea.Msg {
		var err error
		if key == "a" {
			err = backend.SendKeySequence(s.Pane.PaneID, "Down", "Enter")
		} else {
			err = backend.SendKeys(s.Pane.PaneID, key)
		}
		return responseSentMsg{id: response.id, err: err}
	}
}

func sameResponseTarget(current, original monitor.Session) bool {
	if current.Pane.PaneID != original.Pane.PaneID || current.Pane.PID != original.Pane.PID || current.Provider != original.Provider || current.Status != original.Status {
		return false
	}
	if (current.Request == nil) != (original.Request == nil) || (current.Question == nil) != (original.Question == nil) {
		return false
	}
	if current.Request != nil && *current.Request != *original.Request {
		return false
	}
	return current.Question == nil || *current.Question == *original.Question
}

func (m *Model) finishResponse(msg responseSentMsg) {
	if m.response == nil || msg.id != m.response.id {
		return
	}
	response := *m.response
	m.response = nil
	if msg.err != nil {
		m.err = fmt.Errorf("response to %s failed; inspect the pane before retrying: %w", response.session.Pane.PaneID, msg.err)
		m.updateViewport()
		return
	}
	selected := m.selectedPaneID()
	m.syncFilterSource()
	for i, s := range m.allSessions {
		if !sameResponseTarget(s, response.session) {
			continue
		}
		m.allSessions[i].Status = monitor.StatusWorking
		if response.key == "n" {
			m.allSessions[i].Status = monitor.StatusIdle
		}
		m.allSessions[i].Request = nil
		m.allSessions[i].Question = nil
		break
	}
	m.rebuildFilteredSessions(selected)
}
