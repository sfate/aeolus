package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfate/aeolus/internal/history"
	"github.com/sfate/aeolus/internal/monitor"
	promptstore "github.com/sfate/aeolus/internal/prompts"
	workflowstore "github.com/sfate/aeolus/internal/workflows"
)

type localResultKind string

const (
	localHistory  localResultKind = "Command"
	localWorkflow localResultKind = "Workflow"
	localPrompt   localResultKind = "Prompt"
)

type localSearchResult struct {
	kind                          localResultKind
	name, preview, metadata, body string
}

type localSearchState struct {
	active, loading bool
	request         uint64
	target          string
	query           string
	selection       int
	results         []localSearchResult
	err             string
}

type localSearchLoadedMsg struct {
	request uint64
	target  string
	results []localSearchResult
	err     string
}

func (m *Model) openLocalSearch() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.sessions) {
		return nil
	}
	s := m.sessions[m.cursor]
	if !m.composer.active {
		m.composer = promptComposer{active: true, paneID: s.Pane.PaneID, provider: s.Provider, kind: s.Kind}
	}
	if !m.composerTargetAvailable() {
		m.composer.err = fmt.Errorf("target session changed or disappeared; draft retained")
		return nil
	}
	m.localSeq++
	request, target, shell := m.localSeq, m.composer.paneID, m.composer.kind == monitor.SessionShell
	m.localSearch = localSearchState{active: true, loading: true, request: request, target: target}
	m.updateViewport()

	historyLoad := m.historyLoad
	if historyLoad == nil {
		historyLoad = func() ([]history.Entry, error) {
			path, err := history.DefaultPath()
			if err != nil {
				return nil, err
			}
			return history.Load(path)
		}
	}
	workflowOps := m.workflowIO
	if workflowOps.path == nil {
		workflowOps.path = workflowstore.DefaultPath
	}
	if workflowOps.load == nil {
		workflowOps.load = workflowstore.Load
	}
	return func() tea.Msg {
		msg := localSearchLoadedMsg{request: request, target: target}
		if shell {
			entries, err := historyLoad()
			if err != nil {
				msg.err = "History: " + err.Error()
			} else {
				for i := len(entries) - 1; i >= 0; i-- {
					entry := entries[i]
					preview := strings.ReplaceAll(entry.Command, "\n", " ↵ ")
					exit := "unknown exit"
					if entry.ExitCode != nil {
						exit = fmt.Sprintf("exit %d", *entry.ExitCode)
					}
					msg.results = append(msg.results, localSearchResult{localHistory, "Saved command", preview, entry.Directory + " · " + exit, entry.Command})
				}
			}
			path, err := workflowOps.path()
			if err == nil {
				var entries []workflowstore.Entry
				entries, err = workflowOps.load(path)
				if err == nil {
					for _, entry := range entries {
						params, _ := workflowstore.Parameters(entry.Template)
						meta := "No parameters"
						if len(params) > 0 {
							meta = fmt.Sprintf("%d parameters", len(params))
						}
						msg.results = append(msg.results, localSearchResult{localWorkflow, entry.Name, strings.ReplaceAll(entry.Template, "\n", " ↵ "), meta, entry.Template})
					}
				}
			}
			if err != nil {
				msg.err = appendSearchError(msg.err, "Workflows: "+err.Error())
			}
		}
		path, err := promptstore.DefaultPath()
		if err == nil {
			var entries []promptstore.Entry
			entries, err = promptstore.Load(path)
			if err == nil {
				for _, entry := range entries {
					msg.results = append(msg.results, localSearchResult{localPrompt, entry.Name, strings.ReplaceAll(entry.Text, "\n", " ↵ "), "Reusable prompt", entry.Text})
				}
			}
		}
		if err != nil {
			msg.err = appendSearchError(msg.err, "Prompts: "+err.Error())
		}
		return msg
	}
}

func appendSearchError(current, next string) string {
	if current == "" {
		return next
	}
	return current + "\n" + next
}

func (m *Model) localSearchLoaded(msg localSearchLoadedMsg) {
	if !m.localSearch.active || m.localSearch.request != msg.request || m.localSearch.target != msg.target || m.composer.paneID != msg.target {
		return
	}
	m.localSearch.loading = false
	m.localSearch.results = msg.results
	m.localSearch.err = msg.err
	m.localSearch.selection = 0
	m.updateViewport()
}

func fuzzyScore(query, text string) (int, bool) {
	queryRunes := []rune(strings.ToLower(query))
	textRunes := []rune(strings.ToLower(text))
	if len(queryRunes) == 0 {
		return 0, true
	}
	score, previous, start := 0, -1, -1
	for _, wanted := range queryRunes {
		found := -1
		for i := previous + 1; i < len(textRunes); i++ {
			if textRunes[i] == wanted {
				found = i
				break
			}
		}
		if found < 0 {
			return 0, false
		}
		if start < 0 {
			start = found
		}
		score += 10
		if previous >= 0 && found == previous+1 {
			score += 8
		}
		if found == 0 || unicode.IsSpace(textRunes[found-1]) || strings.ContainsRune("_-/.", textRunes[found-1]) {
			score += 5
		}
		previous = found
	}
	return score - start, true
}

