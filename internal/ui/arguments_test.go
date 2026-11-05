package ui

import (
	"github.com/sfate/aeolus/internal/config"
	"github.com/sfate/aeolus/internal/tmux"
	"testing"
)

func TestProviderArgumentConfigurationIsCopied(t *testing.T) {
	settings := config.Config{ProviderArgs: map[string][]string{"claude": {"--profile", "work"}}}
	m := NewModelWithConfig(settings)
	settings.ProviderArgs["claude"][1] = "changed"
	settings.ProviderArgs["codex"] = []string{"changed"}
	backend := m.backend.(tmux.Client)
	if backend.ProviderArgs["claude"][1] != "work" || backend.ProviderArgs["codex"] != nil {
		t.Fatal("launch arguments alias mutable configuration")
	}
}
