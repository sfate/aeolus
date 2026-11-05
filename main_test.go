package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestStartupValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`{"version":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--config", path}, {"--config", path + "missing"}, {"--unknown"}, {"unexpected"}} {
		if _, err := loadConfig(args, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if _, err := loadConfig([]string{"--help"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("help error=%v", err)
	}
}
