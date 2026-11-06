package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DashboardKeys defines stable action names and their default dashboard shortcut.
// Text input, modal controls, arrows, Enter, and Ctrl+C remain fixed.
var DashboardKeys = map[string]string{
	"launch": "c", "rename": "R", "git_review": "g", "compose": "i", "filters": "f", "search": "/", "refresh": "r", "quit": "q", "kill": "x", "approve": "y", "deny": "n", "approve_session": "a", "next": "j", "previous": "k",
}

func decodeKeys(raw json.RawMessage) (map[string]string, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("keybindings must be an object")
	}
	result := map[string]string{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, err
		}
		action := token.(string)
		if _, ok := DashboardKeys[action]; !ok {
			return nil, fmt.Errorf("unknown dashboard action %q", action)
		}
		if _, ok := result[action]; ok {
			return nil, fmt.Errorf("duplicate keybinding action %q", action)
		}
		var key string
		if err = d.Decode(&key); err != nil || !validDashboardKey(key) {
			return nil, fmt.Errorf("invalid dashboard shortcut for %q", action)
		}
		result[action] = key
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("unexpected keybindings content")
	}
	used := map[string]string{}
	for action, def := range DashboardKeys {
		key := def
		if custom, ok := result[action]; ok {
			key = custom
		}
		if other, ok := used[key]; ok {
			return nil, fmt.Errorf("shortcut %q conflicts between %s and %s", key, other, action)
		}
		used[key] = action
	}
	return result, nil
}

func validDashboardKey(key string) bool {
	if strings.HasPrefix(key, "ctrl+") {
		// Keep modal editing and application shortcuts available for their own actions.
		return len(key) == 6 && strings.ContainsRune("bdfgjlnrtwxyz", rune(key[5]))
	}
	if !utf8.ValidString(key) || utf8.RuneCountInString(key) != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(key)
	return !unicode.IsControl(r) && !unicode.IsSpace(r) && !strings.ContainsRune("?WSH!", r)
}
