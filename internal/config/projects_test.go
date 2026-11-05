package config

import (
	"strings"
	"testing"
)

func TestProjectConfiguration(t *testing.T) {
	valid := `{"version":1,"default_provider":"codex","projects":[{"name":"雪","directory":"/nonexistent/project","provider":"gemini"}]}`
	got, err := decode(strings.NewReader(valid))
	if err != nil || got.DefaultProvider != "codex" || len(got.Projects) != 1 || got.Projects[0].Name != "雪" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	got, err = decode(strings.NewReader(`{"version":1}`))
	if err != nil || got.DefaultProvider != "claude" || len(got.Projects) != 0 {
		t.Fatalf("backcompat got=%#v err=%v", got, err)
	}
	for _, bad := range []string{
		`{"version":1,"default_provider":"bash"}`, `{"version":1,"default_provider":null}`,
		`{"version":1,"projects":null}`, `{"version":1,"projects":{}}`,
		`{"version":1,"projects":[null]}`, `{"version":1,"projects":[{"name":"a","directory":"relative","provider":"claude"}]}`,
		`{"version":1,"projects":[{"name":"a","directory":"/tmp","provider":"bash"}]}`,
		`{"version":1,"projects":[{"name":"a","directory":"/tmp","provider":"claude","typo":true}]}`,
		`{"version":1,"projects":[{"name":"a","name":"b","directory":"/tmp","provider":"claude"}]}`,
		`{"version":1,"projects":[{"name":null,"directory":"/tmp","provider":"claude"}]}`,
		`{"version":1,"projects":[{"name":"a","provider":"claude"}]}`,
		`{"version":1,"projects":[{"name":"a","directory":"/tmp","provider":"claude"},{"name":"a","directory":"/else","provider":"codex"}]}`,
		`{"version":1,"projects":[],"projects":[]}`,
	} {
		if _, err := decode(strings.NewReader(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
