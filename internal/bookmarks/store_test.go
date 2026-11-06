package bookmarks

import (
	"os"
	"path/filepath"
	"testing"
)

const captureID = "0123456789abcdef0123456789abcdef"

func TestSetLoadsPrivateBookmarkReferencesAndRemovesThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "bookmarks.json")
	entry := Entry{CaptureID: captureID, BlockID: 3}
	if err := Set(path, entry, true); err != nil {
		t.Fatal(err)
	}
	entries, err := Load(path)
	if err != nil || len(entries) != 1 || entries[0] != entry {
		t.Fatalf("bookmark round trip: %#v, %v", entries, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("bookmark file mode: %v, %v", info, err)
	}
	if err := Set(path, entry, true); err != nil {
		t.Fatal(err)
	}
	if entries, err = Load(path); err != nil || len(entries) != 1 {
		t.Fatalf("duplicate bookmark was stored: %#v, %v", entries, err)
	}
	if err := Set(path, entry, false); err != nil {
		t.Fatal(err)
	}
	if entries, err = Load(path); err != nil || len(entries) != 0 {
		t.Fatalf("bookmark removal failed: %#v, %v", entries, err)
	}
}

func TestBookmarkRejectsInvalidCaptureAndCorruptStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.json")
	if err := Set(path, Entry{CaptureID: "../../elsewhere", BlockID: 1}, true); err == nil {
		t.Fatal("invalid capture ID was accepted")
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"bookmarks":[{"capture_id":"../../elsewhere","block_id":1}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("invalid stored capture ID was accepted")
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"version":1,"bookmarks":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("duplicate JSON key was accepted")
	}
}

func TestDefaultPathRequiresAbsoluteXDGDataHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "relative")
	if _, err := DefaultPath(); err == nil {
		t.Fatal("relative XDG_DATA_HOME accepted")
	}
}
