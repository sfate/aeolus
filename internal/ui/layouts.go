package ui

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	store "github.com/sfate/aeolus/internal/layouts"
	"github.com/sfate/aeolus/internal/monitor"
	"github.com/sfate/aeolus/internal/tmux"
)

type layoutLibrary struct {
	active, loading, creating, saving, viewing bool
	confirmRun, launching                      bool
	request, launchRequest                     uint64
	layouts                                    []store.Layout
	query, source, err                         string
	selected, position, preview                int
}
type layoutsLoadedMsg struct {
	request uint64
	layouts []store.Layout
	err     error
}
type layoutSavedMsg struct {
	request uint64
	layout  store.Layout
	err     error
}

type layoutLaunchedMsg struct {
	request uint64
	layout  store.Layout
	result  tmux.LayoutResult
	err     error
}

func (m *Model) openLayoutBrowser() tea.Cmd {
	m.layoutSeq++
	m.layouts = layoutLibrary{active: true, loading: true, request: m.layoutSeq}
	m.updateViewport()
	request := m.layouts.request
	return func() tea.Msg {
		path, err := store.DefaultPath()
		var items []store.Layout
		if err == nil {
			items, err = store.Load(path)
		}
		return layoutsLoadedMsg{request: request, layouts: items, err: err}
	}
}
func (m *Model) finishLayoutsLoad(msg layoutsLoadedMsg) {
	if !m.layouts.active || !m.layouts.loading || m.layouts.request != msg.request {
		return
	}
	m.layouts.loading = false
	if msg.err != nil {
		m.layouts.err = msg.err.Error()
	} else {
		m.layouts.layouts = msg.layouts
	}
	m.updateViewport()
}
func (m layoutLibrary) matches() []int {
	type ranked struct{ i, s int }
	var rs []ranked
	for i, l := range m.layouts {
		var b strings.Builder
		b.WriteString(l.Name)
		b.WriteByte(' ')
		b.WriteString(l.Directory)
		for _, w := range l.Windows {
			b.WriteByte(' ')
			b.WriteString(w.Name)
			for _, p := range w.Panes {
				b.WriteByte(' ')
				b.WriteString(string(p.Type))
				b.WriteByte(' ')
				b.WriteString(p.Provider)
			}
		}
		if s, ok := fuzzyScore(m.query, b.String()); ok {
			rs = append(rs, ranked{i, s})
		}
	}
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j].s > rs[j-1].s; j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
	out := make([]int, len(rs))
	for i, r := range rs {
		out[i] = r.i
	}
	return out
}
func layoutTemplate() string {
	return `{"name":"Review","directory":"/absolute/project/path","windows":[{"name":"Code","panes":[{"type":"agent","provider":"codex"},{"type":"shell","split":"vertical","size_percent":40}]},{"name":"Notes","panes":[{"type":"shell"}]}]}`
}
func (m *Model) layoutKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.layouts.active {
		return false, nil
	}
	l := &m.layouts
	if msg.String() == "ctrl+c" {
		if l.launching {
			return true, nil
		}
		m.quitting = true
		return true, tea.Quit
	}
	if l.loading || l.saving || l.launching {
		if msg.String() == "esc" && l.loading {
			*l = layoutLibrary{}
			m.updateViewport()
		}
		return true, nil
	}
	if l.viewing {
		if l.confirmRun {
			switch msg.String() {
			case "esc", "n":
				l.confirmRun = false
				l.err = "Launch cancelled."
			case "y":
				return true, m.launchLayout()
			}
		} else {
			switch msg.String() {
			case "esc":
				l.viewing = false
			case "l":
				l.confirmRun = true
				l.err = ""
			}
		}
		m.updateViewport()
		return true, nil
	}
	if l.creating {
		return true, m.layoutEditorKey(msg)
	}
	matches := l.matches()
	switch msg.String() {
	case "esc":
		*l = layoutLibrary{}
	case "n", "N":
		l.creating = true
		l.source = layoutTemplate()
		l.position = len([]rune(l.source))
		l.err = ""
	case "up", "ctrl+p", "k":
		if l.selected > 0 {
			l.selected--
		}
	case "down", "ctrl+n", "j":
		if l.selected+1 < len(matches) {
			l.selected++
		}
	case "enter":
		if len(matches) > 0 {
			idx := matches[min(l.selected, len(matches)-1)]
			l.viewing = true
			l.preview = idx
			m.updateViewport()
			return true, nil
		}
	case "backspace", "ctrl+h":
		r := []rune(l.query)
		if len(r) > 0 {
			l.query = string(r[:len(r)-1])
			l.selected = 0
		}
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			input := msg.Runes
			if msg.Type == tea.KeySpace {
				input = []rune{' '}
			}
			for _, r := range input {
				if !unicode.IsControl(r) && len(l.query) < 512 {
					l.query += string(r)
				}
			}
			l.selected = 0
		}
	}
	m.updateViewport()
	return true, nil
}
func (m *Model) layoutEditorKey(msg tea.KeyMsg) tea.Cmd {
	l := &m.layouts
	r := []rune(l.source)
	switch msg.String() {
	case "esc":
		l.creating = false
		l.err = ""
	case "ctrl+s":
		parsed, err := store.Parse([]byte(`{"version":1,"layouts":[` + l.source + `]}`))
		if err != nil {
			l.err = err.Error()
			break
		}
		if len(parsed) != 1 {
			l.err = "Enter exactly one layout object."
			break
		}
		item := parsed[0]
		l.saving = true
		l.request++
		req := l.request
		doc := item
		m.updateViewport()
		return func() tea.Msg {
			path, err := store.DefaultPath()
			if err == nil {
				err = store.Save(path, doc)
			}
			return layoutSavedMsg{request: req, layout: doc, err: err}
		}
	case "left":
		l.position = max(0, l.position-1)
	case "right":
		l.position = min(len(r), l.position+1)
	case "home", "ctrl+a":
		l.position = lineStart(r, l.position)
	case "end", "ctrl+e":
		l.position = lineEnd(r, l.position)
	case "up":
		start := lineStart(r, l.position)
		if start > 0 {
			col := l.position - start
			prev := lineStart(r, start-1)
			l.position = min(prev+col, start-1)
		}
	case "down":
		end := lineEnd(r, l.position)
		if end < len(r) {
			col := l.position - lineStart(r, l.position)
			l.position = min(end+1+col, lineEnd(r, end+1))
		}
	case "backspace", "ctrl+h":
		if l.position > 0 {
			l.source = string(append(r[:l.position-1], r[l.position:]...))
			l.position--
		}
	case "delete":
		if l.position < len(r) {
			l.source = string(append(r[:l.position], r[l.position+1:]...))
		}
	case "enter":
		insertLayoutText(l, []rune{'\n'})
	case "tab":
		insertLayoutText(l, []rune{'\t'})
	case "ctrl+u":
		l.source = ""
		l.position = 0
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			in := msg.Runes
			if msg.Type == tea.KeySpace {
				in = []rune{' '}
			}
			clean := []rune{}
			for _, c := range in {
				if !unicode.IsControl(c) || c == '\n' || c == '\t' {
					clean = append(clean, c)
				}
			}
			insertLayoutText(l, clean)
		}
	}
	m.updateViewport()
	return nil
}
func insertLayoutText(l *layoutLibrary, in []rune) {
	r := []rune(l.source)
	if len(string(r))+len(string(in)) > store.MaxFileBytes {
		l.err = "Layout JSON exceeds 64 KiB."
		return
	}
	n := append([]rune{}, r[:l.position]...)
	n = append(n, in...)
	n = append(n, r[l.position:]...)
	l.source = string(n)
	l.position += len(in)
	l.err = ""
}
func (m *Model) finishLayoutSave(msg layoutSavedMsg) {
	l := &m.layouts
	if !l.active || !l.creating || !l.saving || l.request != msg.request {
		return
	}
	l.saving = false
	if msg.err != nil {
		l.err = msg.err.Error()
	} else {
		l.creating = false
		l.layouts = append(l.layouts, msg.layout)
		l.err = "Layout saved locally."
	}
	m.updateViewport()
}
func (m *Model) launchLayout() tea.Cmd {
	l := &m.layouts
	if l.preview < 0 || l.preview >= len(l.layouts) {
		l.confirmRun = false
		l.err = "Selected layout is unavailable."
		return nil
	}
	creator, ok := m.backend.(tmux.LayoutLifecycle)
	if !ok {
		l.confirmRun = false
		l.err = "This transport cannot create tmux layouts."
		return nil
	}
	selected := l.layouts[l.preview]
	l.confirmRun = false
	l.launching = true
	l.launchRequest++
	request := l.launchRequest
	m.updateViewport()
	return func() tea.Msg {
		path, err := store.DefaultPath()
		var saved []store.Layout
		if err == nil {
			saved, err = store.Load(path)
		}
		if err == nil {
			found := false
			for _, item := range saved {
				if item.Name == selected.Name {
					found = true
					if !reflect.DeepEqual(item, selected) {
						err = fmt.Errorf("saved layout changed after preview; reopen it before launching")
					}
					break
				}
			}
			if !found {
				err = fmt.Errorf("saved layout was removed after preview; reopen the layout browser")
			}
		}
		result := tmux.LayoutResult{}
		if err == nil {
			result, err = creator.CreateLayout(selected)
			if err != nil && (result.SessionName != "" || result.SessionID != "") {
				if panes, listErr := m.backend.ListAllPanes(); listErr == nil {
					result.Panes = reconcileLayoutPanes(result, panes)
				}
			}
		}
		return layoutLaunchedMsg{request: request, layout: selected, result: result, err: err}
	}
}

