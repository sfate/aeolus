package shellblocks

import (
	"fmt"
	"os"
	"path/filepath"
)

func zshBootstrap(dir, id string) (string, string, error) {
	original, wasSet := os.LookupEnv("ZDOTDIR")
	if wasSet && !filepath.IsAbs(original) {
		return "", "", fmt.Errorf("captured shells require an absolute ZDOTDIR when set")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	userDir := home
	restore := "unset ZDOTDIR\n"
	if wasSet {
		userDir = original
		restore = "export ZDOTDIR=" + shellQuote(original) + "\n"
	}
	name := "_aeolus_bootstrap_" + id
	env := restore + "if [[ -r " + shellQuote(filepath.Join(userDir, ".zshenv")) + " ]]; then\n  source " + shellQuote(filepath.Join(userDir, ".zshenv")) + "\nfi\n" +
		"typeset -g " + name + "_set=${+ZDOTDIR}\ntypeset -g " + name + "_dir=${ZDOTDIR-$HOME}\nexport ZDOTDIR=" + shellQuote(dir) + "\n"
	rc := "if (( " + name + "_set )); then\n  export ZDOTDIR=\"$" + name + "_dir\"\nelse\n  unset ZDOTDIR\nfi\n" +
		"if [[ -r \"$" + name + "_dir/.zshrc\" ]]; then\n  source \"$" + name + "_dir/.zshrc\"\nfi\nsource " + shellQuote(filepath.Join(dir, "hooks.zsh")) + "\nif (( ${+functions[_aeolus_" + id + "_preexec]} )); then\n  builtin printf ready > " + shellQuote(filepath.Join(dir, "ready")) + "\nfi\nunset " + name + "_set " + name + "_dir\n"
	return env, rc, nil
}
