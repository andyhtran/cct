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

// DiscoverFiles reads exactly one directory level deep under ~/.claude/projects/,
// matching Claude Code's current storage layout: <projects>/<dir>/<file>.jsonl.
func DiscoverFiles(projectFilter string) []string {
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
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") && f.Name() != "sessions-index.json" {
				files = append(files, filepath.Join(dirPath, f.Name()))
			}
		}
	}
	return files
}

func ScanAll(projectFilter string, fullParse bool) []*Session {
	files := DiscoverFiles(projectFilter)
	return ScanFiles(files, fullParse)
}

func ScanFiles(files []string, fullParse bool) []*Session {
	numWorkers := runtime.NumCPU()
	jobs := make(chan string, len(files))
	results := make(chan *Session, len(files))
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				var s *Session
				if fullParse {
					s = ParseFullSession(path)
				} else {
					s = ExtractMetadata(path)
				}
				if s != nil {
					results <- s
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

	var sessions []*Session
	for s := range results {
		sessions = append(sessions, s)
	}
	return sessions
}

func SearchFiles(files []string, keyword string, snippetWidth int, maxMatches int) []*SearchResult {
	keyLower := strings.ToLower(keyword)
	numWorkers := runtime.NumCPU()
	jobs := make(chan string, len(files))
	results := make(chan *SearchResult, len(files))
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				r := searchOneFile(path, keyLower, snippetWidth, maxMatches)
				if r != nil {
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

	var out []*SearchResult
	for r := range results {
		out = append(out, r)
	}
	return out
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

	scanner := NewJSONLScanner(f)

	var matches []string

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
			extractUserMetadata(s, obj)
		}

		if maxMatches > 0 && len(matches) >= maxMatches {
			continue
		}

		text := ExtractPromptText(obj)
		if text == "" {
			continue
		}

		if strings.Contains(strings.ToLower(text), keyLower) {
			snippet := output.ExtractSnippet(text, keyLower, snippetWidth)
			matches = append(matches, snippet)
		}
	}
	// scanner.Err() intentionally not checked — partial results are acceptable.

	if len(matches) == 0 {
		return nil
	}

	return &SearchResult{Session: s, Matches: matches}
}