func reconcileLayoutPanes(result tmux.LayoutResult, listed []tmux.Pane) []tmux.Pane {
	panes := append([]tmux.Pane(nil), result.Panes...)
	seen := make(map[string]bool, len(panes))
	for _, pane := range panes {
		seen[pane.PaneID] = true
	}
	for _, pane := range listed {
		inSession := result.SessionID != "" && pane.SessionID == result.SessionID || result.SessionName != "" && pane.SessionName == result.SessionName
		if inSession && !seen[pane.PaneID] {
			panes = append(panes, pane)
			seen[pane.PaneID] = true
		}
	}
	return panes
}

func (m *Model) finishLayoutLaunch(msg layoutLaunchedMsg) tea.Cmd {
	l := &m.layouts
	if !l.active || !l.viewing || !l.launching || l.launchRequest != msg.request {
		return nil
	}
	l.launching = false
	if len(msg.result.Panes) == 0 {
		l.err = errorString(msg.err)
		m.updateViewport()
		return nil
	}
	m.syncFilterSource()
	added := make([]monitor.Session, 0, len(msg.result.Panes))
	for _, pane := range msg.result.Panes {
		session := monitor.Session{Pane: pane, Status: monitor.StatusUnknown, UpdatedAt: time.Now()}
		if pane.Shell {
			session.Kind = monitor.SessionShell
		} else {
			session.Provider = monitor.Provider(filepath.Base(pane.CurrentCommand))
		}
		added = append(added, session)
	}
	for _, session := range added {
		found := false
		for i := range m.allSessions {
			if m.allSessions[i].Pane.PaneID == session.Pane.PaneID {
				m.allSessions[i] = session
				found = true
				break
			}
		}
		if !found {
			m.allSessions = append(m.allSessions, session)
		}
	}
	m.filters.applied = filterChoice{}
	m.rebuildFilteredSessions(added[0].Pane.PaneID)
	m.layouts = layoutLibrary{}
	m.updateViewport()
	m.layoutNotice = layoutLaunchNotice(msg.layout, msg.result, msg.err)
	m.err = nil
	m.tmuxMissing = false
	m.refreshObsolete = m.refreshing
	return m.doRefresh()
}

