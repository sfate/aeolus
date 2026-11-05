// Package history stores only explicitly selected command metadata, never output.
package history

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxEntries = 128
const maxFileBytes = 1 << 20

type Entry struct {
	Command   string        `json:"command"`
	Directory string        `json:"directory"`
	ExitCode  *int          `json:"exit_code"`
	Duration  time.Duration `json:"duration_ns"`
	SavedAt   time.Time     `json:"saved_at"`
}

type library struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
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
		return "", fmt.Errorf("XDG_DATA_HOME must be absolute")
	}
	return filepath.Join(root, "aeolus", "history.json"), nil
}

func validate(e Entry) error {
	if strings.TrimSpace(e.Command) == "" || len(e.Command) > 16<<10 || len(e.Directory) > 4<<10 {
		return fmt.Errorf("history command must be nonempty and at most 16 KiB; directory at most 4 KiB")
	}
	for _, text := range []string{e.Command, e.Directory} {
		if !utf8.ValidString(text) {
			return fmt.Errorf("history text must be UTF-8")
		}
		for _, r := range text {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return fmt.Errorf("history text contains terminal controls")
			}
		}
	}
	if e.Duration < 0 || (e.ExitCode != nil && (*e.ExitCode < 0 || *e.ExitCode > 255)) {
		return fmt.Errorf("invalid command status or duration")
	}
	return nil
}

// uniqueJSON validates duplicate keys before ordinary schema decoding.
func uniqueJSON(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate or invalid JSON key")
			}
			seen[name] = true
			if err := uniqueJSON(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSON(d); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	_, err = d.Token()
	return err
}

func Load(path string) ([]Entry, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxFileBytes {
		return nil, fmt.Errorf("history must be a regular file no larger than 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileBytes || !utf8.Valid(data) {
		return nil, fmt.Errorf("invalid history size or encoding")
	}
	check := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueJSON(check); err != nil {
		return nil, err
	}
	var extra any
	if check.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("trailing history content")
	}
	var raw struct {
		Version int               `json:"version"`
		Entries []json.RawMessage `json:"entries"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	if raw.Version != 1 || raw.Entries == nil || len(raw.Entries) > MaxEntries {
		return nil, fmt.Errorf("unsupported history version or invalid entries")
	}
	entries := make([]Entry, 0, len(raw.Entries))
	for _, item := range raw.Entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil || len(fields) != 5 {
			return nil, fmt.Errorf("history entry requires all five fields")
		}
		for _, key := range []string{"command", "directory", "exit_code", "duration_ns", "saved_at"} {
			value, ok := fields[key]
			if !ok || (key != "exit_code" && string(value) == "null") {
				return nil, fmt.Errorf("invalid history field %s", key)
			}
		}
		var entry Entry
		if err := json.Unmarshal(item, &entry); err != nil {
			return nil, err
		}
		if err := validate(entry); err != nil {
			return nil, err
		}
		if entry.SavedAt.IsZero() {
			return nil, fmt.Errorf("history timestamp is required")
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Append persists one explicit save, returning how many oldest entries were evicted.
// It reloads under an exclusive lock so another dashboard cannot lose a saved entry.
func Append(path string, entry Entry) (int, error) {
	if err := validate(entry); err != nil {
		return 0, err
	}
	if !filepath.IsAbs(path) {
		return 0, fmt.Errorf("history path must be absolute")
	}
	entry.SavedAt = time.Now().UTC()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return 0, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return 0, err
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return 0, fmt.Errorf("history is busy: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	entries, err := Load(path)
	if err != nil {
		return 0, err
	}
	entries = append(entries, entry)
	evicted := 0
	if len(entries) > MaxEntries {
		evicted = len(entries) - MaxEntries
		entries = entries[evicted:]
	}
	var data []byte
	for {
		data, err = json.MarshalIndent(library{1, entries}, "", "  ")
		if err != nil {
			return 0, err
		}
		if len(data)+1 <= maxFileBytes {
			break
		}
		entries = entries[1:]
		evicted++
	}
	file, err := os.CreateTemp(dir, ".history-*")
	if err != nil {
		return 0, err
	}
	name := file.Name()
	defer func() { _ = file.Close(); _ = os.Remove(name) }()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return 0, err
	}
	if err := file.Sync(); err != nil {
		return 0, err
	}
	if err := file.Close(); err != nil {
		return 0, err
	}
	return evicted, os.Rename(name, path)
}
