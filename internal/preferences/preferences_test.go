package preferences

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestLoadSaveAndPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "dashboard.json")
	got, err := Load(path)
	if err != nil || got != (Snapshot{}) {
		t.Fatalf("missing file: %#v %v", got, err)
	}
	want := Snapshot{Provider: "codex", Status: "unknown", Directory: "空/project"}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil || got != want {
		t.Fatalf("roundtrip %#v %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissions %v %v", info, err)
	}
}

func TestRejectCorruptOrInvalidPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.json")
	for _, data := range []string{`{}`, `null`, `{"version":2}`, `{"version":1,"version":1}`, `{"version":1,"provider":"bad"}`, `{"version":1,"status":null}`, `{"version":1,"extra":0}`, `{"version":1} {}`, `{"version":1,"directory":"\u001b"}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	if err := os.WriteFile(path, make([]byte, 16385), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("oversized state accepted")
	}
}

func TestInvalidSaveDoesNotReplaceExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.json")
	want := Snapshot{Provider: "claude"}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Snapshot{Provider: "invalid"}); err == nil {
		t.Fatal("invalid save")
	}
	got, err := Load(path)
	if err != nil || got != want {
		t.Fatal("existing data lost")
	}
}

func TestBusySaveAndRelativeStateHome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.json")
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	if err := Save(path, Snapshot{}); err == nil {
		t.Fatal("concurrent save did not fail busy")
	}
	t.Setenv("XDG_STATE_HOME", "relative")
	if _, err := DefaultPath(); err == nil {
		t.Fatal("relative XDG state home accepted")
	}
}
