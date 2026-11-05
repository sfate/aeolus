package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	for _, tt := range []struct {
		name, text, wantErr string
		want                time.Duration
	}{
		{"valid", `{"version":1,"refresh_interval":"500ms"}`, "", 500 * time.Millisecond},
		{"defaults", `{"version":1}`, "", 2 * time.Second},
		{"unknown", `{"version":1,"typo":true}`, "unknown", 0},
		{"version", `{"version":2}`, "version", 0},
		{"missing version", `{}`, "version", 0},
		{"null", `null`, "object", 0},
		{"null interval", `{"version":1,"refresh_interval":null}`, "string", 0},
		{"range", `{"version":1,"refresh_interval":"1ms"}`, "100ms", 0},
		{"long", `{"version":1,"refresh_interval":"2h"}`, "1m", 0},
		{"trailing", `{"version":1} {}`, "trailing", 0},
		{"duplicate", `{"version":1,"version":1}`, "duplicate", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tt.text), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := Load(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error=%v want %s", err, tt.wantErr)
				}
				return
			}
			if err != nil || got.RefreshInterval != tt.want {
				t.Fatalf("got=%v err=%v", got, err)
			}
		})
	}
}

func TestDefaultAndExplicitMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got, err := Load("")
	if err != nil || got.RefreshInterval != 2*time.Second {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("explicit missing file accepted")
	}
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "aeolus")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config.json"), []byte(`{"version":1,"refresh_interval":"3s"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = Load("")
	if err != nil || got.RefreshInterval != 3*time.Second {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
