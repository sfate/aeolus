package shellblocks

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef"

func startFrame(id int, command string) string {
	return fmt.Sprintf("\x1b]777;aeolus;v1;%s;S;%d;1700000000.25;%s;%s\a", testToken, id, base64.StdEncoding.EncodeToString([]byte(command)), base64.StdEncoding.EncodeToString([]byte("/project 雪")))
}

func endFrame(id, status int) string {
	return fmt.Sprintf("\x1b]777;aeolus;v1;%s;E;%d;1700000001.50;%d\a", testToken, id, status)
}

func TestStreamSplitMarkersAndOutput(t *testing.T) {
	p, err := NewParser(testToken)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	p.now = func() time.Time { now = now.Add(time.Second); return now }
	input := "prompt not a block" + startFrame(1, "printf '雪'\nfalse") + "output\r\n\x1b[31mred\x1b[0m" + endFrame(1, 1) + "next prompt"
	for _, b := range []byte(input) {
		_, _ = p.Write([]byte{b})
	}
	blocks := p.Snapshot()
	if len(blocks) != 1 || blocks[0].Command != "printf '雪'\nfalse" || blocks[0].Directory != "/project 雪" || blocks[0].ExitCode == nil || *blocks[0].ExitCode != 1 || blocks[0].Duration != time.Second || string(blocks[0].Output) != "output\r\n\x1b[31mred\x1b[0m" || blocks[0].Incomplete {
		t.Fatalf("block: %#v", blocks)
	}
	blocks[0].Output[0] = 'X'
	if p.Snapshot()[0].Output[0] == 'X' {
		t.Fatal("snapshot aliases internal state")
	}
}

func TestBoundsIncompleteAndForgedFrames(t *testing.T) {
	p, _ := NewParser(testToken)
	_, _ = p.Write([]byte(startFrame(1, "long")))
	_, _ = p.Write([]byte(strings.Repeat("x", MaxOutputBytes+1)))
	forged := strings.Replace(endFrame(1, 0), testToken, strings.Repeat("f", 32), 1)
	_, _ = p.Write([]byte(forged))
	p.Close()
	b := p.Snapshot()[0]
	if len(b.Output) != MaxOutputBytes || !b.Truncated || !b.Incomplete || b.ExitCode != nil {
		t.Fatalf("bounded incomplete block: %#v", b)
	}
	for id := 2; id <= MaxBlocks+5; id++ {
		_, _ = p.Write([]byte(startFrame(id, "next") + endFrame(id, 0)))
	}
	if len(p.Snapshot()) != MaxBlocks {
		t.Fatal("block count unbounded")
	}
}

func TestMalformedFramesDoNotCreateCommands(t *testing.T) {
	for _, input := range []string{strings.Replace(startFrame(1, "ok"), ";S;", ";X;", 1), strings.Replace(startFrame(1, "ok"), ";1;", ";-1;", 1), startFrame(1, strings.Repeat("x", MaxCommandBytes+1)), "\x1b]777;aeolus;v1;" + strings.Repeat("x", MaxFrameBytes*2)} {
		p, _ := NewParser(testToken)
		_, _ = p.Write([]byte(input))
		p.Close()
		if len(p.Snapshot()) != 0 {
			t.Fatal("malformed frame created block")
		}
	}
	if _, err := NewParser("bad token"); err == nil {
		t.Fatal("accepted invalid token")
	}
}

func TestInvalidEndAndReplayRetainExplicitBoundaries(t *testing.T) {
	p, _ := NewParser(testToken)
	_, _ = p.Write([]byte(startFrame(1, "first") + "body"))
	badEnd := endFrame(1, 999)
	_, _ = p.Write([]byte(badEnd + startFrame(1, "replayed") + startFrame(2, "second") + endFrame(2, 0)))
	b := p.Snapshot()
	if len(b) != 2 || !b[0].Incomplete || b[0].ExitCode != nil || !strings.Contains(string(b[0].Output), badEnd) || b[1].Command != "second" || b[1].ExitCode == nil {
		t.Fatalf("invalid/replayed event changed boundaries: %#v", b)
	}
	*b[1].ExitCode = 42
	if *p.Snapshot()[1].ExitCode != 0 {
		t.Fatal("exit code pointer aliases parser state")
	}
}

func TestInvalidTimesAndMetadata(t *testing.T) {
	for _, input := range []string{
		strings.Replace(startFrame(1, "test"), "1700000000.25", "NaN", 1),
		strings.Replace(startFrame(1, "test"), "1700000000.25", "-1", 1),
		strings.Replace(startFrame(1, "test"), base64.StdEncoding.EncodeToString([]byte("test")), "%%%", 1),
		startFrame(1, "nul\x00command"),
	} {
		p, _ := NewParser(testToken)
		_, _ = p.Write([]byte(input))
		if len(p.Snapshot()) != 0 {
			t.Fatal("invalid metadata accepted")
		}
	}
}
