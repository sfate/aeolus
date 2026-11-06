package shellblocks

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unicode/utf8"
)

const MaxCaptureBytes = 8 << 20

type Capture struct{ ID, Directory string }
type captureSnapshot struct {
	Version int     `json:"version"`
	Blocks  []Block `json:"blocks"`
}

// IsValidCaptureID accepts only opaque lowercase random identifiers, never paths.
func IsValidCaptureID(id string) bool { return validToken(id) }

func runtimeDirectory(id string) (string, error) {
	if !IsValidCaptureID(id) {
		return "", fmt.Errorf("invalid capture ID")
	}
	root, err := filepath.Abs(os.TempDir())
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "aeolus-blocks-"+id), nil
}

func privateDirectory(id string) (string, error) {
	dir, err := runtimeDirectory(id)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("capture is unavailable: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode().Perm() != 0700 || !ok || stat.Uid != uint32(os.Geteuid()) {
		return "", fmt.Errorf("capture directory must be private and owned by the current user")
	}
	return dir, nil
}

// PrepareCapture writes private, temporary zsh bootstrap files. It does not launch
// a shell or enable a terminal output tap.
func PrepareCapture() (Capture, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Capture{}, err
	}
	id := hex.EncodeToString(random[:])
	dir, err := runtimeDirectory(id)
	if err != nil {
		return Capture{}, err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return Capture{}, err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(dir)
		}
	}()
	base64Path, err := exec.LookPath("base64")
	if err != nil {
		return Capture{}, fmt.Errorf("capture needs base64 on PATH: %w", err)
	}
	trPath, err := exec.LookPath("tr")
	if err != nil {
		return Capture{}, fmt.Errorf("capture needs tr on PATH: %w", err)
	}
	base64Path, err = filepath.Abs(base64Path)
	if err != nil {
		return Capture{}, err
	}
	trPath, err = filepath.Abs(trPath)
	if err != nil {
		return Capture{}, err
	}
	hooks, err := ZshHooks(id, base64Path, trPath)
	if err != nil {
		return Capture{}, err
	}
	env, rc, err := zshBootstrap(dir, id)
	if err != nil {
		return Capture{}, err
	}
	for name, text := range map[string]string{"hooks.zsh": hooks, ".zshenv": env, ".zshrc": rc, "ready": "pending"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			return Capture{}, err
		}
	}
	complete = true
	return Capture{ID: id, Directory: dir}, nil
}

// CaptureReady confirms that the private bootstrap loaded its hook functions.
func CaptureReady(id string) bool {
	dir, err := privateDirectory(id)
	if err != nil {
		return false
	}
	f, err := os.OpenFile(filepath.Join(dir, "ready"), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(f, 8))
	return err == nil && string(data) == "ready"
}

// DiscardCapture removes only a validated private runtime directory.
func DiscardCapture(id string) error {
	dir, err := privateDirectory(id)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func writeSnapshot(dir string, p *Parser) error {
	data, err := json.Marshal(captureSnapshot{Version: 1, Blocks: append([]Block{}, p.Snapshot()...)})
	if err != nil {
		return err
	}
	if len(data) > MaxCaptureBytes {
		return fmt.Errorf("capture exceeds snapshot limit")
	}
	f, err := os.CreateTemp(dir, "snapshot-*")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, "snapshot.json"))
}

// CollectCapture is the hidden helper's bounded output sink. Runtime files are
// removed when its input closes. Persistent history is never written here.
func CollectCapture(id string, input io.Reader) error {
	dir, err := privateDirectory(id)
	if err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "collector.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("capture collector is already active or unavailable: %w", err)
	}
	_ = lock.Close()
	finished := false
	defer func() {
		if finished {
			_ = DiscardCapture(id)
		}
	}()
	if closer, ok := input.(io.Closer); ok {
		defer func() { _ = closer.Close() }()
	}
	p, _ := NewParser(id)
	if err := writeSnapshot(dir, p); err != nil {
		return err
	}
	type readResult struct {
		data []byte
		err  error
	}
	reads := make(chan readResult, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			buffer := make([]byte, 32<<10)
			n, err := input.Read(buffer)
			select {
			case reads <- readResult{buffer[:n], err}:
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	dirty := false
	for {
		select {
		case result := <-reads:
			if len(result.data) > 0 {
				_, _ = p.Write(result.data)
				dirty = true
			}
			if result.err != nil {
				p.Close()
				if result.err != io.EOF {
					return result.err
				}
				finished = true
				return nil
			}
		case <-ticker.C:
			if dirty {
				if err := writeSnapshot(dir, p); err != nil {
					return err
				}
				dirty = false
			}
		}
	}
}

// ReadCapture reads only the fixed snapshot of a validated opaque capture ID.
func ReadCapture(id string) ([]Block, error) {
	dir, err := privateDirectory(id)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "snapshot.json"), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("capture snapshot unavailable: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > MaxCaptureBytes {
		return nil, fmt.Errorf("invalid capture snapshot file")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxCaptureBytes+1))
	if err != nil || len(data) > MaxCaptureBytes {
		return nil, fmt.Errorf("cannot read bounded capture snapshot")
	}
	var snapshot captureSnapshot
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("invalid capture snapshot: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF || snapshot.Version != 1 || snapshot.Blocks == nil || len(snapshot.Blocks) > MaxBlocks {
		return nil, fmt.Errorf("unsupported or oversized capture snapshot")
	}
	var previous uint64
	for _, b := range snapshot.Blocks {
		if b.ID <= previous || b.Command == "" || len(b.Command) > MaxCommandBytes || !utf8.ValidString(b.Command) || len(b.Directory) > MaxDirectoryBytes || !filepath.IsAbs(b.Directory) || !utf8.ValidString(b.Directory) || len(b.Output) > MaxOutputBytes || b.Duration < 0 || b.StartedAt.IsZero() || b.ExitCode != nil && (*b.ExitCode < 0 || *b.ExitCode > 255 || b.Incomplete) || b.ExitCode == nil && !b.Incomplete {
			return nil, fmt.Errorf("invalid captured block")
		}
		previous = b.ID
	}
	return snapshot.Blocks, nil
}
