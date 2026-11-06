// Package preferences stores explicitly saved dashboard filters.
package preferences

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

const maxBytes = 16 << 10

type Snapshot struct{ Provider, Status, Directory string }

func DefaultPath() (string, error) {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("XDG_STATE_HOME must be an absolute path")
	}
	return filepath.Join(root, "aeolus", "dashboard.json"), nil
}

func validate(s Snapshot) error {
	switch s.Provider {
	case "", "all", "claude", "codex", "gemini", "unknown", "shell":
	default:
		return fmt.Errorf("invalid saved provider")
	}

	switch s.Status {
	case "", "all", "unknown", "idle", "working", "permission", "question":
	default:
		return fmt.Errorf("invalid saved status")
	}
	if len(s.Directory) > 4096 || !utf8.ValidString(s.Directory) {
		return fmt.Errorf("invalid saved directory filter")
	}
	for _, r := range s.Directory {
		if unicode.IsControl(r) {
			return fmt.Errorf("directory filter cannot contain controls")
		}
	}
	return nil
}

func Load(path string) (Snapshot, error) {
	var s Snapshot
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return s, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return s, fmt.Errorf("preferences must be a regular file no larger than 16 KiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return s, err
	}
	if len(data) > maxBytes || !utf8.Valid(data) {
		return s, fmt.Errorf("invalid preferences encoding or size")
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return s, fmt.Errorf("preferences must be a JSON object")
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return s, err
		}
		key := token.(string)
		if seen[key] {
			return s, fmt.Errorf("duplicate preferences field %q", key)
		}
		seen[key] = true
		var raw json.RawMessage
		if err = d.Decode(&raw); err != nil {
			return s, err
		}
		if string(raw) == "null" {
			return s, fmt.Errorf("preferences fields cannot be null")
		}
		switch key {
		case "version":
			var v int
			if json.Unmarshal(raw, &v) != nil || v != 1 {
				return s, fmt.Errorf("preferences version must be 1")
			}
		case "provider":
			err = json.Unmarshal(raw, &s.Provider)
		case "status":
			err = json.Unmarshal(raw, &s.Status)
		case "directory":
			err = json.Unmarshal(raw, &s.Directory)
		default:
			return s, fmt.Errorf("unknown preferences field %q", key)
		}
		if err != nil {
			return s, err
		}
	}
	if _, err = d.Token(); err != nil {
		return s, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF || !seen["version"] {
		return s, fmt.Errorf("missing version or trailing preferences content")
	}
	return s, validate(s)
}

// Save atomically replaces one complete filter snapshot. Concurrent saves fail busy.
func Save(path string, s Snapshot) error {
	if err := validate(s); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("preferences path must be absolute")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("preferences are busy: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	data, err := json.MarshalIndent(struct {
		Version   int    `json:"version"`
		Provider  string `json:"provider"`
		Status    string `json:"status"`
		Directory string `json:"directory"`
	}{1, s.Provider, s.Status, s.Directory}, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".dashboard-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() { _ = file.Close(); _ = os.Remove(name) }()
	if _, err = file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
