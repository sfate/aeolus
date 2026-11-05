// Package shellblocks interprets explicit shell events in a terminal byte stream.
package shellblocks

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxBlocks         = 32
	MaxOutputBytes    = 64 << 10
	MaxCommandBytes   = 16 << 10
	MaxDirectoryBytes = 4 << 10
	MaxFrameBytes     = 32 << 10
)

var prefix = []byte("\x1b]777;aeolus;v1;")

// Block contains terminal bytes between explicit preexec and precmd events.
// Background process output can interleave; Output is not exclusive process stdout.
type Block struct {
	ID                    uint64
	Command, Directory    string
	StartedAt, FinishedAt time.Time
	Duration              time.Duration
	ExitCode              *int
	Output                []byte
	Truncated, Incomplete bool
}

// Parser is a bounded single-stream collector. It is not safe for concurrent use.
type Parser struct {
	token          string
	pending, frame []byte
	blocks         []Block
	active         int
	lastID         uint64
	started        time.Time
	now            func() time.Time
}

func validToken(token string) bool {
	b, err := hex.DecodeString(token)
	return err == nil && len(b) == 16 && token == strings.ToLower(token)
}

func NewParser(token string) (*Parser, error) {
	if !validToken(token) {
		return nil, fmt.Errorf("invalid shell event token")
	}
	return &Parser{token: token, active: -1, now: time.Now}, nil
}

func (p *Parser) output(data []byte) {
	if p.active < 0 {
		return
	}
	b := &p.blocks[p.active]
	remaining := MaxOutputBytes - len(b.Output)
	if len(data) > remaining {
		data = data[:remaining]
		b.Truncated = true
	}
	b.Output = append(b.Output, data...)
}

// Write accepts arbitrary chunk boundaries, including split protocol markers.
func (p *Parser) Write(data []byte) (int, error) {
	for _, c := range data {
		if len(p.frame) > 0 {
			p.frame = append(p.frame, c)
			if c == '\a' {
				if !p.event(string(p.frame[len(prefix) : len(p.frame)-1])) {
					p.output(p.frame)
				}
				p.frame = p.frame[:0]
			} else if len(p.frame) >= MaxFrameBytes {
				p.output(p.frame)
				p.frame = p.frame[:0]
			}
			continue
		}
		p.pending = append(p.pending, c)
		for len(p.pending) > 0 && !bytes.HasPrefix(prefix, p.pending) {
			p.output(p.pending[:1])
			p.pending = p.pending[1:]
		}
		if len(p.pending) == len(prefix) {
			p.frame = append(p.frame[:0], p.pending...)
			p.pending = nil
		}
	}
	return len(data), nil
}

func eventTime(value string) (time.Time, bool) {
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > 1e11 {
		return time.Time{}, false
	}
	whole, fraction := math.Modf(seconds)
	return time.Unix(int64(whole), int64(fraction*1e9)), true
}

func decode(value string, limit int) (string, bool) {
	data, err := base64.StdEncoding.Strict().DecodeString(value)
	return string(data), err == nil && len(data) <= limit && utf8.Valid(data) && !bytes.ContainsRune(data, 0)
}

func (p *Parser) event(data string) bool {
	fields := strings.Split(data, ";")
	if len(fields) < 5 || fields[0] != p.token {
		return false
	}
	id, err := strconv.ParseUint(fields[2], 10, 64)
	at, validTime := eventTime(fields[3])
	if err != nil || id == 0 || !validTime {
		return false
	}
	switch fields[1] {
	case "S":
		if len(fields) != 6 || id <= p.lastID {
			return false
		}
		command, commandOK := decode(fields[4], MaxCommandBytes)
		directory, directoryOK := decode(fields[5], MaxDirectoryBytes)
		if !commandOK || !directoryOK || command == "" || !strings.HasPrefix(directory, "/") {
			return false
		}
		if len(p.blocks) == MaxBlocks {
			p.blocks = p.blocks[1:]
		}
		p.blocks = append(p.blocks, Block{ID: id, Command: command, Directory: directory, StartedAt: at, Incomplete: true})
		p.active = len(p.blocks) - 1
		p.lastID = id
		p.started = p.now()
		return true
	case "E":
		if len(fields) != 5 || p.active < 0 || p.blocks[p.active].ID != id {
			return false
		}
		status, err := strconv.Atoi(fields[4])
		if err != nil || status < 0 || status > 255 {
			return false
		}
		b := &p.blocks[p.active]
		b.ExitCode = &status
		b.FinishedAt = at
		b.Duration = max(0, p.now().Sub(p.started))
		b.Incomplete = false
		p.active = -1
		return true
	}
	return false
}

// Close flushes an unfinished marker and retains incomplete commands without a
// guessed exit status. No filesystem writes occur in this package.
func (p *Parser) Close() {
	p.output(p.frame)
	p.output(p.pending)
	p.frame = nil
	p.pending = nil
	p.active = -1
}

// Snapshot returns copies so consumers cannot mutate collector state.
func (p *Parser) Snapshot() []Block {
	result := append([]Block(nil), p.blocks...)
	for i := range result {
		result[i].Output = append([]byte(nil), result[i].Output...)
		if result[i].ExitCode != nil {
			status := *result[i].ExitCode
			result[i].ExitCode = &status
		}
	}
	return result
}
