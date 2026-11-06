package main

import (
	"strings"
	"testing"
)

func TestInternalCollectorArguments(t *testing.T) {
	for _, args := range [][]string{{"--internal-shell-collector"}, {"--internal-shell-collector", "../path"}, {"--internal-shell-collector", "0123456789abcdef0123456789abcdef", "extra"}} {
		handled, err := runInternalHelper(args, strings.NewReader(""))
		if !handled || err == nil {
			t.Fatalf("invalid helper invocation accepted: %v", args)
		}
	}
	if handled, err := runInternalHelper([]string{"--config", "config.json"}, strings.NewReader("")); handled || err != nil {
		t.Fatal("ordinary startup intercepted")
	}
}
