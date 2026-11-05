package tmux

import (
	"crypto/rand"
	"fmt"
	"os/exec"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SendText pastes literal user text and then submits it with a separate Enter.
// Bracketed paste is used when enabled by the receiving application.
func (Client) SendText(paneID, text string) error {
	return sendText(func(input string, args ...string) error {
		cmd := exec.Command("tmux", args...)
		cmd.Stdin = strings.NewReader(input)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}, paneID, text)
}

func sendText(run func(string, ...string) error, paneID, text string) error {
	if len(paneID) < 2 || paneID[0] != '%' {
		return fmt.Errorf("invalid pane ID")
	}
	for _, r := range paneID[1:] {
		if r < '0' || r > '9' {
			return fmt.Errorf("invalid pane ID")
		}
	}
	if !utf8.ValidString(text) {
		return fmt.Errorf("text must be valid UTF-8")
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("text must not be empty")
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return fmt.Errorf("text contains an unsupported control character")
		}
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return fmt.Errorf("create paste buffer name: %w", err)
	}
	name := fmt.Sprintf("aeolus-%x", token)
	defer func() { _ = run("", "delete-buffer", "-b", name) }()
	if err := run(text, "load-buffer", "-b", name, "-"); err != nil {
		return fmt.Errorf("load prompt: %w", err)
	}
	if err := run("", "paste-buffer", "-p", "-r", "-b", name, "-t", paneID); err != nil {
		return fmt.Errorf("paste prompt: %w", err)
	}
	if err := run("", "send-keys", "-t", paneID, "Enter"); err != nil {
		return fmt.Errorf("prompt was pasted but submission failed; inspect the target before retrying: %w", err)
	}
	return nil
}
