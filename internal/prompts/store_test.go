package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSaveLoadPrivate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private", "prompts.json")
	entries, err := Load(p)
	if err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	if err = Save(p, "猫 prompt", "Enter Escape\ntext"); err != nil {
		t.Fatal(err)
	}
	entries, err = Load(p)
	if err != nil || len(entries) != 1 || entries[0].Text != "Enter Escape\ntext" {
		t.Fatal(entries, err)
	}
	info, _ := os.Stat(p)
	dir, _ := os.Stat(filepath.Dir(p))
	if info.Mode().Perm() != 0600 || dir.Mode().Perm() != 0700 {
		t.Fatal("permissions")
	}
	before, _ := os.ReadFile(p)
	if Save(p, "猫 prompt", "overwrite") == nil {
		t.Fatal("duplicate accepted")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("clobbered")
	}
}
func TestInvalidStoreAndInput(t *testing.T) {
	p := filepath.Join(t.TempDir(), "prompts.json")
	for _, raw := range []string{`{"version":2,"prompts":[]}`, `{"version":1,"prompts":[],"unknown":true}`, `{"version":1,"prompts":[]} {}`, `{"version":1,"prompts":[{"name":"same","text":"a"},{"name":"same","text":"b"}]}`, strings.Repeat("x", MaxFileBytes+1)} {
		if err := os.WriteFile(p, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Fatal("accepted invalid store")
		}
		if Save(p, "new", "text") == nil {
			t.Fatal("overwrote invalid store")
		}
		after, _ := os.ReadFile(p)
		if string(after) != raw {
			t.Fatal("clobbered invalid store")
		}
	}
	for _, e := range []Entry{{"", "text"}, {"bad\nname", "text"}, {"name", ""}, {"name", "bad\x1b"}, {"name", strings.Repeat("a", MaxTextBytes+1)}} {
		if validateEntry(e) == nil {
			t.Fatalf("accepted %+v", e)
		}
	}
}

func TestStrictSchemaAndBusyLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "prompts.json")
	for _, raw := range []string{`{"version":1}`, `{"version":1,"prompts":null}`, `{"version":1,"version":1,"prompts":[]}`, `{"version":1,"prompts":[{"name":"a","name":"b","text":"c"}]}`} {
		if err := os.WriteFile(p, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := Save(p, "first", "text"); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(p+".lock", os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if err = Save(p, "second", "text"); err == nil {
		t.Fatal("ignored writer lock")
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if err = Save(p, "second", "text"); err != nil {
		t.Fatal(err)
	}
	entries, err := Load(p)
	if err != nil || len(entries) != 2 {
		t.Fatal(entries, err)
	}
}
func TestDefaultPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p, err := DefaultPath()
	if err != nil || p != filepath.Join(os.Getenv("XDG_DATA_HOME"), "aeolus", "prompts.json") {
		t.Fatal(p, err)
	}
	t.Setenv("XDG_DATA_HOME", "relative")
	if _, err = DefaultPath(); err == nil {
		t.Fatal("relative data home accepted")
	}
}
