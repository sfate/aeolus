// Package gitsafe applies the shared command policy for local Git operations.
package gitsafe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const maxConfigurationBytes = 256 << 10

type boundedOutput struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	size := len(p)
	remaining := b.limit - b.buffer.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, err := b.buffer.Write(p)
	return size, err
}

func environment() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1", "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat", "GIT_ATTR_NOSYSTEM=1")
}

func Run(ctx context.Context, directory string, config []string, limit int, args ...string) ([]byte, bool, error) {
	prefix := []string{"--no-pager", "--no-optional-locks", "-C", directory, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "core.untrackedCache=false", "-c", "color.ui=false", "-c", "protocol.allow=never"}
	prefix = append(prefix, config...)
	cmd := exec.CommandContext(ctx, "git", append(prefix, args...)...)
	cmd.Env = environment()
	output := &boundedOutput{limit: limit}
	stderr := &boundedOutput{limit: 8 << 10}
	cmd.Stdout = output
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return nil, false, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.buffer.String()))
	}
	return output.buffer.Bytes(), output.truncated, nil
}

var filterKey = regexp.MustCompile(`^filter\.[A-Za-z0-9_.-]+\.(clean|smudge|process|required)$`)

// FilterOverrides disables configured clean, smudge, and process filters.
func FilterOverrides(ctx context.Context, directory string) ([]string, error) {
	data, truncated, err := Run(ctx, directory, nil, maxConfigurationBytes, "config", "--null", "--name-only", "--get-regexp", `^filter\..*\.(clean|smudge|process|required)$`)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	if truncated {
		return nil, fmt.Errorf("filter configuration exceeds operation limit")
	}
	var config []string
	for _, key := range strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00") {
		if key == "" {
			continue
		}
		if !filterKey.MatchString(key) {
			return nil, fmt.Errorf("unsupported filter configuration key; Git operation disabled rather than executing it")
		}
		value := ""
		if strings.HasSuffix(key, ".required") {
			value = "false"
		}
		config = append(config, "-c", key+"="+value)
	}
	return config, nil
}
