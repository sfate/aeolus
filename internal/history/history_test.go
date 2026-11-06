package history

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExplicitHistoryRoundtripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "history.json")
	entries, err := Load(path)
	if err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
	status := 7
	want := Entry{Command: "printf '空\\n'", Directory: "/tmp/project", ExitCode: &status, Duration: time.Second}
	if _, err := Append(path, want); err != nil {
		t.Fatal(err)
	}
	entries, err = Load(path)
	if err != nil || len(entries) != 1 || entries[0].Command != want.Command || *entries[0].ExitCode != 7 || entries[0].SavedAt.IsZero() {
		t.Fatalf("roundtrip %#v %v", entries, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("file not private", err)
	}
}

func TestHistoryRejectsCorruptionWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	for _, data := range []string{`null`, `{}`, `{"version":2,"entries":[]}`, `{"version":1,"entries":null}`, `{"version":1,"version":1,"entries":[]}`, `{"version":1,"entries":[],"other":0}`, `{"version":1,"entries":[{}]}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %s", data)
		}
		if _, err := Append(path, Entry{Command: "echo test"}); err == nil {
			t.Fatalf("overwrote corrupt %s", data)
		}
		after, _ := os.ReadFile(path)
		if string(after) != data {
			t.Fatal("corrupt history overwritten")
		}
	}
}

func TestHistoryValidationAndRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	for _, entry := range []Entry{{Command: ""}, {Command: "bad\x1b"}, {Command: "ok", Duration: -1}} {
		if _, err := Append(path, entry); err == nil {
			t.Fatal("invalid entry accepted")
		}
	}
	for i := 0; i < MaxEntries+2; i++ {
		if _, err := Append(path, Entry{Command: "echo test"}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := Load(path)
	if err != nil || len(entries) != MaxEntries {
		t.Fatal("retention failed", err)
	}
}

func TestHistoryLockAndBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if _, err := Append(path, Entry{Command: "echo test"}); err == nil {
		t.Fatal("busy writer accepted")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if _, err := Append(path, Entry{Command: strings.Repeat("x", 16385)}); err == nil {
		t.Fatal("oversized command accepted")
	}
	if err := os.WriteFile(path, make([]byte, (1<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("oversized history accepted")
	}
}
