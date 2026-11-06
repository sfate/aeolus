package workflows

import (
	"bytes"
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

const MaxFileBytes = 1 << 20

type Entry struct {
	Name     string `json:"name"`
	Template string `json:"template"`
}

func DefaultPath() (string, error) {
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("XDG_DATA_HOME must be an absolute path")
	}
	return filepath.Join(root, "aeolus", "workflows.json"), nil
}

func validateEntry(entry Entry) error {
	if !utf8.ValidString(entry.Name) || len([]rune(entry.Name)) > 80 || strings.TrimSpace(entry.Name) == "" || strings.TrimSpace(entry.Name) != entry.Name {
		return fmt.Errorf("workflow name must contain 1-80 characters without surrounding whitespace")
	}
	for _, r := range entry.Name {
		if unicode.IsControl(r) {
			return fmt.Errorf("workflow name contains controls")
		}
	}
	_, err := Parameters(entry.Template)
	return err
}

func validate(entries []Entry) error {
	if len(entries) > 128 {
		return fmt.Errorf("workflow library exceeds 128 entries")
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if err := validateEntry(entry); err != nil {
			return err
		}
		if seen[entry.Name] {
			return fmt.Errorf("workflow name %q already exists", entry.Name)
		}
		seen[entry.Name] = true
	}
	return nil
}

func object(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("expected a JSON object")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("invalid field")
		}
		if _, exists := fields[name]; exists {
			return nil, fmt.Errorf("duplicate field %q", name)
		}
		known := false
		for _, candidate := range allowed {
			if name == candidate {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("unknown field %q", name)
		}
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return nil, err
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("field %q cannot be null", name)
		}
		fields[name] = raw
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("unexpected trailing JSON content")
	}
	return fields, nil
}

func Load(path string) ([]Entry, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open workflow library: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
		return nil, fmt.Errorf("workflow library must be a regular file no larger than 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileBytes || !utf8.Valid(data) {
		return nil, fmt.Errorf("workflow library encoding or size is invalid")
	}
	fields, err := object(data, "version", "workflows")
	if err != nil {
		return nil, err
	}
	var version int
	if json.Unmarshal(fields["version"], &version) != nil || version != 1 {
		return nil, fmt.Errorf("workflow library version must be 1")
	}
	var rawEntries []json.RawMessage
	if json.Unmarshal(fields["workflows"], &rawEntries) != nil || rawEntries == nil {
		return nil, fmt.Errorf("workflow library requires a workflows array")
	}
	if len(rawEntries) > 128 {
		return nil, fmt.Errorf("workflow library exceeds 128 entries")
	}
	entries := make([]Entry, 0, len(rawEntries))
	for _, raw := range rawEntries {
		fields, err := object(raw, "name", "template")
		if err != nil {
			return nil, err
		}
		var entry Entry
		if json.Unmarshal(fields["name"], &entry.Name) != nil || json.Unmarshal(fields["template"], &entry.Template) != nil {
			return nil, fmt.Errorf("workflow name and template must be strings")
		}
		entries = append(entries, entry)
	}
	if err := validate(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// Save appends one unique workflow under a nonblocking lock, preserving concurrent additions and replacing the library atomically.
func Save(path, name, template string) error {
	entry := Entry{Name: name, Template: template}
	if err := validateEntry(entry); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("workflow library path must be absolute")
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	info, err := lock.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("workflow lock must be a regular file")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("workflow library is busy: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	entries, err := Load(path)
	if err != nil {
		return err
	}
	entries = append(entries, entry)
	if err := validate(entries); err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		Version   int     `json:"version"`
		Workflows []Entry `json:"workflows"`
	}{1, entries}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > MaxFileBytes {
		return fmt.Errorf("workflow library exceeds 1 MiB")
	}
	file, err := os.CreateTemp(directory, ".workflows-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { _ = file.Close(); _ = os.Remove(temporary) }()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return nil
}
