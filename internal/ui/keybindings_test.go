package ui

import (
	"github.com/sfate/aeolus/internal/monitor"
	"testing"
)

func TestRemappedLaunchAndRemovedDefault(t *testing.T) {
	m := NewModelWithBackend(&lifecycleFake{}, &fakeMonitor{})
	m.keybindings = map[string]string{"launch": "L"}
	m = key(m, t, "c")
	if m.lifecycle.kind != lifecycleNone {
		t.Fatal("old default still active")
	}
	m = key(m, t, "L")
	if m.lifecycle.kind != lifecycleLaunch {
		t.Fatal("new binding inactive")
	}
	m = key(m, t, "L")
	if m.lifecycle.text == "" {
		t.Fatal("remapped key not text in form")
	}
}

func TestKeybindingsCannotEscapeComposerOrFilters(t *testing.T) {
	m := NewModelWithBackend(&lifecycleFake{}, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	m.keybindings = map[string]string{"launch": "L"}
	m = key(m, t, "i")
	m = key(m, t, "L")
	if m.composer.draft != "L" || m.lifecycle.kind != lifecycleNone {
		t.Fatal("binding escaped composer")
	}
}
