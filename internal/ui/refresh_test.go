package ui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
)

func TestRefreshRequestsCoalesce(t *testing.T) {
	backend := &fakeBackend{panes: []tmux.Pane{{PaneID: "%1"}}}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	first := m.doRefresh()
	if first == nil {
		t.Fatal("first refresh was not scheduled")
	}
	for range 3 {
		var cmd tea.Cmd
		m, cmd = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		if cmd != nil {
			t.Fatal("overlapping refresh scheduled instead of coalesced")
		}
	}
	m, followup := updateModel(t, m, first())
	if followup == nil || len(m.sessions) != 1 {
		t.Fatal("completed refresh must apply and schedule one follow-up")
	}
	m, extra := updateModel(t, m, followup())
	if extra != nil {
		t.Fatal("coalesced requests produced more than one follow-up")
	}
	if m.doRefresh() == nil {
		t.Fatal("completed refresh did not release refresh gate")
	}
}

func TestRefreshErrorReleasesGateAndRunsQueuedRequest(t *testing.T) {
	backend := &fakeBackend{listErr: errors.New("no server")}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	first := m.doRefresh()
	m.doRefresh()
	m, next := updateModel(t, m, first())
	if m.err == nil || !m.tmuxMissing || next == nil {
		t.Fatal("failed refresh must report error and schedule queued request")
	}
	backend.listErr = nil
	backend.panes = []tmux.Pane{{PaneID: "%2"}}
	m, cmd := updateModel(t, m, next())
	if cmd != nil || m.err != nil || m.tmuxMissing || m.sessions[0].Pane.PaneID != "%2" {
		t.Fatal("recovery refresh did not restore current sessions")
	}
}

func TestRefreshBeforeKillCannotRestoreKilledPane(t *testing.T) {
	backend := &fakeBackend{panes: []tmux.Pane{{PaneID: "%1"}}}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	first := m.doRefresh()
	oldSnapshot := first()
	m, _ = updateModel(t, m, killPaneMsg{paneID: "%1"})
	m, next := updateModel(t, m, oldSnapshot)
	if len(m.sessions) != 0 || next == nil {
		t.Fatal("pre-kill snapshot restored killed pane or lost follow-up refresh")
	}
}

func TestInitialAndTickRefreshUseGate(t *testing.T) {
	backend := &fakeBackend{panes: []tmux.Pane{{PaneID: "%1"}}}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	batch := m.Init()().(tea.BatchMsg)
	if len(batch) != 2 {
		t.Fatal("initial command must schedule polling and a refresh request")
	}
	m, first := updateModel(t, m, batch[1]())
	if first == nil || !m.refreshing {
		t.Fatal("initial refresh bypassed gate")
	}
	m.composer.active = true
	m.composer.draft = "keep this draft"
	m, polling := updateModel(t, m, tickMsg(time.Now()))
	if polling == nil || !m.refreshQueued || m.composer.draft != "keep this draft" {
		t.Fatal("tick must retain polling, queue refresh and preserve modal state")
	}
	m, next := updateModel(t, m, first())
	if next == nil || !m.composer.active || m.composer.draft != "keep this draft" {
		t.Fatal("refresh completion lost queued refresh or modal state")
	}
}

func TestRefreshBeforeRenameCannotRestoreOldName(t *testing.T) {
	backend := &lifecycleFake{fakeBackend: fakeBackend{panes: []tmux.Pane{{PaneID: "%1"}}}}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	first := m.doRefresh()
	oldSnapshot := first()
	m.lifecycle = lifecycleForm{kind: lifecycleRename, paneID: "%1", text: "renamed", busy: true}
	m, _ = updateModel(t, m, lifecycleMsg{})
	m, next := updateModel(t, m, oldSnapshot)
	if m.sessions[0].Pane.WindowName != "renamed" || next == nil {
		t.Fatal("pre-rename capture replaced new name or lost follow-up refresh")
	}
}
