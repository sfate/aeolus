// Package notebooks stores versioned local Markdown runbooks with explicit shell blocks.
package notebooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

const (
	MaxDocuments = 128
	MaxBlocks    = 64
	MaxTextBytes = 64 << 10
	MaxFileBytes = 1 << 20
)

type BlockKind string

const (
	Markdown BlockKind = "markdown"
	Shell    BlockKind = "shell"
)

type Block struct {
	Kind    BlockKind `json:"kind"`
	Content string    `json:"content"`
}

type Document struct {
	Title  string  `json:"title"`
	Blocks []Block `json:"blocks"`
}

type library struct {
	Version   int        `json:"version"`
	Documents []Document `json:"documents"`
}

func DefaultPath() (string, error) {
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("XDG_DATA_HOME must be an absolute path")
	}
	return filepath.Join(root, "aeolus", "notebooks.json"), nil
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

func validateDocument(document Document) error {
	if !utf8.ValidString(document.Title) || strings.TrimSpace(document.Title) == "" || strings.TrimSpace(document.Title) != document.Title || len([]rune(document.Title)) > 80 {
		return fmt.Errorf("notebook title must contain 1-80 characters without surrounding whitespace")
	}
	for _, r := range document.Title {
		if unicode.IsControl(r) {
			return fmt.Errorf("notebook title contains a control character")
		}
	}
	if len(document.Blocks) == 0 || len(document.Blocks) > MaxBlocks {
		return fmt.Errorf("notebook must contain 1-%d blocks", MaxBlocks)
	}
	for _, block := range document.Blocks {
		if !validText(block.Content) || strings.TrimSpace(block.Content) == "" {
			return fmt.Errorf("notebook block must be nonempty UTF-8 up to %d bytes without terminal controls", MaxTextBytes)
		}
		if block.Kind != Markdown && block.Kind != Shell {
			return fmt.Errorf("unsupported notebook block kind %q", block.Kind)
		}
	}
	return nil
}

func ParseMarkdown(title, source string) (Document, error) {
	if !validText(source) {
		return Document{}, fmt.Errorf("notebook Markdown must be UTF-8 up to %d bytes without terminal controls", MaxTextBytes)
	}
	lines := strings.Split(source, "\n")
	blocks := make([]Block, 0)
	var markdown, command strings.Builder
	inShell := false
	flushMarkdown := func() {
		content := strings.TrimSpace(markdown.String())
		if content != "" {
			blocks = append(blocks, Block{Kind: Markdown, Content: content})
		}
		markdown.Reset()
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inShell && (trimmed == "```sh" || trimmed == "```bash" || trimmed == "```shell") {
			flushMarkdown()
			inShell = true
			command.Reset()
			continue
		}
		if inShell && trimmed == "```" {
			content := strings.TrimSpace(command.String())
			if content == "" {
				return Document{}, fmt.Errorf("shell code blocks must contain a command")
			}
			blocks = append(blocks, Block{Kind: Shell, Content: content})
			inShell = false
			continue
		}
		if inShell {
			if command.Len() > 0 {
				command.WriteByte('\n')
			}
			command.WriteString(line)
		} else {
			if markdown.Len() > 0 {
				markdown.WriteByte('\n')
			}
			markdown.WriteString(line)
		}
		if len(blocks) > MaxBlocks {
			return Document{}, fmt.Errorf("notebook exceeds %d blocks", MaxBlocks)
		}
	}
	if inShell {
		return Document{}, fmt.Errorf("unclosed shell code block")
	}
	flushMarkdown()
	document := Document{Title: title, Blocks: blocks}
	if err := validateDocument(document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func jsonObject(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("expected a JSON object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("invalid JSON field")
		}
		if _, exists := fields[name]; exists {
			return nil, fmt.Errorf("duplicate JSON field %q", name)
		}
		known := false
		for _, candidate := range allowed {
			if name == candidate {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("unknown JSON field %q", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing JSON content")
	}
	return fields, nil
}

func Load(path string) ([]Document, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("notebook path must be absolute")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return []Document{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open notebooks: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > MaxFileBytes {
		return nil, fmt.Errorf("notebook store must be a private regular file no larger than 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if err != nil || len(data) > MaxFileBytes || !utf8.Valid(data) {
		return nil, fmt.Errorf("notebook store exceeds its size limit or is not UTF-8")
	}
	fields, err := jsonObject(data, "version", "documents")
	if err != nil {
		return nil, err
	}
	var version int
	if json.Unmarshal(fields["version"], &version) != nil || version != 1 {
		return nil, fmt.Errorf("notebook store version must be 1")
	}
	var raw []json.RawMessage
	if json.Unmarshal(fields["documents"], &raw) != nil || raw == nil || len(raw) > MaxDocuments {
		return nil, fmt.Errorf("notebook store requires a documents array of at most %d items", MaxDocuments)
	}
	documents := make([]Document, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		fields, err := jsonObject(item, "title", "blocks")
		if err != nil {
			return nil, err
		}
		var document Document
		var rawBlocks []json.RawMessage
		if json.Unmarshal(fields["title"], &document.Title) != nil || json.Unmarshal(fields["blocks"], &rawBlocks) != nil || rawBlocks == nil {
			return nil, fmt.Errorf("notebook title and blocks have invalid types")
		}
		for _, rawBlock := range rawBlocks {
			blockFields, err := jsonObject(rawBlock, "kind", "content")
			if err != nil {
				return nil, err
			}
			var block Block
			if json.Unmarshal(blockFields["kind"], &block.Kind) != nil || json.Unmarshal(blockFields["content"], &block.Content) != nil {
				return nil, fmt.Errorf("notebook block kind and content have invalid types")
			}
			document.Blocks = append(document.Blocks, block)
		}
		if err := validateDocument(document); err != nil {
			return nil, err
		}
		if seen[document.Title] {
			return nil, fmt.Errorf("notebook title %q is duplicated", document.Title)
		}
		seen[document.Title] = true
		documents = append(documents, document)
	}
	return documents, nil
}

func Save(path string, document Document) error {
	if err := validateDocument(document); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("notebook path must be absolute")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return fmt.Errorf("open notebook lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	info, err := lock.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return fmt.Errorf("notebook lock must be a private regular file")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("notebook store is busy: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	documents, err := Load(path)
	if err != nil {
		return err
	}
	for _, existing := range documents {
		if existing.Title == document.Title {
			return fmt.Errorf("notebook title %q already exists", document.Title)
		}
	}
	if len(documents) >= MaxDocuments {
		return fmt.Errorf("notebook store is full (%d notebooks)", MaxDocuments)
	}
	documents = append(documents, document)
	return saveDocuments(dir, path, documents)
}

func saveDocuments(dir, path string, documents []Document) error {
	data, err := json.MarshalIndent(library{Version: 1, Documents: documents}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > MaxFileBytes {
		return fmt.Errorf("notebook store exceeds 1 MiB")
	}
	temp, err := os.CreateTemp(dir, ".notebooks-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() { _ = temp.Close(); _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
