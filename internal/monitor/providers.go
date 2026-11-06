package monitor

import (
	"path/filepath"
	"strings"
)

// Provider identifies the CLI running in a pane.
type Provider string

const (
	ProviderUnknown Provider = ""
	ProviderClaude  Provider = "claude"
	ProviderCodex   Provider = "codex"
	ProviderGemini  Provider = "gemini"
)

func (p Provider) String() string {
	switch p {
	case ProviderClaude:
		return "Claude"
	case ProviderCodex:
		return "Codex"
	case ProviderGemini:
		return "Gemini"
	default:
		return "Unknown"
	}
}

// CanRespond reports whether the provider supports the dashboard's response keys.
func (s Session) CanRespond() bool { return s.Provider == ProviderClaude }

type providerAdapter struct {
	provider    Provider
	executables []string
	nodeScripts []string
	detect      func(string) (Status, *PermissionRequest, *QuestionRequest)
}

// Unknown states intentionally expose no provider-specific actions.
func unknownStatus(string) (Status, *PermissionRequest, *QuestionRequest) {
	return StatusUnknown, nil, nil
}

var providerAdapters = []providerAdapter{
	{ProviderClaude, []string{"claude", "claude-code"}, []string{"/@anthropic-ai/claude-code/cli.js"}, detectClaudeStatus},
	{ProviderCodex, []string{"codex"}, []string{"/@openai/codex/bin/codex.js"}, detectCodexStatus},
	{ProviderGemini, []string{"gemini"}, []string{"/@google/gemini-cli/dist/index.js"}, unknownStatus},
}

func providerForProcess(args string) Provider {
	fields := strings.Fields(strings.ToLower(args))
	if len(fields) == 0 {
		return ProviderUnknown
	}
	executable := filepath.Base(fields[0])
	for _, adapter := range providerAdapters {
		for _, name := range adapter.executables {
			if executable == name {
				return adapter.provider
			}
		}
		// Only inspect the script position of a Node invocation. Never inspect arbitrary
		// arguments, shell command strings, prompts, or node -e expressions.
		if executable == "node" || executable == "nodejs" {
			if len(fields) < 2 || strings.HasPrefix(fields[1], "-") {
				continue
			}
			for _, name := range adapter.executables {
				if filepath.Base(fields[1]) == name {
					return adapter.provider
				}
			}
			for _, script := range adapter.nodeScripts {
				if strings.HasSuffix(fields[1], script) {
					return adapter.provider
				}
			}
		}
	}
	return ProviderUnknown
}

// paneProvider chooses the nearest recognized process to the pane root. This
// avoids reclassifying a parent agent when it starts another agent as a tool.
func paneProvider(rootPID int, procs []processInfo, currentCommand string) Provider {
	byPID := make(map[int]processInfo, len(procs))
	for _, proc := range procs {
		byPID[proc.PID] = proc
	}
	children := buildChildMap(procs)
	queue := []int{rootPID}
	visited := make(map[int]bool)
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if visited[pid] {
			continue
		}
		visited[pid] = true
		if proc, ok := byPID[pid]; ok {
			if provider := providerForProcess(proc.Args); provider != ProviderUnknown {
				return provider
			}
		}
		queue = append(queue, children[pid]...)
	}
	return providerForProcess(currentCommand)
}

func detectProviderStatus(provider Provider, content string) (Status, *PermissionRequest, *QuestionRequest) {
	for _, adapter := range providerAdapters {
		if adapter.provider == provider {
			return adapter.detect(content)
		}
	}
	return unknownStatus(content)
}
