package shellblocks

import (
	"fmt"
	"path/filepath"
	"strings"
)

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

// ZshHooks builds a sourceable hook script. It preserves existing hook arrays,
// installs once per token, and does not enable capture or write startup files.
func ZshHooks(token, base64Path, trPath string) (string, error) {
	if !validToken(token) {
		return "", fmt.Errorf("invalid shell event token")
	}
	for _, path := range []string{base64Path, trPath} {
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
			return "", fmt.Errorf("hook tools must use absolute executable paths")
		}
	}
	script := `
if (( ${+functions[_aeolus_TOKEN_preexec]} )); then
  return 0
fi
zmodload zsh/datetime || return 0
typeset -g _aeolus_TOKEN_sequence=0 _aeolus_TOKEN_active=0
_aeolus_TOKEN_preexec() {
  emulate -L zsh
  local LC_ALL=C
  local _command="$1" _directory="$PWD" _command64 _directory64
  _aeolus_TOKEN_active=0
  if (( ${#_command} == 0 || ${#_command} > COMMAND_LIMIT || ${#_directory} > DIRECTORY_LIMIT )); then
    return 0
  fi
  _command64=$(builtin printf '%s' "$_command" | BASE64 | TR -d '\n') || return 0
  _directory64=$(builtin printf '%s' "$_directory" | BASE64 | TR -d '\n') || return 0
  (( _aeolus_TOKEN_sequence += 1 ))
  _aeolus_TOKEN_active=1
  builtin printf '\033]777;aeolus;v1;TOKEN;S;%s;%s;%s;%s\a' "$_aeolus_TOKEN_sequence" "$EPOCHREALTIME" "$_command64" "$_directory64"
  return 0
}
_aeolus_TOKEN_precmd() {
  local _status=$?
  emulate -L zsh
  if (( _aeolus_TOKEN_active )); then
    builtin printf '\033]777;aeolus;v1;TOKEN;E;%s;%s;%s\a' "$_aeolus_TOKEN_sequence" "$EPOCHREALTIME" "$_status"
    _aeolus_TOKEN_active=0
  fi
  return 0
}
typeset -ga preexec_functions precmd_functions
preexec_functions+=(_aeolus_TOKEN_preexec)
precmd_functions+=(_aeolus_TOKEN_precmd)
`
	r := strings.NewReplacer("TOKEN", token, "BASE64", shellQuote(base64Path), "TR", shellQuote(trPath), "COMMAND_LIMIT", fmt.Sprint(MaxCommandBytes), "DIRECTORY_LIMIT", fmt.Sprint(MaxDirectoryBytes))
	return r.Replace(script), nil
}