func errorString(err error) string {
	if err == nil {
		return "Layout launch returned no panes."
	}
	return err.Error()
}

func layoutLaunchNotice(layout store.Layout, result tmux.LayoutResult, launchErr error) string {
	var b strings.Builder
	status := "created"
	if launchErr != nil {
		status = "partially created"
	}
	fmt.Fprintf(&b, "Layout %q %s: ", layout.Name, status)
	for i, pane := range result.Panes {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s/%s (%s)", pane.WindowName, pane.PaneID, pane.SessionName)
	}
	if launchErr != nil {
		fmt.Fprintf(&b, ". Failed: %s", launchErr.Error())
	} else {
		b.WriteString(". First pane selected.")
	}
	return b.String()
}

func (m Model) layoutBody() string {
	l := m.layouts
	if l.loading {
		return "Loading local layouts..."
	}
	if l.viewing && l.preview >= 0 && l.preview < len(l.layouts) {
		item := l.layouts[l.preview]
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n%s %s\n\n", m.theme.screenTitle(item.Name), m.theme.metadata("Directory:"), m.theme.metadata(item.Directory))
		for _, w := range item.Windows {
			fmt.Fprintf(&b, "%s\n", m.theme.sectionLabel("Window: "+w.Name))
			for i, p := range w.Panes {
				fmt.Fprintf(&b, "  %d. %s", i+1, p.Type)
				if p.Provider != "" {
					fmt.Fprintf(&b, " (%s)", p.Provider)
				}
				if p.Split != "" {
					fmt.Fprintf(&b, " - split %s", p.Split)
				}
				if p.SizePercent > 0 {
					fmt.Fprintf(&b, " at %d%%", p.SizePercent)
				}
				b.WriteByte('\n')
			}
			b.WriteByte('\n')
		}
		if l.err != "" {
			b.WriteString("\n\n" + m.theme.errorText(safeReviewText(l.err)))
		}
		if l.launching {
			b.WriteString("\n\nCreating layout. Keep Aeolus open until pane results are reported.")
		} else if l.confirmRun {
			b.WriteString("\n\n" + m.theme.errorText("CONFIRM LAYOUT CREATION") + "\n" + m.theme.metadata("This creates a detached tmux session and launches the configured agents and shells.") + "\n\n" + m.theme.primaryHint("Press y to create it, or n/Esc to cancel."))
		} else {
			b.WriteString("\n\n" + m.theme.primaryHint("Press l to launch this layout. A confirmation is required before tmux changes."))
		}
		return b.String()
	}
	if l.creating {
		r := []rune(l.source)
		pos := min(l.position, len(r))
		source := string(r[:pos]) + "▏" + string(r[pos:])
		label := "Create or preview local layout"
		if l.err == "Preview only - launch is handled in a later workflow." {
			label = "Layout preview (read only)"
		}
		body := m.theme.screenTitle(label) + "\n" + m.theme.metadata("Edit a version 1 JSON layout. Agent providers: claude, codex, gemini. First pane in each window has no split.") + "\n\n" + source
		if l.err != "" {
			body = m.theme.errorText("Error: "+safeReviewText(l.err)) + "\n\n" + body
		}
		return body
	}
	matches := l.matches()
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s %s\n\n", m.theme.sectionLabel(fmt.Sprintf("Saved layouts · %d", len(matches))), m.theme.sectionLabel("Search"), l.query)
	if l.err != "" {
		fmt.Fprintf(&b, "%s\n\n", m.theme.errorText(safeReviewText(l.err)))
	}
	if len(matches) == 0 {
		b.WriteString("No saved layouts. Press N to create one.\n")
	}
	for i, idx := range matches {
		mark := "  "
		if i == l.selected {
			mark = "> "
		}
		item := l.layouts[idx]
		panes := 0
		for _, w := range item.Windows {
			panes += len(w.Panes)
		}
		label := fmt.Sprintf("%s  %d windows, %d panes", item.Name, len(item.Windows), panes)
		if i == l.selected {
			b.WriteString(m.theme.selectedRow(label) + "\n")
		} else {
			fmt.Fprintf(&b, "%s%s\n", mark, label)
		}
		fmt.Fprintf(&b, "  %s\n", m.theme.metadata(ansi.Truncate(safeReviewText(item.Directory), max(1, m.viewport.Width-4), "…")))
	}
	return b.String()
}
func (m Model) layoutView() string {
	return fitScreenWithFooter(m.theme.screenTitle("Local tmux layouts")+"\n\n"+m.theme.renderer.NewStyle().Render(m.viewport.View())+"\n"+m.theme.primaryHint(m.tmuxLayoutFooter()), m.width, m.height)
}
func (m Model) tmuxLayoutFooter() string {
	if m.layouts.saving {
		return "Saving layout..."
	}
	if m.layouts.launching {
		return "Creating layout..."
	}
	if m.layouts.viewing && m.layouts.confirmRun {
		return "[y] Create layout  [n/Esc] Cancel"
	}
	if m.layouts.viewing {
		return "[l] Launch layout  [esc] Back to layouts"
	}
	if m.layouts.creating {
		return "[ctrl+s] Save locally  [esc] Back  [arrows] Edit JSON  [enter] New line"
	}
	return "[type] Fuzzy search  [↑↓/j/k] Select  [enter] Preview  [N] New  [esc] Close"
}
