package workflows

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestStoreRoundTripPrivateAndRetainsAdditions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "workflows.json")
	entries, err := Load(path)
	if err != nil || len(entries) != 0 {
		t.Fatalf("missing store %v %v", entries, err)
	}
	if err := Save(path, "Find", "rg {{pattern}} {{path}}"); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, "Status", "git status --short"); err != nil {
		t.Fatal(err)
	}
	entries, err = Load(path)
	if err != nil || len(entries) != 2 || entries[0].Name != "Find" || entries[1].Name != "Status" {
		t.Fatalf("entries %v %v", entries, err)
	}
	for _, name := range []string{path, path + ".lock"} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("permissions %s %v %v", name, info, err)
		}
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("directory permissions %v %v", info, err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, "Find", "echo different"); err == nil {
		t.Fatal("duplicate name overwrote workflow")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("duplicate changed store")
	}
}

func TestLoadRejectsCorruptSchemas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflows.json")
	for _, data := range []string{
		`null`, `{}`, `{"version":2,"workflows":[]}`, `{"version":1,"workflows":null}`, `{"version":1,"workflows":[] "oops":1}`,
		`{"version":1,"version":1,"workflows":[]}`, `{"version":1,"workflows":[],"extra":true}`, `{"version":1,"workflows":[]} {}`,
		`{"version":1,"workflows":[{"name":"x","name":"y","template":"echo hi"}]}`,
		`{"version":1,"workflows":[{"name":"x","template":"echo hi","extra":true}]}`,
		`{"version":1,"workflows":[{"name":"x","template":"echo hi"},{"name":"x","template":"echo bye"}]}`,
		`{"version":1,"workflows":[{"name":"x","template":null}]}`, `{"version":1,"workflows":[{"name":"x","template":"echo \"{{arg}}\""}]}`,
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", MaxFileBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("oversized store accepted")
	}
}

func TestStoreBusyInvalidInputAndDataHome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflows.json")
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	if err := Save(path, "Busy", "echo literal"); err == nil {
		t.Fatal("busy save accepted")
	}
	for _, name := range []string{"", " surrounding ", "\x1b", strings.Repeat("a", 81)} {
		if err := Save(path, name, "echo literal"); err == nil {
			t.Fatalf("invalid name %q accepted", name)
		}
	}
	t.Setenv("XDG_DATA_HOME", "relative")
	if _, err := DefaultPath(); err == nil {
		t.Fatal("relative data home accepted")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if got, err := DefaultPath(); err != nil || got != filepath.Join(os.Getenv("XDG_DATA_HOME"), "aeolus", "workflows.json") {
		t.Fatalf("default path %q %v", got, err)
	}
}

func TestCorruptLibraryCannotBeOverwrittenAndNonregularPathsFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflows.json")
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, "Valid", "echo hi"); err == nil {
		t.Fatal("corrupt library replaced")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "corrupt" {
		t.Fatal("corrupt data lost")
	}
	if err := os.WriteFile(path, []byte{0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(fifo); err == nil {
		t.Fatal("FIFO library accepted")
	}
	if err := syscall.Mkfifo(fifo+".lock", 0600); err != nil {
		t.Fatal(err)
	}
	if err := Save(fifo, "Valid", "echo hi"); err == nil {
		t.Fatal("FIFO lock accepted")
	}
}
