package workflows

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRenderQuotesLiteralParameters(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	for _, value := range []string{"", "hello 空 world", "a'b", "line one\nline two\tend", "$(touch " + marker + ")", "'; touch " + marker + "; printf '", "`touch " + marker + "`"} {
		rendered, err := Render(`printf '%s' {{value}}`, map[string]string{"value": value})
		if err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("sh", "-c", rendered).CombinedOutput()
		if err != nil || string(out) != value {
			t.Fatalf("round trip %q: %q %v (%s)", value, out, err, rendered)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("parameter executed as shell syntax: %v", err)
	}
	got, err := Parameters("printf '%s %s %s' {{second}} {{first}} {{second}} | cat")
	if err != nil || !reflect.DeepEqual(got, []string{"second", "first"}) {
		t.Fatalf("parameters %v %v", got, err)
	}
}

func TestRejectUnsafeOrAmbiguousParameterContexts(t *testing.T) {
	for _, template := range []string{
		`echo "{{value}}"`, `echo '{{value}}'`, `echo prefix{{value}}`, `echo {{value}}suffix`,
		`VALUE={{value}} echo hi`, "echo hi # {{value}}", "cat <<EOF\n{{value}}\nEOF\n",
		`echo ${HOME:-{{value}}}`, `echo "$(echo "{{value}}")"`, `echo $(echo {{value}})`, "echo `echo {{value}}`",
		`echo >{{value}}`, `for {{value}} in a; do :; done`, `echo \{{value}}`,
		`echo {{bad-name}}`, `echo {{}}`, `echo {{value`, `echo value}}`, `echo {{value}} '`,
	} {
		if _, err := Parameters(template); err == nil {
			t.Fatalf("accepted %q", template)
		}
	}
}

func TestRenderRejectsMissingExtraControlsAndOversizeValues(t *testing.T) {
	for _, values := range []map[string]string{nil, {"other": "x"}, {"value": "x", "other": "y"}, {"value": "\x1b"}, {"value": strings.Repeat("a", MaxTextBytes)}} {
		if _, err := Render("echo {{value}}", values); err == nil {
			t.Fatalf("accepted invalid values (count %d)", len(values))
		}
	}
	if text, err := Render("echo literal $(date)", nil); err != nil || text != "echo literal $(date)" {
		t.Fatalf("parameter-free template changed %q %v", text, err)
	}
	if _, err := Parameters(strings.Repeat("a", MaxTextBytes+1)); err == nil {
		t.Fatal("oversized template accepted")
	}
	if _, err := Parameters("echo \x1b"); err == nil {
		t.Fatal("template control accepted")
	}
}

func TestParameterSentinelCannotCollideWithTemplate(t *testing.T) {
	template := "printf '%s %s' AEOLUS_WF_PARAM_0_ {{value}}"
	rendered, err := Render(template, map[string]string{"value": "literal"})
	if err != nil || !strings.Contains(rendered, "AEOLUS_WF_PARAM_0_") || strings.Contains(rendered, "{{value}}") {
		t.Fatalf("sentinel collision %q %v", rendered, err)
	}
}

func TestTemplateParameterAndOccurrenceLimits(t *testing.T) {
	template := "echo"
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q"} {
		template += " {{" + name + "}}"
	}
	if _, err := Parameters(template); err == nil {
		t.Fatal("more than 16 parameters accepted")
	}
	if _, err := Parameters("echo" + strings.Repeat(" {{a}}", 257)); err == nil {
		t.Fatal("more than 256 occurrences accepted")
	}
}
