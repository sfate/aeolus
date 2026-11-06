package config

import (
	"strings"
	"testing"
)

func TestThemeConfigurationAndDefaults(t *testing.T) {
	for _, name := range []string{"dark", "light", "monochrome"} {
		got, err := decode(strings.NewReader(`{"version":1,"theme":"` + name + `"}`))
		if err != nil || got.Theme != name {
			t.Fatalf("theme %q: %#v %v", name, got, err)
		}
	}
	got, err := decode(strings.NewReader(`{"version":1,"refresh_interval":"3s","default_provider":"codex","projects":[]}`))
	if err != nil || got.Theme != "dark" || got.DefaultProvider != "codex" {
		t.Fatalf("old configuration %#v %v", got, err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got, err = Load("")
	if err != nil || got.Theme != "dark" {
		t.Fatalf("missing default config %#v %v", got, err)
	}
}

func TestThemeConfigurationRejectsInvalidValues(t *testing.T) {
	for _, text := range []string{`{"version":1,"theme":null}`, `{"version":1,"theme":1}`, `{"version":1,"theme":[]}`, `{"version":1,"theme":""}`, `{"version":1,"theme":"Dark"}`, `{"version":1,"theme":"custom"}`, `{"version":1,"theme":"light","theme":"dark"}`} {
		if _, err := decode(strings.NewReader(text)); err == nil {
			t.Fatalf("accepted %s", text)
		}
	}
}
