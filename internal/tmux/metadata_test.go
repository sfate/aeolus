package tmux

import (
	"fmt"
	"strings"
	"testing"
)

func paneRecord(fields ...string) string {
	var b strings.Builder
	for _, v := range fields {
		fmt.Fprintf(&b, "%d:%s", len(v), v)
	}
	return b.String() + "\n"
}

func TestParsePaneMetadata(t *testing.T) {
	path := "/tmp/雪 project|with\ttab\nand\\slashes# "
	record := paneRecord("%2", "0", "1", "session|雪", "$1", "claude", "123", "80", "24", "window|name", path)
	got, err := parsePanes([]byte(record + record))
	if err != nil || len(got) != 2 || got[0].CurrentPath != path || got[0].WindowName != "window|name" || got[0].SessionName != "session|雪" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	for _, bad := range []string{"bad", "999:short", "-1:x", record[:len(record)-2], strings.Replace(record, "3:123", "3:abc", 1)} {
		if _, err := parsePanes([]byte(bad)); err == nil {
			t.Fatalf("accepted malformed record %q", bad)
		}
	}
}
