package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sfate/aeolus/internal/history"
	"github.com/sfate/aeolus/internal/monitor"
	promptstore "github.com/sfate/aeolus/internal/prompts"
	"github.com/sfate/aeolus/internal/workflows"
)

func TestLocalSearchLoadsAndFuzzyMatchesEveryLocalType(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	historyPath, _ := history.DefaultPath()
	workflowPath, _ := workflows.DefaultPath()
	promptPath, _ := promptstore.DefaultPath()
	if _, err := history.Append(historyPath, history.Entry{Command: "git deploy", Directory: "/project/app"}); err != nil {
		t.Fatal(err)
	}
	if err := workflows.Save(workflowPath, "Deploy service", "printf '%s' {{environment}}"); err != nil {
		t.Fatal(err)
	}
	if err := promptstore.Save(promptPath, "Deploy notes", "Review the deployment notes"); err != nil {
		t.Fatal(err)
	}

	m := NewModelWithBackend(&textBackend{}, &fakeMonitor{})
	s := testSession("%1")
	s.Kind, s.Provider = monitor.SessionShell, monitor.ProviderUnknown
	m.sessions = []monitor.Session{s}
	cmd := m.openLocalSearch()
	if cmd == nil || !m.localSearch.loading {
		t.Fatal("local search did not load asynchronously")
	}
	m, _ = updateModel(t, m, cmd())
	m.localSearch.query = "dpl"
	matches := m.localSearch.matches()
	kinds := map[localResultKind]bool{}
	for _, result := range matches {
		kinds[result.kind] = true
	}
	if !kinds[localHistory] || !kinds[localWorkflow] || !kinds[localPrompt] {
		t.Fatalf("fuzzy cross-type matches omitted a result type: %#v", matches)
	}
	m.localSearch.query = "unlikely-no-result"
	m.width = 80
	if got := m.localSearch.matches(); len(got) != 0 || !strings.Contains(m.localSearchView(), "No matching local items") {
		t.Fatalf("empty search state is unclear: %#v", got)
	}
}

func TestLocalSearchSelectionInsertsPromptForReviewWithoutSending(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path, _ := promptstore.DefaultPath()
	if err := promptstore.Save(path, "Review prompt", "Inspect deployment safely"); err != nil {
		t.Fatal(err)
	}
	backend := &textBackend{}
	m := NewModelWithBackend(backend, &fakeMonitor{})
	m.sessions = []monitor.Session{testSession("%1")}
	cmd := m.openLocalSearch()
	m, _ = updateModel(t, m, cmd())
	m.localSearch.query = "review prompt"
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.localSearch.active || m.composer.draft != "Inspect deployment safely" || backend.text != "" {
		t.Fatalf("selection did not remain a reviewable draft: active=%v draft=%q sent=%q", m.localSearch.active, m.composer.draft, backend.text)
	}
}

func TestLocalSearchPaletteActionAndShellTypeFiltering(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path, _ := promptstore.DefaultPath()
	if err := promptstore.Save(path, "Reusable", "Draft text"); err != nil {
		t.Fatal(err)
	}
	m := NewModel()
	m.sessions = []monitor.Session{testSession("%1")}
	m = openPalette(t, m)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Search local library")})
	m, cmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.localSearch.active || cmd == nil {
		t.Fatal("palette action did not open unified local search")
	}
	m, _ = updateModel(t, m, cmd())
	if matches := m.localSearch.matches(); len(matches) != 1 || matches[0].kind != localPrompt {
		t.Fatalf("agent session exposed shell-only results: %#v", matches)
	}
}

func TestFuzzyScorePrefersContiguousMatches(t *testing.T) {
	contiguous, ok := fuzzyScore("gco", "gco command")
	if !ok {
		t.Fatal("subsequence fuzzy match was rejected")
	}
	scattered, ok := fuzzyScore("gco", "gathered curious object")
	if !ok || contiguous <= scattered {
		t.Fatalf("contiguous match score %d should beat scattered score %d (matched=%v)", contiguous, scattered, ok)
	}
}
