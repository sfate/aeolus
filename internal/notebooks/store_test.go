package notebooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMarkdownAndStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "notebooks.json")
	document, err := ParseMarkdown("Release", "# Release\n\nReview the deployment.\n\n```sh\nprintf '%s' ready\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Blocks) != 2 || document.Blocks[0].Kind != Markdown || document.Blocks[1].Kind != Shell || document.Blocks[1].Content != "printf '%s' ready" {
		t.Fatalf("parsed blocks: %#v", document.Blocks)
	}
	if err := Save(path, document); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || len(got) != 1 || got[0].Title != document.Title || len(got[0].Blocks) != len(document.Blocks) {
		t.Fatalf("round trip: %#v, %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("notebook permissions: %v, %v", info, err)
	}
	updated, _ := ParseMarkdown("Release", "Updated notes")
	if err := Save(path, updated); err == nil {
		t.Fatal("duplicate title overwrote saved notebook")
	}
	got, err = Load(path)
	if err != nil || len(got) != 1 || got[0].Blocks[0].Content != "# Release\n\nReview the deployment." {
		t.Fatalf("existing notebook changed: %#v, %v", got, err)
	}
}

func TestLoadRejectsMalformedNotebookDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notebooks.json")
	cases := []string{
		`null`, `{}`, `{"version":2,"documents":[]}`, `{"version":1,"documents":null}`,
		`{"version":1,"version":1,"documents":[]}`, `{"version":1,"documents":[],"extra":true}`,
		`{"version":1,"documents":[{"title":"x","blocks":[],"extra":true}]}`,
		`{"version":1,"documents":[{"title":"x","blocks":[{"kind":"sh","content":"echo hi"}]}]}`,
		`{"version":1,"documents":[{"title":"x","blocks":[{"kind":"shell","content":""}]}]}`,
		`{"version":1,"documents":[{"title":"x","blocks":[{"kind":"markdown","content":"ok","extra":true}]}]}`,
	}
	for _, data := range cases {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("accepted malformed store %s", data)
		}
	}
}

func TestMarkdownRejectsInvalidShellFencesAndControls(t *testing.T) {
	for _, source := range []string{"```sh\necho hi", "```sh\n\n```", "hello\x1b[31m"} {
		if _, err := ParseMarkdown("bad", source); err == nil {
			t.Errorf("accepted invalid Markdown %q", source)
		}
	}
	if _, err := ParseMarkdown("bad", strings.Repeat("x", MaxTextBytes+1)); err == nil {
		t.Fatal("oversized Markdown accepted")
	}
}

func TestLoadRejectsNonPrivateNotebookFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notebooks.json")
	document, err := ParseMarkdown("Private", "Notes")
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, document); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("non-private notebook file was accepted")
	}
}
