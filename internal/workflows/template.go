// Package workflows stores local command templates and renders literal shell arguments.
package workflows

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"mvdan.cc/sh/v3/syntax"
)

const MaxTextBytes = 64 << 10

var parameterName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

type placeholder struct {
	name, sentinel string
	start, end     int
}

func validText(text string) bool {
	if len(text) > MaxTextBytes || !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}

func parseTemplate(template string) ([]placeholder, []string, error) {
	if !validText(template) || strings.TrimSpace(template) == "" {
		return nil, nil, fmt.Errorf("workflow template must be nonempty UTF-8 up to 64 KiB without terminal controls")
	}
	prefix := "AEOLUS_WF_PARAM_"
	for strings.Contains(template, prefix) {
		prefix += "X"
	}
	var parts []placeholder
	names := []string{}
	seen := map[string]bool{}
	var replaced strings.Builder
	for position := 0; position < len(template); {
		remaining := template[position:]
		open, close := strings.Index(remaining, "{{"), strings.Index(remaining, "}}")
		if close >= 0 && (open < 0 || close < open) {
			return nil, nil, fmt.Errorf("unmatched workflow parameter delimiter")
		}
		if open < 0 {
			replaced.WriteString(remaining)
			break
		}
		start := position + open
		end := strings.Index(template[start+2:], "}}")
		if end < 0 {
			return nil, nil, fmt.Errorf("unclosed workflow parameter")
		}
		end += start + 4
		name := template[start+2 : end-2]
		if !parameterName.MatchString(name) {
			return nil, nil, fmt.Errorf("parameter names must be ASCII identifiers up to 64 characters")
		}
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
		if len(names) > 16 || len(parts) >= 256 {
			return nil, nil, fmt.Errorf("workflow exceeds 16 parameters or 256 placeholder occurrences")
		}
		sentinel := prefix + strconv.Itoa(len(parts)) + "_"
		parts = append(parts, placeholder{name: name, sentinel: sentinel, start: start, end: end})
		replaced.WriteString(template[position:start])
		replaced.WriteString(sentinel)
		position = end
	}
	if len(parts) == 0 {
		return parts, names, nil
	}
	source := replaced.String()
	file, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX), syntax.KeepComments(true)).Parse(strings.NewReader(source), "workflow")
	if err != nil {
		return nil, nil, fmt.Errorf("parameterized workflows require valid POSIX shell syntax: %w", err)
	}
	accepted := map[string]bool{}
	syntax.Walk(file, func(node syntax.Node) bool {
		switch node := node.(type) {
		case *syntax.Assign, *syntax.Redirect, *syntax.CmdSubst, *syntax.ParamExp, *syntax.ArithmExp, *syntax.SglQuoted, *syntax.DblQuoted:
			return false
		case *syntax.CallExpr:
			for _, word := range node.Args {
				if len(word.Parts) != 1 {
					continue
				}
				literal, ok := word.Parts[0].(*syntax.Lit)
				if !ok {
					continue
				}
				start, end := int(word.Pos().Offset()), int(word.End().Offset())
				if start >= 0 && end <= len(source) && source[start:end] == literal.Value {
					accepted[literal.Value] = true
				}
			}
		}
		return true
	})
	for _, part := range parts {
		if !accepted[part.sentinel] {
			return nil, nil, fmt.Errorf("parameter %q must be a standalone unquoted command word, outside assignments, redirections, comments, and substitutions", part.name)
		}
	}
	return parts, names, nil
}

// Parameters returns unique parameter names in first-occurrence order after validating their shell syntax contexts.
func Parameters(template string) ([]string, error) {
	_, names, err := parseTemplate(template)
	return names, err
}

// Render substitutes POSIX single-quoted literal values without running commands or persisting values.
func Render(template string, values map[string]string) (string, error) {
	parts, names, err := parseTemplate(template)
	if err != nil {
		return "", err
	}
	if len(values) != len(names) {
		return "", fmt.Errorf("supply exactly the workflow's named parameter values")
	}
	for _, name := range names {
		value, ok := values[name]
		if !ok || !validText(value) {
			return "", fmt.Errorf("parameter %q is missing, oversized, or contains unsupported controls", name)
		}
	}
	var text strings.Builder
	position := 0
	for _, part := range parts {
		text.WriteString(template[position:part.start])
		text.WriteByte('\'')
		text.WriteString(strings.ReplaceAll(values[part.name], "'", "'\"'\"'"))
		text.WriteByte('\'')
		if text.Len() > MaxTextBytes {
			return "", fmt.Errorf("rendered workflow exceeds 64 KiB")
		}
		position = part.end
	}
	text.WriteString(template[position:])
	if text.Len() > MaxTextBytes {
		return "", fmt.Errorf("rendered workflow exceeds 64 KiB")
	}
	return text.String(), nil
}
