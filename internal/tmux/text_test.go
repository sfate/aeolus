package tmux

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type textCall struct {
	input string
	args  []string
}

func TestSendTextLiteralAndCleanup(t *testing.T) {
	var calls []textCall
	err := sendText(func(input string, args ...string) error { calls = append(calls, textCall{input, args}); return nil }, "%7", "Enter Escape 猫\r\nsecond\tline")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 4 || calls[0].input != "Enter Escape 猫\nsecond\tline" || calls[0].args[0] != "load-buffer" {
		t.Fatalf("calls=%+v", calls)
	}
	name := calls[0].args[2]
	if !strings.HasPrefix(name, "aeolus-") || !reflect.DeepEqual(calls[1].args, []string{"paste-buffer", "-p", "-r", "-b", name, "-t", "%7"}) || !reflect.DeepEqual(calls[2].args, []string{"send-keys", "-t", "%7", "Enter"}) || !reflect.DeepEqual(calls[3].args, []string{"delete-buffer", "-b", name}) {
		t.Fatalf("calls=%+v", calls)
	}
}
func TestSendTextRejectsControlsAndInvalidTarget(t *testing.T) {
	for _, text := range []string{"", "  ", "bad\x00", "bad\x1b[201~", "bad\x03", string([]byte{255})} {
		called := false
		err := sendText(func(string, ...string) error { called = true; return nil }, "%1", text)
		if err == nil || called {
			t.Fatalf("accepted %q", text)
		}
	}
	for _, id := range []string{"", "shell", "%", "%1;kill-server", "-t"} {
		if err := sendText(func(string, ...string) error { t.Fatal("called transport"); return nil }, id, "hello"); err == nil {
			t.Fatalf("accepted target %q", id)
		}
	}
}
func TestSendTextFailureNeverSubmitsAndCleans(t *testing.T) {
	for _, stage := range []string{"load-buffer", "paste-buffer", "send-keys"} {
		t.Run(stage, func(t *testing.T) {
			var commands []string
			err := sendText(func(_ string, args ...string) error {
				commands = append(commands, args[0])
				if args[0] == stage {
					return errors.New("failed")
				}
				return nil
			}, "%1", "prompt")
			if err == nil || commands[len(commands)-1] != "delete-buffer" {
				t.Fatalf("err=%v commands=%v", err, commands)
			}
			if stage != "send-keys" {
				for _, cmd := range commands {
					if cmd == "send-keys" {
						t.Fatal("submitted after failed paste")
					}
				}
			}
		})
	}
}
