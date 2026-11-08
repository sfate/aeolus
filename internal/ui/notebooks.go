package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sfate/aeolus/internal/monitor"
	store "github.com/sfate/aeolus/internal/notebooks"
)

type notebookOperations struct {
	path func() (string, error)
	load func(string) ([]store.Document, error)
	save func(string, store.Document) error
}

type notebookLibrary struct {
	active, loading, viewing, creating bool
	editingTitle, saving               bool
	confirmRun, running                bool
	request, saveRequest, runRequest   uint64
	paneID                             string
	provider                           monitor.Provider
	kind                               monitor.SessionKind
	documents                          []store.Document
	query                              string
	selected, document, command        int
	title, source                      string
	position                           int
	err, notice                        string
}

type notebooksLoadedMsg struct {
	request uint64
	paneID  string
	docs    []store.Document
	err     error
}

type notebookSavedMsg struct {
	request uint64
	paneID  string
	doc     store.Document
	err     error
}

type notebookRunMsg struct {
	request uint64
	paneID  string
	title   string
	err     error
}

func (m *Model) openNotebookBrowser() tea.Cmd {
	var paneID string
	var provider monitor.Provider
	var kind monitor.SessionKind
	if m.composer.active {
		paneID, provider, kind = m.composer.paneID, m.composer.provider, m.composer.kind
	} else if m.cursor >= 0 && m.cursor < len(m.sessions) {
		s := m.sessions[m.cursor]
		paneID, provider, kind = s.Pane.PaneID, s.Provider, s.Kind
	}
	if paneID == "" || kind != monitor.SessionShell {
		return nil
	}
	m.notebookSeq++
	m.notebooks = notebookLibrary{active: true, loading: true, request: m.notebookSeq, paneID: paneID, provider: provider, kind: kind, command: -1}
	m.viewport.GotoTop()
	m.updateViewport()
	request, target := m.notebooks.request, m.notebooks.paneID
	ops := m.notebookIO
	if ops.path == nil {
		ops.path = store.DefaultPath
	}
	if ops.load == nil {
		ops.load = store.Load
	}
	return func() tea.Msg {
		path, err := ops.path()
		var docs []store.Document
		if err == nil {
			docs, err = ops.load(path)
		}
		return notebooksLoadedMsg{request: request, paneID: target, docs: docs, err: err}
	}
}

func (m Model) notebookTargetAvailable() bool {
	sessions := m.allSessions
	if sessions == nil {
		sessions = m.sessions
	}
	for _, session := range sessions {
		if session.Pane.PaneID == m.notebooks.paneID && session.Provider == m.notebooks.provider && session.Kind == m.notebooks.kind {
			return true
		}
	}
	return false
}

func (m *Model) finishNotebooksLoad(msg notebooksLoadedMsg) {
	if !m.notebooks.active || !m.notebooks.loading || m.notebooks.request != msg.request || m.notebooks.paneID != msg.paneID {
		return
	}
	m.notebooks.loading = false
	m.notebooks.err = ""
	if msg.err != nil {
		m.notebooks.err = msg.err.Error()
	} else {
		m.notebooks.documents = msg.docs
	}
	m.updateViewport()
}

func (m notebookLibrary) matches() []int {
	type ranked struct{ index, score int }
	matches := make([]ranked, 0, len(m.documents))
	for i, document := range m.documents {
		var fields strings.Builder
		fields.WriteString(document.Title)
		for _, block := range document.Blocks {
			fields.WriteByte(' ')
			fields.WriteString(block.Content)
		}
		if score, ok := fuzzyScore(m.query, fields.String()); ok {
			matches = append(matches, ranked{i, score})
		}
	}
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].score > matches[j-1].score; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}
	indices := make([]int, len(matches))
	for i, match := range matches {
		indices[i] = match.index
	}
	return indices
}

