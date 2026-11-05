// Package config loads versioned local dashboard settings.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const DefaultRefreshInterval = 2 * time.Second

// Config contains validated runtime settings.
type Config struct {
	Keybindings map[string]string

	ProviderArgs    map[string][]string
	RefreshInterval time.Duration
	Projects        []Project
	DefaultProvider string
	Theme           string
}

// Project is a named directory and agent preset used by the launch form.
type Project struct{ Name, Directory, Provider string }

// Load reads an explicit file, or the default XDG configuration when path is empty.
// A missing default file uses defaults; an explicit missing file is an error.
func Load(path string) (Config, error) {
	result := Config{RefreshInterval: DefaultRefreshInterval, DefaultProvider: "claude", Theme: "dark"}
	explicit := path != ""
	if !explicit {
		root := os.Getenv("XDG_CONFIG_HOME")
		if root == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return result, fmt.Errorf("find config directory: %w", err)
			}
			root = filepath.Join(home, ".config")
		}
		if !filepath.IsAbs(root) {
			return result, fmt.Errorf("XDG_CONFIG_HOME must be an absolute path")
		}
		path = filepath.Join(root, "aeolus", "config.json")
	}
	file, err := os.Open(path)
	if err != nil {
		if !explicit && os.IsNotExist(err) {
			return result, nil
		}
		return result, fmt.Errorf("read config %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	parsed, err := decode(file)
	if err != nil {
		return result, fmt.Errorf("config %s: %w", path, err)
	}
	return parsed, nil
}

func decode(r io.Reader) (Config, error) {
	result := Config{RefreshInterval: DefaultRefreshInterval, DefaultProvider: "claude", Theme: "dark"}
	d := json.NewDecoder(r)
	token, err := d.Token()
	if err != nil {
		return result, err
	}
	if token != json.Delim('{') {
		return result, fmt.Errorf("expected a JSON object")
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return result, err
		}
		key := token.(string)
		if seen[key] {
			return result, fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = true
		var raw json.RawMessage
		if err = d.Decode(&raw); err != nil {
			return result, err
		}
		switch key {
		case "theme":
			var value string
			if err = json.Unmarshal(raw, &value); err != nil || (value != "dark" && value != "light" && value != "monochrome") {
				return result, fmt.Errorf("theme must be dark, light, or monochrome")
			}
			result.Theme = value
		case "version":
			var version int
			if string(raw) == "null" {
				return result, fmt.Errorf("version must be 1")
			}
			if err = json.Unmarshal(raw, &version); err != nil || version != 1 {
				return result, fmt.Errorf("version must be 1")
			}
		case "keybindings":
			keys, err := decodeKeys(raw)
			if err != nil {
				return result, err
			}
			result.Keybindings = keys

		case "provider_args":
			args, err := decodeProviderArgs(raw)
			if err != nil {
				return result, err
			}
			result.ProviderArgs = args
		case "default_provider":
			var value string
			if err = json.Unmarshal(raw, &value); err != nil || !supportedProvider(value) {
				return result, fmt.Errorf("default_provider must be claude, codex, or gemini")
			}
			result.DefaultProvider = value
		case "projects":
			projects, err := decodeProjects(raw)
			if err != nil {
				return result, err
			}
			result.Projects = projects
		case "refresh_interval":
			var value string
			if string(raw) == "null" {
				return result, fmt.Errorf("refresh_interval must be a duration string")
			}
			if err = json.Unmarshal(raw, &value); err != nil {
				return result, fmt.Errorf("refresh_interval must be a duration string")
			}
			interval, err := time.ParseDuration(value)
			if err != nil {
				return result, fmt.Errorf("refresh_interval: %w", err)
			}
			if interval < 100*time.Millisecond || interval > time.Minute {
				return result, fmt.Errorf("refresh_interval must be between 100ms and 1m")
			}
			result.RefreshInterval = interval
		default:
			return result, fmt.Errorf("unknown field %q", key)
		}
	}
	if _, err = d.Token(); err != nil {
		return result, err
	}
	if !seen["version"] {
		return result, fmt.Errorf("version is required and must be 1")
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return result, fmt.Errorf("unexpected trailing content")
	}
	return result, nil
}
