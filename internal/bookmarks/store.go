// Package bookmarks stores references to captured blocks without copying commands or output.
package bookmarks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sfate/aeolus/internal/shellblocks"
)

const (
	MaxEntries  = 256
	maxFileSize = 64 << 10
)

type Entry struct {
	CaptureID string `json:"capture_id"`
	BlockID   uint64 `json:"block_id"`
}

type library struct {
	Version   int     `json:"version"`
	Bookmarks []Entry `json:"bookmarks"`
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
	return filepath.Join(root, "aeolus", "bookmarks.json"), nil
}

func validate(entry Entry) error {
	if !shellblocks.IsValidCaptureID(entry.CaptureID) || entry.BlockID == 0 {
		return fmt.Errorf("bookmark requires a valid capture ID and block ID")
	}
	return nil
}

func uniqueJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
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
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate or invalid bookmark JSON key")
			}
			seen[name] = true
			if err := uniqueJSON(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueJSON(decoder); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected bookmark JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}

func Load(path string) ([]Entry, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("bookmark path must be absolute")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open bookmarks: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxFileSize {
		return nil, fmt.Errorf("bookmark file must be regular and no larger than %d bytes", maxFileSize)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("bookmark file exceeds size limit")
	}
	check := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueJSON(check); err != nil {
		return nil, err
	}
	if check.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("bookmark file must contain one JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value library
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode bookmarks: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("bookmark file must contain one JSON object")
	}
	if value.Version != 1 || value.Bookmarks == nil || len(value.Bookmarks) > MaxEntries {
		return nil, fmt.Errorf("unsupported bookmark version or invalid entries")
	}
	seen := make(map[Entry]bool, len(value.Bookmarks))
	for _, entry := range value.Bookmarks {
		if err := validate(entry); err != nil {
			return nil, err
		}
		if seen[entry] {
			return nil, fmt.Errorf("duplicate bookmark")
		}
		seen[entry] = true
	}
	return value.Bookmarks, nil
}

// Set atomically adds or removes one reference while preserving concurrent dashboard updates.
func Set(path string, entry Entry, bookmarked bool) error {
	if err := validate(entry); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("bookmark path must be absolute")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return fmt.Errorf("open bookmark lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	info, err := lock.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("bookmark lock must be a regular file")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("bookmark store is busy: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	entries, err := Load(path)
	if err != nil {
		return err
	}
	found := -1
	for i, saved := range entries {
		if saved == entry {
			found = i
			break
		}
	}
	if bookmarked && found < 0 {
		if len(entries) >= MaxEntries {
			return fmt.Errorf("bookmark store is full (%d entries)", MaxEntries)
		}
		entries = append(entries, entry)
	} else if !bookmarked && found >= 0 {
		entries = append(entries[:found], entries[found+1:]...)
	} else {
		return nil
	}
	data, err := json.MarshalIndent(library{Version: 1, Bookmarks: entries}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxFileSize {
		return fmt.Errorf("bookmark store exceeds size limit")
	}
	temp, err := os.CreateTemp(dir, ".bookmarks-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() { _ = temp.Close(); _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