func (m localSearchState) matches() []localSearchResult {
	type ranked struct {
		result localSearchResult
		score  int
		order  int
	}
	matches := make([]ranked, 0, len(m.results))
	for i, result := range m.results {
		fields := result.name + " " + result.preview + " " + result.metadata
		if score, ok := fuzzyScore(m.query, fields); ok {
			matches = append(matches, ranked{result, score, i})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].order < matches[j].order
	})
	result := make([]localSearchResult, len(matches))
	for i := range matches {
		result[i] = matches[i].result
	}
	return result
}

func (m *Model) localSearchKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.localSearch.active {
		return false, nil
	}
	s := &m.localSearch
	if s.loading {
		if msg.String() == "esc" {
			*s = localSearchState{}
			m.updateViewport()
		}
		return true, nil
	}
	matches := s.matches()
	switch msg.String() {
	case "esc":
		*s = localSearchState{}
	case "ctrl+c":
		m.quitting = true
		return true, tea.Quit
	case "up", "ctrl+p":
		if s.selection > 0 {
			s.selection--
		}
	case "down", "ctrl+n":
		if s.selection+1 < len(matches) {
			s.selection++
		}
	case "enter":
		if len(matches) > 0 && m.composerTargetAvailable() {
			m.insertLocalResult(matches[s.selection])
		}
	case "backspace", "ctrl+h":
		query := []rune(s.query)
		if len(query) > 0 {
			s.query = string(query[:len(query)-1])
			s.selection = 0
		}
	case "ctrl+u":
		s.query, s.selection = "", 0
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			input := msg.Runes
			if msg.Type == tea.KeySpace {
				input = []rune{' '}
			}
			for _, r := range input {
				if !unicode.IsControl(r) && len(s.query) < 4096 {
					s.query += string(r)
				}
			}
			s.selection = 0
		}
	}
	m.updateViewport()
	return true, nil
}

func (m *Model) insertLocalResult(result localSearchResult) {
	if !m.composerTargetAvailable() {
		m.localSearch.err = "Target session changed or disappeared; draft retained."
		return
	}
	switch result.kind {
	case localHistory, localPrompt:
		m.composer.insert([]rune(result.body))
		m.localSearch = localSearchState{}
	case localWorkflow:
		params, err := workflowstore.Parameters(result.body)
		if err != nil {
			m.localSearch.err = err.Error()
			return
		}
		m.localSearch = localSearchState{}
		m.composer.workflows = workflowModal{active: true, mode: workflowBrowse, name: result.name, template: result.body, names: params, values: make([]string, len(params))}
		if len(params) > 0 {
			m.composer.workflows.mode = workflowParameters
		} else {
			m.insertWorkflow()
			m.composer.workflows = workflowModal{}
		}
	}
	m.updateViewport()
}

func (m Model) localSearchView() string {
	var text strings.Builder
	scope := "Search reusable prompts for the selected agent. Selection inserts into the draft; it never sends."
	if m.composer.kind == monitor.SessionShell {
		scope = "Search saved commands, workflows, and prompts. Selection inserts into the draft; it never executes or sends."
	}
	fmt.Fprintf(&text, "%s\n%s\n\n%s %s▏\n", m.theme.screenTitle("Local library search"), m.theme.metadata(scope), m.theme.sectionLabel("Search"), ansi.Truncate(m.localSearch.query, max(4, m.width-12), "…"))
	if m.localSearch.loading {
		text.WriteString("\n\n" + m.theme.metadata("Loading local libraries…"))
	} else {
		if m.localSearch.err != "" {
			fmt.Fprintf(&text, "\n%s\n", m.theme.errorText(safeReviewText(m.localSearch.err)))
		}
		matches := m.localSearch.matches()
		if len(matches) == 0 {
			text.WriteString("\nNo matching local items.\n")
		}
		rows := max(1, m.height-10)
		start := max(0, min(m.localSearch.selection-rows/2, len(matches)-rows))
		end := min(len(matches), start+rows)
		for i := start; i < end; i++ {
			marker := "  "
			if i == m.localSearch.selection {
				marker = "> "
			}
			result := matches[i]
			label := fmt.Sprintf("%-8s %s", result.kind, ansi.Truncate(safeReviewText(result.name), max(1, m.width-14), "…"))
			if i == m.localSearch.selection {
				text.WriteString(m.theme.selectedRow(label) + "\n")
			} else {
				fmt.Fprintf(&text, "%s%s\n", marker, label)
			}
			fmt.Fprintf(&text, "  %s\n  %s\n", ansi.Truncate(safeReviewText(result.preview), max(1, m.width-4), "…"), m.theme.metadata(ansi.Truncate(safeReviewText(result.metadata), max(1, m.width-4), "…")))
		}
	}
	text.WriteString("\n" + m.theme.primaryHint("[↑↓/ctrl+p/ctrl+n] Select  [enter] Insert for review  [esc] Back"))
	return ansi.Hardwrap(text.String(), max(1, m.width), true)
}