func shellBlockIndexes(document store.Document) []int {
	indices := make([]int, 0)
	for i, block := range document.Blocks {
		if block.Kind == store.Shell {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m *Model) notebookKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.notebooks.active {
		return false, nil
	}
	n := &m.notebooks
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return true, tea.Quit
	}
	if n.running || n.loading || n.saving {
		if msg.String() == "esc" && !n.running && !n.saving {
			*n = notebookLibrary{}
			m.updateViewport()
		}
		return true, nil
	}
	if n.confirmRun {
		switch msg.String() {
		case "esc", "n":
			n.confirmRun = false
			n.notice = "Run cancelled."
		case "y":
			if !m.notebookTargetAvailable() {
				n.confirmRun = false
				n.err = "Target shell changed or disappeared; command was not sent."
				break
			}
			sender, ok := m.backend.(textSender)
			if !ok {
				n.confirmRun = false
				n.err = "This transport cannot run shell commands."
				break
			}
			if n.document < 0 || n.document >= len(n.documents) {
				n.confirmRun = false
				n.err = "Notebook is no longer available."
				break
			}
			document := n.documents[n.document]
			indices := shellBlockIndexes(document)
			if n.command < 0 || n.command >= len(indices) {
				n.confirmRun = false
				n.err = "Selected command block is unavailable."
				break
			}
			block := document.Blocks[indices[n.command]]
			n.confirmRun, n.running, n.err = false, true, ""
			n.runRequest++
			request, target, title, command := n.runRequest, n.paneID, document.Title, block.Content
			m.updateViewport()
			return true, func() tea.Msg {
				return notebookRunMsg{request: request, paneID: target, title: title, err: sender.SendText(target, command)}
			}
		default:
			return true, nil
		}
		m.updateViewport()
		return true, nil
	}
	if n.creating {
		return true, m.notebookEditKey(msg)
	}
	if n.viewing {
		switch msg.String() {
		case "esc":
			n.viewing = false
			n.command = -1
			n.notice = ""
		case "up", "k":
			if n.command > 0 {
				n.command--
			}
		case "down", "j":
			if n.document < len(n.documents) {
				commands := shellBlockIndexes(n.documents[n.document])
				if n.command+1 < len(commands) {
					n.command++
				}
			}
		case "r":
			if n.document >= len(n.documents) {
				n.err = "Notebook is no longer available."
			} else if n.command < 0 || n.command >= len(shellBlockIndexes(n.documents[n.document])) {
				n.err = "Select a shell command block first."
			} else if !m.notebookTargetAvailable() {
				n.err = "Target shell changed or disappeared; command was not sent."
			} else {
				n.confirmRun = true
				n.err, n.notice = "", ""
			}
		case "pgup":
			m.viewport.SetYOffset(m.viewport.YOffset - m.viewport.Height)
		case "pgdown":
			m.viewport.SetYOffset(m.viewport.YOffset + m.viewport.Height)
		}
		m.updateViewport()
		return true, nil
	}
	switch msg.String() {
	case "esc":
		*n = notebookLibrary{}
		m.updateViewport()
	case "ctrl+c":
		m.quitting = true
		return true, tea.Quit
	case "up", "ctrl+p":
		if n.selected > 0 {
			n.selected--
		}
	case "down", "ctrl+n":
		if n.selected+1 < len(n.matches()) {
			n.selected++
		}
	case "enter":
		matches := n.matches()
		if n.selected < len(matches) {
			n.document = matches[n.selected]
			n.viewing = true
			n.command = -1
			if commands := shellBlockIndexes(n.documents[n.document]); len(commands) > 0 {
				n.command = 0
			}
			n.err, n.notice = "", ""
		}
	case "N":
		n.creating, n.editingTitle = true, true
		n.title, n.source = "", ""
		n.position, n.err, n.notice = 0, "", ""
	case "r":
		return true, m.openNotebookBrowser()
	case "backspace", "ctrl+h":
		query := []rune(n.query)
		if len(query) > 0 {
			n.query = string(query[:len(query)-1])
			n.selected = 0
		}
	case "ctrl+u":
		n.query, n.selected = "", 0
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			input := msg.Runes
			if msg.Type == tea.KeySpace {
				input = []rune{' '}
			}
			for _, r := range input {
				if !unicode.IsControl(r) && len(n.query) < 512 {
					n.query += string(r)
				}
			}
			n.selected = 0
		}
	}
	m.updateViewport()
	return true, nil
}

