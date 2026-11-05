package config

import (
	"strings"
	"testing"
)

func TestKeybindingsValidation(t *testing.T) {
	good, err := decode(strings.NewReader(`{"version":1,"keybindings":{"launch":"L","refresh":"ctrl+r"}}`))
	if err != nil || good.Keybindings["launch"] != "L" || good.Keybindings["refresh"] != "ctrl+r" {
		t.Fatalf("decode %#v %v", good, err)
	}
	for _, raw := range []string{`null`, `[]`, `{"unknown":"L"}`, `{"launch":"q"}`, `{"launch":"L","rename":"L"}`, `{"launch":"L","launch":"M"}`, `{"launch":"ctrl+c"}`, `{"launch":"?"}`, `{"launch":""}`, `{"launch":"ctrl+o"}`, `{"launch":"ctrl+m"}`, `{"launch":null}`, `{"launch":"two"}`} {
		if _, err := decode(strings.NewReader(`{"version":1,"keybindings":` + raw + `}`)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
