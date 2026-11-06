package shellblocks

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaptureLifecycleAndPrivateSnapshots(t *testing.T) {
	capture, err := PrepareCapture()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DiscardCapture(capture.ID) })
	if CaptureReady(capture.ID) {
		t.Fatal("unloaded bootstrap reported ready")
	}
	if err := os.WriteFile(filepath.Join(capture.Directory, "ready"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	if !CaptureReady(capture.ID) {
		t.Fatal("bootstrap readiness not detected")
	}
	info, err := os.Stat(capture.Directory)
	if err != nil || info.Mode().Perm() != 0700 || !IsValidCaptureID(capture.ID) {
		t.Fatal("capture directory is not private")
	}
	r, w := io.Pipe()
	finished := make(chan error, 1)
	go func() { finished <- CollectCapture(capture.ID, r) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := ReadCapture(capture.ID); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("collector did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	frame := startFrame(1, "false") + "result" + endFrame(1, 1)
	frame = strings.ReplaceAll(frame, testToken, capture.ID)
	if _, err := w.Write([]byte(frame)); err != nil {
		t.Fatal(err)
	}
	for {
		blocks, err := ReadCapture(capture.ID)
		if err == nil && len(blocks) == 1 && blocks[0].ExitCode != nil {
			if *blocks[0].ExitCode != 1 || string(blocks[0].Output) != "result" {
				t.Fatalf("snapshot %#v", blocks)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("snapshot not published: %#v %v", blocks, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	fileInfo, err := os.Stat(filepath.Join(capture.Directory, "snapshot.json"))
	if err != nil || fileInfo.Mode().Perm() != 0600 {
		t.Fatal("snapshot not private")
	}
	_ = w.Close()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(capture.Directory); !os.IsNotExist(err) {
		t.Fatalf("runtime not cleaned: %v", err)
	}
}

func TestCaptureRejectsCorruptAndOversizedSnapshots(t *testing.T) {
	capture, err := PrepareCapture()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DiscardCapture(capture.ID) })
	path := filepath.Join(capture.Directory, "snapshot.json")
	for _, data := range [][]byte{[]byte(`{"version":2,"blocks":[]}`), []byte(`{"version":1,"blocks":null}`), []byte(`{"version":1,"blocks":[]} trailing`), []byte("{")} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadCapture(capture.ID); err == nil {
			t.Fatalf("invalid snapshot accepted: %s", data)
		}
	}
	p, _ := NewParser(testToken)
	_, _ = p.Write([]byte(startFrame(1, "test") + endFrame(1, 0)))
	blocks := p.Snapshot()
	blocks[0].Output = make([]byte, MaxOutputBytes+1)
	data, _ := json.Marshal(captureSnapshot{Version: 1, Blocks: blocks})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCapture(capture.ID); err == nil {
		t.Fatal("oversized block accepted")
	}
	if err := os.WriteFile(filepath.Join(capture.Directory, "collector.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := CollectCapture(capture.ID, strings.NewReader("")); err == nil {
		t.Fatal("second collector accepted")
	}
	if _, err := os.Stat(capture.Directory); err != nil {
		t.Fatal("second collector removed active runtime")
	}
}

func TestCaptureRejectsInvalidIDsAndSymlinks(t *testing.T) {
	for _, id := range []string{"", "../escape", "/tmp", "ABCDEF0123456789abcdef0123456789ab"} {
		if IsValidCaptureID(id) {
			t.Fatalf("accepted ID %q", id)
		}
		if _, err := ReadCapture(id); err == nil {
			t.Fatal("invalid ID read allowed")
		}
	}
	capture, err := PrepareCapture()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DiscardCapture(capture.ID) })
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), filepath.Join(capture.Directory, "snapshot.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCapture(capture.ID); err == nil {
		t.Fatal("symlink snapshot accepted")
	}
	if err := os.Remove(filepath.Join(capture.Directory, "ready")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), filepath.Join(capture.Directory, "ready")); err != nil {
		t.Fatal(err)
	}
	if CaptureReady(capture.ID) {
		t.Fatal("symlink readiness accepted")
	}
}