func (m *Model) notebookEditKey(msg tea.KeyMsg) tea.Cmd {
	n := &m.notebooks
	if n.editingTitle {
		runes := []rune(n.title)
		switch msg.String() {
		case "esc":
			n.creating, n.editingTitle = false, false
		case "enter":
			if strings.TrimSpace(n.title) == "" {
				n.err = "Enter a notebook title."
			} else {
				n.editingTitle = false
				n.source = "# " + n.title + "\n\n"
				n.position = len([]rune(n.source))
				n.err = ""
			}
		case "left":
			n.position = max(0, n.position-1)
		case "right":
			n.position = min(len(runes), n.position+1)
		case "home", "ctrl+a":
			n.position = 0
		case "end", "ctrl+e":
			n.position = len(runes)
		case "backspace", "ctrl+h":
			if n.position > 0 {
				n.title = string(append(runes[:n.position-1], runes[n.position:]...))
				n.position--
			}
		case "delete":
			if n.position < len(runes) {
				n.title = string(append(runes[:n.position], runes[n.position+1:]...))
			}
		case "ctrl+u":
			n.title, n.position = "", 0
		default:
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
				input := msg.Runes
				if msg.Type == tea.KeySpace {
					input = []rune{' '}
				}
				clean := make([]rune, 0, len(input))
				for _, r := range input {
					if !unicode.IsControl(r) {
						clean = append(clean, r)
					}
				}
				if len(runes)+len(clean) <= 80 {
					next := append([]rune{}, runes[:n.position]...)
					next = append(next, clean...)
					next = append(next, runes[n.position:]...)
					n.title, n.position = string(next), n.position+len(clean)
				}
			}
		}
	} else {
		return m.notebookSourceKey(msg)
	}
	m.updateViewport()
	return nil
}

func (m *Model) notebookSourceKey(msg tea.KeyMsg) tea.Cmd {
	n := &m.notebooks
	runes := []rune(n.source)
	switch msg.String() {
	case "esc":
		n.creating = false
	case "ctrl+s":
		document, err := store.ParseMarkdown(n.title, n.source)
		if err != nil {
			n.err = err.Error()
			break
		}
		ops := m.notebookIO
		if ops.path == nil {
			ops.path = store.DefaultPath
		}
		if ops.save == nil {
			ops.save = store.Save
		}
		n.saving = true
		n.saveRequest++
		request, target, doc := n.saveRequest, n.paneID, document
		m.updateViewport()
		return func() tea.Msg {
			path, err := ops.path()
			if err == nil {
				err = ops.save(path, doc)
			}
			return notebookSavedMsg{request: request, paneID: target, doc: doc, err: err}
		}
	case "left":
		n.position = max(0, n.position-1)
	case "right":
		n.position = min(len(runes), n.position+1)
	case "up":
		start := lineStart(runes, n.position)
		if start > 0 {
			column := n.position - start
			previous := lineStart(runes, start-1)
			n.position = min(previous+column, start-1)
		}
	case "down":
		end := lineEnd(runes, n.position)
		if end < len(runes) {
			column := n.position - lineStart(runes, n.position)
			n.position = min(end+1+column, lineEnd(runes, end+1))
		}
	case "home", "ctrl+a":
		n.position = lineStart(runes, n.position)
	case "end", "ctrl+e":
		n.position = lineEnd(runes, n.position)
	case "backspace", "ctrl+h":
		if n.position > 0 {
			n.source = string(append(runes[:n.position-1], runes[n.position:]...))
			n.position--
		}
	case "delete":
		if n.position < len(runes) {
			n.source = string(append(runes[:n.position], runes[n.position+1:]...))
		}
	case "ctrl+u":
		n.source, n.position = "", 0
	case "enter":
		insertNotebookText(n, []rune{'\n'})
	case "tab":
		insertNotebookText(n, []rune{'\t'})
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			input := msg.Runes
			if msg.Type == tea.KeySpace {
				input = []rune{' '}
			}
			clean := make([]rune, 0, len(input))
			for _, r := range input {
				if !unicode.IsControl(r) || r == '\n' || r == '\t' {
					clean = append(clean, r)
				}
			}
			insertNotebookText(n, clean)
		}
	}
	m.updateViewport()
	return nil
}

