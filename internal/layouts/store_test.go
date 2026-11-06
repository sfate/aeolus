package layouts

import (
	"os"
	"path/filepath"
	"testing"
)

func validLayout() Layout {
	return Layout{Name: "Review", Directory: "/tmp/project", Windows: []Window{{Name: "Code", Panes: []Pane{{Type: Agent, Provider: "codex"}, {Type: Shell, Split: Vertical, SizePercent: 40}}}}}
}
func TestSaveLoadLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aeolus", "layouts.json")
	want := validLayout()
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != want.Name || got[0].Windows[0].Panes[1].Split != Vertical {
		t.Fatalf("unexpected layouts: %#v", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("store mode: %v %v", info, err)
	}
}
func TestRejectInvalidAndDuplicateWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layouts.json")
	l := validLayout()
	if err := Save(path, l); err != nil {
		t.Fatal(err)
	}
	bad := validLayout()
	bad.Windows[0].Panes[1].Split = "diagonal"
	if err := Save(path, bad); err == nil {
		t.Fatal("invalid split accepted")
	}
	if err := Save(path, l); err == nil {
		t.Fatal("duplicate name accepted")
	}
	got, err := Load(path)
	if err != nil || len(got) != 1 {
		t.Fatalf("existing layout lost: %#v %v", got, err)
	}
}
func TestRejectUnknownAndDuplicateFields(t *testing.T) {
	for _, data := range []string{`{"version":1,"layouts":[],"extra":true}`, `{"version":1,"version":1,"layouts":[]}`} {
		path := filepath.Join(t.TempDir(), "layouts.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted malformed JSON %s", data)
		}
	}
}
func TestValidateLayoutBounds(t *testing.T) {
	cases := []Layout{{Name: "bad", Directory: "relative", Windows: []Window{{Name: "w", Panes: []Pane{{Type: Shell}}}}}}
	bad := validLayout()
	bad.Windows[0].Panes[0].Provider = "bad"
	cases = append(cases, bad)
	for i, c := range cases {
		if err := Validate(c); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}

func TestParseRejectsDuplicateUnknownFieldsAndMultipleLayouts(t *testing.T) {
	for _, source := range []string{
		`{"name":"x","name":"y","directory":"/tmp","windows":[{"name":"w","panes":[{"type":"shell"}]}]}`,
		`{"name":"x","directory":"/tmp","windows":[{"name":"w","panes":[{"type":"shell","extra":true}]}]}`,
	} {
		if _, err := Parse([]byte(`{"version":1,"layouts":[` + source + `]}`)); err == nil {
			t.Fatalf("accepted invalid source: %s", source)
		}
	}
}
