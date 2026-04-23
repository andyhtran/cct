package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/paths"
)

// DiscoverFiles walks ~/.claude/projects/ at two depths:
//   - flat parent sessions: <projects>/<projectDir>/<sessionID>.jsonl
//   - nested subagents:     <projects>/<projectDir>/<parentSessionID>/subagents/agent-*.jsonl
//
// Claude Code moved subagents from the flat layout to the nested one at some
// point; both can coexist. Nested scanning only runs when includeAgents=true,
// since every nested entry is an agent by construction.
func DiscoverFiles(projectFilter string, includeAgents bool) []string {
	dir := paths.ProjectsDir()
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "cct: cannot read %s: %v\n", dir, err)
		}
		return nil
	}

	filterLower := strings.ToLower(projectFilter)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()

		if projectFilter != "" && !strings.Contains(strings.ToLower(dirName), filterLower) {
			continue
		}

		dirPath := filepath.Join(dir, dirName)
		dirEntries, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}
		for _, f := range dirEntries {
			name := f.Name()
			if f.IsDir() {
				if !includeAgents {
					continue
				}
				files = append(files, discoverNestedSubagents(filepath.Join(dirPath, name))...)
				continue
			}
			if !strings.HasSuffix(name, ".jsonl") || name == "sessions-index.json" {
				continue
			}
			if !includeAgents && strings.HasPrefix(name, "agent-") {
				continue
			}
			files = append(files, filepath.Join(dirPath, name))
		}
	}
	return files
}

// discoverNestedSubagents returns agent-*.jsonl paths under <sessionDir>/subagents/.
// Sibling dirs like tool-results/ are intentionally ignored — only subagents/
// holds session-shaped JSONL.
func discoverNestedSubagents(sessionDir string) []string {
	subDir := filepath.Join(sessionDir, "subagents")
	entries, err := os.ReadDir(subDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, f := range entries {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		out = append(out, filepath.Join(subDir, name))
	}
	return out
}

func ScanAll(projectFilter string, fullParse bool, includeAgents bool) []*Session {
	files := DiscoverFiles(projectFilter, includeAgents)
	return ScanFiles(files, fullParse)
}

// parallelMap applies fn to each file path using a worker pool and collects non-nil results.
func parallelMap[T any](files []string, fn func(string) *T) []*T {
	numWorkers := runtime.NumCPU()
	jobs := make(chan string, len(files))
	results := make(chan *T, len(files))
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				if r := fn(path); r != nil {
					results <- r
				}
			}
		}()
	}

	for _, f := range files {
		jobs <- f
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var out []*T
	for r := range results {
		out = append(out, r)
	}
	return out
}

func ScanFiles(files []string, fullParse bool) []*Session {
	parse := ExtractMetadata
	if fullParse {
		parse = ParseFullSession
	}
	return parallelMap(files, parse)
}

func SearchFiles(files []string, keyword string, snippetWidth int, maxMatches int) []*SearchResult {
	keyLower := strings.ToLower(keyword)
	return parallelMap(files, func(path string) *SearchResult {
		return searchOneFile(path, keyLower, snippetWidth, maxMatches)
	})
}

func searchOneFile(path, keyLower string, snippetWidth int, maxMatches int) *SearchResult {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil
	}
	s := &Session{
		ID:       ExtractIDFromFilename(path),
		FilePath: path,
		Modified: info.ModTime(),
	}
	s.ShortID = ShortID(s.ID)
	s.IsAgent = IsAgentSession(s.ID)

	terms := strings.Fields(keyLower)
	isPhrase := len(terms) <= 1

	scanner := NewJSONLScanner(f)

	var matches []Match
	var termSeen []bool
	if !isPhrase {
		termSeen = make([]bool, len(terms))
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		lineType := FastExtractType(line)

		if lineType != "user" && lineType != "assistant" {
			continue
		}

		var obj map[string]any
		if json.Unmarshal(line, &obj) != nil {
			continue
		}

		if lineType == "user" {
			ExtractUserMetadata(s, obj)
		}

		if maxMatches > 0 && len(matches) >= maxMatches {
			continue
		}

		blocks := ExtractPromptBlocks(obj)
		if len(blocks) == 0 {
			continue
		}

		for _, block := range blocks {
			if maxMatches > 0 && len(matches) >= maxMatches {
				break
			}

			text := block.Text
			textLower := strings.ToLower(text)

			roleWidth := len(lineType) + 3 // "[x] " prefix
			if block.Source != "" {
				roleWidth += len(block.Source) + 1 // ":Tool" suffix
			}
			sw := snippetWidth - roleWidth

			if isPhrase {
				if strings.Contains(textLower, keyLower) {
					snippet := output.ExtractSnippet(text, keyLower, sw)
					matches = append(matches, Match{Role: lineType, Source: block.Source, Snippet: snippet})
				}
				continue
			}

			// Multi-term: check which terms appear in this block.
			var bestTerm string
			for i, term := range terms {
				if strings.Contains(textLower, term) {
					termSeen[i] = true
					if bestTerm == "" || len(term) > len(bestTerm) {
						bestTerm = term
					}
				}
			}
			if bestTerm != "" {
				snippet := output.ExtractSnippet(text, bestTerm, sw)
				matches = append(matches, Match{Role: lineType, Source: block.Source, Snippet: snippet})
			}
		}
	}
	// scanner.Err() intentionally not checked — partial results are acceptable.

	if isPhrase {
		if len(matches) == 0 {
			return nil
		}
		return &SearchResult{Session: s, Matches: matches}
	}

	// Multi-term: only return results where ALL terms appeared somewhere in the session.
	allSeen := true
	for _, seen := range termSeen {
		if !seen {
			allSeen = false
			break
		}
	}
	if !allSeen || len(matches) == 0 {
		return nil
	}

	return &SearchResult{Session: s, Matches: matches}
}