func insertNotebookText(n *notebookLibrary, text []rune) {
	runes := []rune(n.source)
	if len(n.source)+len(string(text)) > store.MaxTextBytes {
		n.err = "Notebook text exceeds the 64 KiB limit."
		return
	}
	next := append([]rune{}, runes[:n.position]...)
	next = append(next, text...)
	next = append(next, runes[n.position:]...)
	n.source, n.position, n.err = string(next), n.position+len(text), ""
}

func (m *Model) finishNotebookSave(msg notebookSavedMsg) {
	n := &m.notebooks
	if !n.active || !n.creating || !n.saving || n.saveRequest != msg.request || n.paneID != msg.paneID {
		return
	}
	n.saving = false
	if msg.err != nil {
		n.err = msg.err.Error()
		m.updateViewport()
		return
	}
	n.documents = append(n.documents, msg.doc)
	n.query = ""
	n.selected = len(n.documents) - 1
	n.creating, n.editingTitle, n.viewing = false, false, false
	n.title, n.source, n.notice, n.err = "", "", "Notebook saved.", ""
	m.viewport.GotoTop()
	m.updateViewport()
}

func (m *Model) finishNotebookRun(msg notebookRunMsg) {
	n := &m.notebooks
	if !n.active || !n.running || n.runRequest != msg.request || n.paneID != msg.paneID {
		return
	}
	n.running = false
	if msg.err != nil {
		n.err = "Command was not sent successfully: " + msg.err.Error()
	} else {
		n.notice = "Command submitted to the selected shell."
		n.err = ""
	}
	m.updateViewport()
}

func (m *Model) resizeNotebookViewport() {
	m.viewport.Width = max(1, m.width-2)
	m.viewport.Height = max(1, m.height-6)
	m.viewport.SetYOffset(m.viewport.YOffset)
}

func (m Model) notebookBody() string {
	n := m.notebooks
	if n.loading {
		return "Loading local notebooks..."
	}
	if n.creating {
		var body string
		if n.editingTitle {
			runes := []rune(n.title)
			query := string(runes[:n.position]) + "▏" + string(runes[n.position:])
			body = m.theme.sectionLabel("Create local notebook") + "\n\nTitle: " + query + "\n\n" + m.theme.metadata("Notebook title is limited to 80 characters.")
		} else {
			runes := []rune(n.source)
			content := string(runes[:n.position]) + "▏" + string(runes[n.position:])
			body = "Create local notebook: " + n.title + "\n\nWrite Markdown notes and optional runnable blocks inside fenced ```sh blocks.\nSaving stores this document locally; it does not run commands.\n\n" + content
		}
		if n.err != "" {
			body = m.theme.errorText("Error: "+safeReviewText(n.err)) + "\n\n" + body
		}
		return body
	}
	if n.viewing && n.document < len(n.documents) {
		return m.notebookDocumentBody(n.documents[n.document])
	}
	if n.err != "" {
		return m.theme.errorText("Error: "+safeReviewText(n.err)) + "\n\n" + m.notebookListBody()
	}
	return m.notebookListBody()
}

func (m Model) notebookListBody() string {
	n := m.notebooks
	matches := n.matches()
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n%s %s\n\n", m.theme.sectionLabel(fmt.Sprintf("Saved notebooks · %d", len(matches))), m.theme.sectionLabel("Search"), n.query)
	if n.notice != "" {
		fmt.Fprintf(&text, "%s\n\n", m.theme.primaryHint(n.notice))
	}
	if len(matches) == 0 {
		text.WriteString("No matching notebooks. Press N to create one.\n")
	}
	for i, index := range matches {
		marker := "  "
		if i == n.selected {
			marker = "> "
		}
		document := n.documents[index]
		commands := 0
		for _, block := range document.Blocks {
			if block.Kind == store.Shell {
				commands++
			}
		}
		preview := ""
		if len(document.Blocks) > 0 {
			preview = strings.ReplaceAll(document.Blocks[0].Content, "\n", " ")
		}
		label := fmt.Sprintf("%s  %d sections, %d runnable blocks", document.Title, len(document.Blocks), commands)
		if i == n.selected {
			text.WriteString(m.theme.selectedRow(label) + "\n")
		} else {
			fmt.Fprintf(&text, "%s%s\n", marker, label)
		}
		fmt.Fprintf(&text, "  %s\n", m.theme.metadata(ansi.Truncate(safeReviewText(preview), max(1, m.viewport.Width-4), "…")))
	}
	return text.String()
}

