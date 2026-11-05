package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

func supportedProvider(value string) bool {
	return value == "claude" || value == "codex" || value == "gemini"
}

func decodeProjects(raw json.RawMessage) ([]Project, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '[' {
		return nil, fmt.Errorf("projects must be an array")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("projects: %w", err)
	}
	projects := make([]Project, 0, len(entries))
	names := map[string]bool{}
	for i, entry := range entries {
		project, err := decodeProject(entry)
		if err != nil {
			return nil, fmt.Errorf("projects[%d]: %w", i, err)
		}
		if names[project.Name] {
			return nil, fmt.Errorf("projects[%d]: duplicate project name %q", i, project.Name)
		}
		names[project.Name] = true
		projects = append(projects, project)
	}
	return projects, nil
}

func decodeProject(raw json.RawMessage) (Project, error) {
	var project Project
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil {
		return project, err
	}
	if token != json.Delim('{') {
		return project, fmt.Errorf("project must be an object")
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return project, err
		}
		key := token.(string)
		if seen[key] {
			return project, fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = true
		var value string
		var rawValue json.RawMessage
		if err = d.Decode(&rawValue); err != nil {
			return project, err
		}
		if bytes.Equal(bytes.TrimSpace(rawValue), []byte("null")) {
			return project, fmt.Errorf("%s must be a string", key)
		}
		if err = json.Unmarshal(rawValue, &value); err != nil {
			return project, fmt.Errorf("%s must be a string", key)
		}
		for _, r := range value {
			if unicode.IsControl(r) {
				return project, fmt.Errorf("%s cannot contain control characters", key)
			}
		}
		switch key {
		case "name":
			project.Name = value
		case "directory":
			project.Directory = value
		case "provider":
			project.Provider = value
		default:
			return project, fmt.Errorf("unknown field %q", key)
		}
	}
	if _, err = d.Token(); err != nil {
		return project, err
	}
	if strings.TrimSpace(project.Name) == "" || len([]rune(project.Name)) > 80 {
		return project, fmt.Errorf("name must contain 1 to 80 characters")
	}
	if !filepath.IsAbs(project.Directory) {
		return project, fmt.Errorf("directory must be an absolute path")
	}
	if !supportedProvider(project.Provider) {
		return project, fmt.Errorf("provider must be claude, codex, or gemini")
	}
	return project, nil
}
