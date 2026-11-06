package config

import (
	"strings"
	"testing"
)

func TestProviderArgumentsStrictValidation(t *testing.T) {
	got, err := decode(strings.NewReader(`{"version":1,"provider_args":{"claude":["--mcp-config","/tmp/project config.json"],"codex":["--profile","work"]}}`))
	if err != nil || len(got.ProviderArgs["claude"]) != 2 {
		t.Fatalf("decode %#v %v", got, err)
	}
	for _, raw := range []string{`null`, `[]`, `{"bash":[]}`, `{"claude":null}`, `{"claude":[null]}`, `{"claude":[1]}`, `{"claude":[],"claude":[]}`, `{"claude":["bad\u001b"]}`} {
		if _, err := decode(strings.NewReader(`{"version":1,"provider_args":` + raw + `}`)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