func (m Model) notebookDocumentBody(document store.Document) string {
	n := m.notebooks
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n\n", m.theme.screenTitle(document.Title))
	commands := shellBlockIndexes(document)
	for index, block := range document.Blocks {
		if block.Kind == store.Markdown {
			text.WriteString(m.renderNotebookMarkdown(block.Content))
			text.WriteString("\n\n")
			continue
		}
		commandIndex := -1
		for i, sourceIndex := range commands {
			if sourceIndex == index {
				commandIndex = i
				break
			}
		}
		label := fmt.Sprintf("Runnable shell block %d  Press r to review and confirm", commandIndex+1)
		if commandIndex == n.command {
			text.WriteString(m.theme.selectedRow(label) + "\n")
		} else {
			text.WriteString(m.theme.sectionLabel(label) + "\n")
		}
		text.WriteString("```sh\n")
		text.WriteString(safeReviewText(block.Content))
		text.WriteString("\n```\n\n")
	}
	if n.confirmRun && n.command >= 0 && n.command < len(commands) {
		text.WriteString("CONFIRM COMMAND EXECUTION\nThis sends the saved command to the selected shell and can have side effects.\n\n")
		text.WriteString(safeReviewText(document.Blocks[commands[n.command]].Content))
		text.WriteString("\n\nPress y to run or n/Esc to cancel. The notebook will not change.\n")
	}
	if n.err != "" {
		fmt.Fprintf(&text, "\n%s\n", m.theme.errorText("Error: "+safeReviewText(n.err)))
	}
	if n.notice != "" {
		fmt.Fprintf(&text, "\n%s\n", m.theme.primaryHint(safeReviewText(n.notice)))
	}
	if n.running {
		text.WriteString("\nSubmitting command...\n")
	}
	return text.String()
}

func (m Model) renderNotebookMarkdown(markdown string) string {
	var rendered strings.Builder
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			rendered.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			rendered.WriteString(m.theme.sectionLabel(heading))
		} else if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			rendered.WriteString("• " + strings.TrimSpace(trimmed[2:]))
		} else {
			rendered.WriteString(strings.ReplaceAll(trimmed, "`", ""))
		}
		rendered.WriteByte('\n')
	}
	return rendered.String()
}

func (m Model) notebookView() string {
	header := m.theme.screenTitle("Local notebooks")
	body := m.theme.renderer.NewStyle().Render(m.viewport.View())
	footer := m.theme.primaryHint(m.notebookFooter())
	return fitScreenWithFooter(header+"\n\n"+body+"\n"+footer, m.width, m.height)
}

func (m Model) notebookFooter() string {
	n := m.notebooks
	switch {
	case n.running:
		return "Submitting command..."
	case n.saving:
		return "Saving notebook..."
	case n.confirmRun:
		return "[y] Run command  [n/Esc] Cancel"
	case n.creating && n.editingTitle:
		return "[enter] Continue  [esc] Cancel"
	case n.creating:
		return "[ctrl+s] Save locally  [esc] Discard  [arrows] Edit  [enter] New line"
	case n.viewing:
		return "[↑↓/j/k] Select command block  [r] Review and confirm run  [pgup/pgdown] Scroll  [esc] Back"
	default:
		return "[type] Fuzzy search  [↑↓/ctrl+p/ctrl+n] Select  [enter] Open  [N] New  [r] Refresh  [esc] Close"
	}
}
