package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode"
	"unicode/utf8"
)

func decodeProviderArgs(raw json.RawMessage) (map[string][]string, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("provider_args must be an object")
	}
	result := map[string][]string{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, err
		}
		provider := token.(string)
		if !supportedProvider(provider) {
			return nil, fmt.Errorf("unsupported provider_args key %q", provider)
		}
		if _, exists := result[provider]; exists {
			return nil, fmt.Errorf("duplicate provider_args key %q", provider)
		}
		var rawArgs json.RawMessage
		if err = d.Decode(&rawArgs); err != nil {
			return nil, err
		}
		if len(rawArgs) == 0 || rawArgs[0] != '[' {
			return nil, fmt.Errorf("provider_args.%s must be an array of strings", provider)
		}
		var entries []json.RawMessage
		if err = json.Unmarshal(rawArgs, &entries); err != nil {
			return nil, err
		}
		if len(entries) > 64 {
			return nil, fmt.Errorf("provider_args.%s exceeds 64 arguments", provider)
		}
		args := make([]string, 0, len(entries))
		total := 0
		for _, rawArg := range entries {
			var arg string
			if bytes.Equal(bytes.TrimSpace(rawArg), []byte("null")) || json.Unmarshal(rawArg, &arg) != nil {
				return nil, fmt.Errorf("provider_args.%s requires strings", provider)
			}
			if len(arg) > 4096 || !utf8.ValidString(arg) {
				return nil, fmt.Errorf("provider argument exceeds 4096 bytes or has invalid encoding")
			}
			for _, r := range arg {
				if unicode.IsControl(r) {
					return nil, fmt.Errorf("provider arguments cannot contain terminal controls")
				}
			}
			total += len(arg)
			if total > 16384 {
				return nil, fmt.Errorf("provider arguments exceed 16 KiB")
			}
			args = append(args, arg)
		}
		result[provider] = args
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	return result, nil
}
