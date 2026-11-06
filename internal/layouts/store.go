// Package layouts stores versioned local tmux layout descriptions.
package layouts

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
	MaxLayouts   = 64
	MaxWindows   = 16
	MaxPanes     = 8
	MaxFileBytes = 64 << 10
)

type PaneType string

const (
	Shell PaneType = "shell"
	Agent PaneType = "agent"
)

type Split string

const (
	Horizontal Split = "horizontal"
	Vertical   Split = "vertical"
)

type Pane struct {
	Type        PaneType `json:"type"`
	Provider    string   `json:"provider,omitempty"`
	Split       Split    `json:"split,omitempty"`
	SizePercent int      `json:"size_percent,omitempty"`
}
type Window struct {
	Name  string `json:"name"`
	Panes []Pane `json:"panes"`
}
type Layout struct {
	Name      string   `json:"name"`
	Directory string   `json:"directory"`
	Windows   []Window `json:"windows"`
}
type library struct {
	Version int      `json:"version"`
	Layouts []Layout `json:"layouts"`
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
	return filepath.Join(root, "aeolus", "layouts.json"), nil
}
func validName(s string) bool {
	if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || strings.TrimSpace(s) != s || len([]rune(s)) > 80 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func Validate(l Layout) error {
	if !validName(l.Name) {
		return fmt.Errorf("layout name must contain 1-80 characters without surrounding whitespace")
	}
	if !filepath.IsAbs(l.Directory) || strings.TrimSpace(l.Directory) != l.Directory || !utf8.ValidString(l.Directory) {
		return fmt.Errorf("layout directory must be an absolute path without surrounding whitespace")
	}
	for _, r := range l.Directory {
		if unicode.IsControl(r) {
			return fmt.Errorf("layout directory contains a control character")
		}
	}
	if len(l.Windows) == 0 || len(l.Windows) > MaxWindows {
		return fmt.Errorf("layout must contain 1-%d windows", MaxWindows)
	}
	seen := map[string]bool{}
	for _, w := range l.Windows {
		if !validName(w.Name) {
			return fmt.Errorf("window name must contain 1-80 characters")
		}
		if seen[w.Name] {
			return fmt.Errorf("window name %q is duplicated", w.Name)
		}
		seen[w.Name] = true
		if len(w.Panes) == 0 || len(w.Panes) > MaxPanes {
			return fmt.Errorf("window %q must contain 1-%d panes", w.Name, MaxPanes)
		}
		for i, p := range w.Panes {
			switch p.Type {
			case Shell:
				if p.Provider != "" {
					return fmt.Errorf("shell pane cannot specify a provider")
				}
			case Agent:
				switch p.Provider {
				case "claude", "codex", "gemini":
				default:
					return fmt.Errorf("agent pane has unsupported provider %q", p.Provider)
				}
			default:
				return fmt.Errorf("pane has unsupported type %q", p.Type)
			}
			if i == 0 && p.Split != "" {
				return fmt.Errorf("first pane in window %q cannot specify a split", w.Name)
			}
			if i > 0 && p.Split != Horizontal && p.Split != Vertical {
				return fmt.Errorf("pane %d in window %q requires horizontal or vertical split", i+1, w.Name)
			}
			if p.SizePercent != 0 && (i == 0 || p.SizePercent < 1 || p.SizePercent > 99) {
				return fmt.Errorf("pane size_percent must be 1-99 and apply only to split panes")
			}
		}
	}
	return nil
}

func object(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("expected JSON object")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		t, e = d.Token()
		if e != nil {
			return nil, e
		}
		k := t.(string)
		found := false
		for _, a := range allowed {
			if a == k {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown JSON field %q", k)
		}
		if _, ok := fields[k]; ok {
			return nil, fmt.Errorf("duplicate JSON field %q", k)
		}
		var v json.RawMessage
		if e = d.Decode(&v); e != nil {
			return nil, e
		}
		fields[k] = v
	}
	if _, e = d.Token(); e != nil {
		return nil, e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return nil, fmt.Errorf("unexpected trailing JSON content")
	}
	return fields, nil
}
func Load(path string) ([]Layout, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("layout path must be absolute")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(e) {
		return []Layout{}, nil
	}
	if e != nil {
		return nil, fmt.Errorf("open layouts: %w", e)
	}
	defer func() { _ = f.Close() }()
	i, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !i.Mode().IsRegular() || i.Mode().Perm() != 0600 || i.Size() > MaxFileBytes {
		return nil, fmt.Errorf("layout store must be a private regular file no larger than 64 KiB")
	}
	data, e := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if e != nil || len(data) > MaxFileBytes || !utf8.Valid(data) {
		return nil, fmt.Errorf("layout store exceeds its size limit or is not UTF-8")
	}
	return Parse(data)
}
func Parse(data []byte) ([]Layout, error) {
	if len(data) > MaxFileBytes || !utf8.Valid(data) {
		return nil, fmt.Errorf("layout JSON exceeds 64 KiB or is not UTF-8")
	}
	root, e := object(data, "version", "layouts")
	if e != nil {
		return nil, e
	}
	var v int
	if json.Unmarshal(root["version"], &v) != nil || v != 1 {
		return nil, fmt.Errorf("layout store version must be 1")
	}
	var raws []json.RawMessage
	if json.Unmarshal(root["layouts"], &raws) != nil || raws == nil || len(raws) > MaxLayouts {
		return nil, fmt.Errorf("layout store requires at most %d layouts", MaxLayouts)
	}
	out := make([]Layout, 0, len(raws))
	names := map[string]bool{}
	for _, raw := range raws {
		lf, e := object(raw, "name", "directory", "windows")
		if e != nil {
			return nil, e
		}
		var l Layout
		if json.Unmarshal(lf["name"], &l.Name) != nil || json.Unmarshal(lf["directory"], &l.Directory) != nil {
			return nil, fmt.Errorf("layout name or directory has invalid type")
		}
		var wr []json.RawMessage
		if json.Unmarshal(lf["windows"], &wr) != nil || wr == nil {
			return nil, fmt.Errorf("layout windows must be an array")
		}
		for _, rw := range wr {
			wf, e := object(rw, "name", "panes")
			if e != nil {
				return nil, e
			}
			var w Window
			if json.Unmarshal(wf["name"], &w.Name) != nil {
				return nil, fmt.Errorf("window name has invalid type")
			}
			var pr []json.RawMessage
			if json.Unmarshal(wf["panes"], &pr) != nil || pr == nil {
				return nil, fmt.Errorf("window panes must be an array")
			}
			for _, rp := range pr {
				_, e = object(rp, "type", "provider", "split", "size_percent")
				if e != nil {
					return nil, e
				}
				var p Pane
				if e = json.Unmarshal(rp, &p); e != nil {
					return nil, e
				}
				w.Panes = append(w.Panes, p)
			}
			l.Windows = append(l.Windows, w)
		}
		if e = Validate(l); e != nil {
			return nil, e
		}
		if names[l.Name] {
			return nil, fmt.Errorf("layout name %q is duplicated", l.Name)
		}
		names[l.Name] = true
		out = append(out, l)
	}
	return out, nil
}

func Save(path string, l Layout) error {
	if e := Validate(l); e != nil {
		return e
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("layout path must be absolute")
	}
	dir := filepath.Dir(path)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	lock, e := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if e != nil {
		return e
	}
	defer func() { _ = lock.Close() }()
	info, e := lock.Stat()
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return fmt.Errorf("layout lock must be a private regular file")
	}
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return fmt.Errorf("layout store is busy: %w", e)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	ls, e := Load(path)
	if e != nil {
		return e
	}
	for _, old := range ls {
		if old.Name == l.Name {
			return fmt.Errorf("layout %q already exists", l.Name)
		}
	}
	if len(ls) >= MaxLayouts {
		return fmt.Errorf("layout store is full (%d layouts)", MaxLayouts)
	}
	ls = append(ls, l)
	data, e := json.MarshalIndent(library{1, ls}, "", "  ")
	if e != nil {
		return e
	}
	data = append(data, '\n')
	if len(data) > MaxFileBytes {
		return fmt.Errorf("layout store exceeds 64 KiB")
	}
	tmp, e := os.CreateTemp(dir, ".layouts-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer func() { _ = tmp.Close(); _ = os.Remove(name) }()
	if e = tmp.Chmod(0600); e != nil {
		return e
	}
	if _, e = tmp.Write(data); e != nil {
		return e
	}
	if e = tmp.Sync(); e != nil {
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	return os.Rename(name, path)
}
