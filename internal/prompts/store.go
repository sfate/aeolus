// Package prompts stores reusable user-authored prompt text.
package prompts

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

const (
	MaxFileBytes = 1024 * 1024
	MaxTextBytes = 64 * 1024
	maxEntries   = 128
)

type Entry struct {
	Name string `json:"name"`
	Text string `json:"text"`
}
type library struct {
	Version int     `json:"version"`
	Prompts []Entry `json:"prompts"`
}

func DefaultPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base != "" {
		if !filepath.IsAbs(base) {
			return "", fmt.Errorf("XDG_DATA_HOME must be absolute")
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "aeolus", "prompts.json"), nil
}
func validateEntry(e Entry) error {
	if !utf8.ValidString(e.Name) || len([]rune(e.Name)) > 80 || strings.TrimSpace(e.Name) == "" || strings.TrimSpace(e.Name) != e.Name {
		return fmt.Errorf("prompt name must contain 1-80 characters without surrounding whitespace")
	}
	for _, r := range e.Name {
		if unicode.IsControl(r) {
			return fmt.Errorf("prompt name contains a control character")
		}
	}
	if !utf8.ValidString(e.Text) || len(e.Text) > MaxTextBytes || strings.TrimSpace(e.Text) == "" {
		return fmt.Errorf("prompt text must be nonempty UTF-8 up to 64 KiB")
	}
	for _, r := range e.Text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return fmt.Errorf("prompt text contains unsupported controls")
		}
	}
	return nil
}
func validate(entries []Entry) error {
	if len(entries) > maxEntries {
		return fmt.Errorf("library exceeds 128 prompts")
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if err := validateEntry(e); err != nil {
			return err
		}
		if seen[e.Name] {
			return fmt.Errorf("prompt name %q already exists", e.Name)
		}
		seen[e.Name] = true
	}
	return nil
}
func Load(path string) ([]Entry, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open prompt library: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("prompt library must be a regular file")
	}
	if info.Size() > MaxFileBytes {
		return nil, fmt.Errorf("prompt library exceeds 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("prompt library exceeds 1 MiB")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("prompt library is not UTF-8")
	}
	if err = checkJSONValue(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return nil, fmt.Errorf("decode prompt library: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value library
	if err = decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode prompt library: %w", err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("prompt library must contain one JSON object")
	}
	if value.Prompts == nil {
		return nil, fmt.Errorf("prompt library must contain a prompts array")
	}
	if value.Version != 1 {
		return nil, fmt.Errorf("unsupported prompt library version %d", value.Version)
	}
	if err = validate(value.Prompts); err != nil {
		return nil, err
	}
	return value.Prompts, nil
}
func Save(path, name, text string) error {
	entry := Entry{Name: name, Text: text}
	if err := validateEntry(entry); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create prompt directory: %w", err)
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return fmt.Errorf("open prompt lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	info, err := lock.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("prompt lock must be regular")
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("prompt library is busy; retry after the other save finishes: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	entries, err := Load(path)
	if err != nil {
		return err
	}
	entries = append(entries, entry)
	if err = validate(entries); err != nil {
		return err
	}
	data, err := json.MarshalIndent(library{Version: 1, Prompts: entries}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > MaxFileBytes {
		return fmt.Errorf("prompt library exceeds 1 MiB")
	}
	temp, err := os.CreateTemp(dir, ".prompts-*")
	if err != nil {
		return fmt.Errorf("create prompt file: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = temp.Close(); _ = os.Remove(tempPath) }()
	if _, err = temp.Write(data); err != nil {
		return fmt.Errorf("write prompt file: %w", err)
	}
	if err = temp.Sync(); err != nil {
		return fmt.Errorf("sync prompt file: %w", err)
	}
	if err = temp.Close(); err != nil {
		return fmt.Errorf("close prompt file: %w", err)
	}
	if err = os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace prompt library: %w", err)
	}
	return nil
}

// checkJSONValue rejects duplicate object fields before normal schema decoding.
func checkJSONValue(d *json.Decoder, depth int) error {
	if depth > 16 {
		return fmt.Errorf("JSON nesting is too deep")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("invalid object field")
			}
			if seen[name] {
				return fmt.Errorf("duplicate field %q", name)
			}
			seen[name] = true
			if err = checkJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := checkJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	_, err = d.Token()
	return err
}
