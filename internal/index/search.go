package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/session"
)

type snippetLocation struct {
	sessionID  string
	filePath   string
	role       string
	source     string
	byteOffset int64
	byteLength int
	// inlineText is populated for synthetic rows (role="description") whose
	// source text lives on the sessions row, not in the JSONL file.
	inlineText string
}

type SearchOptions struct {
	Query         string
	ProjectFilter string
	IncludeAgents bool
	MaxResults    int
	MaxMatches    int
	SnippetWidth  int
	SortBy        string // "recency" (default) or "relevance"
}

type SearchResult struct {
	*session.Session
	Matches []session.Match `json:"matches"`
	Score   float64         `json:"score"`
}

type sessionInfo struct {
	sess  *session.Session
	score float64
}

func (idx *Index) Search(opts SearchOptions) ([]SearchResult, int, error) {
	// Sync failure is non-fatal: search stale data rather than failing entirely.
	if err := idx.Sync(opts.IncludeAgents); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: index sync failed: %v\n", err)
	}

	results, total, err := idx.ftsSearch(opts)
	if err != nil {
		return nil, 0, err
	}

	// FTS5 tokenizes on punctuation, so compound terms like "pre-commit" or
	// "fmt.Println" may return zero FTS hits. Fall back to substring scan
	// for these cases. Only triggers on single-word or compound queries to
	// avoid slow full-scan on multi-word natural language queries.
	if len(results) == 0 && (isSingleWord(opts.Query) || isCompoundQuery(opts.Query)) {
		results = idx.substringSearch(opts)
		total = len(results)
	}

	return results, total, nil
}

func (idx *Index) ProjectExists(name string) bool {
	var count int
	_ = idx.db.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE LOWER(project_dir) LIKE '%' || LOWER(?) || '%'",
		name,
	).Scan(&count)
	return count > 0
}

func isSingleWord(query string) bool {
	return len(strings.Fields(query)) == 1
}

func isCompoundQuery(query string) bool {
	return strings.ContainsAny(query, ".-_")
}

func compoundTerms(query string) []string {
	var terms []string
	for _, term := range strings.Fields(query) {
		if strings.ContainsAny(term, ".-_") {
			terms = append(terms, strings.ToLower(term))
		}
	}
	return terms
}

func (idx *Index) substringSearch(opts SearchOptions) []SearchResult {
	toSearch := session.DiscoverFiles(opts.ProjectFilter, opts.IncludeAgents)
	if len(toSearch) == 0 {
		return nil
	}

	snippetWidth := opts.SnippetWidth
	if snippetWidth <= 0 {
		snippetWidth = 80
	}
	maxMatches := opts.MaxMatches
	if maxMatches <= 0 {
		maxMatches = 3
	}

	limit := opts.MaxResults
	if limit <= 0 {
		limit = 25
	}

	streamResults := session.SearchFiles(toSearch, opts.Query, snippetWidth, maxMatches)

	results := make([]SearchResult, 0, len(streamResults))
	for _, sr := range streamResults {
		if sr == nil || len(sr.Matches) == 0 {
			continue
		}
		if len(results) >= limit {
			break
		}
		results = append(results, SearchResult{
			Session: sr.Session,
			Matches: sr.Matches,
			Score:   float64(len(sr.Matches)),
		})
	}
	return results
}

func (idx *Index) ftsSearch(opts SearchOptions) ([]SearchResult, int, error) {
	tokens := ftsTokens(opts.Query)
	if len(tokens) == 0 {
		return nil, 0, nil
	}

	limit := opts.MaxResults

	compounds := compoundTerms(opts.Query)
	projectFilter := strings.ToLower(opts.ProjectFilter)
	multiTerm := len(tokens) > 1

	// ftsLimit controls the SQL LIMIT clause. 0 means no limit.
	// For compound queries, we need all FTS candidates since post-filtering
	// may discard most of them.
	ftsLimit := limit
	if len(compounds) > 0 || limit <= 0 {
		ftsLimit = 0
	}

	orderBy := "s.modified_at DESC"
	if opts.SortBy == "relevance" {
		orderBy = "m.match_count DESC, s.modified_at DESC"
	}

	var totalMatched int
	var sessionIDs []string
	var sessions map[string]sessionInfo

	if multiTerm {
		intersectSQL, intersectArgs := buildIntersectSQL(tokens)
		orQuery := buildOrQuery(tokens)

		countQuery := `
			WITH session_pool AS (` + intersectSQL + `)
			SELECT COUNT(*) FROM session_pool sp
			JOIN sessions s ON sp.session_id = s.id
			WHERE (? = 1 OR s.is_agent = 0)
			  AND (? = '' OR LOWER(s.project_dir) LIKE '%' || ? || '%')
		`
		countArgs := make([]any, 0, len(intersectArgs)+3)
		countArgs = append(countArgs, intersectArgs...)
		countArgs = append(countArgs, boolToInt(opts.IncludeAgents), projectFilter, projectFilter)
		_ = idx.db.QueryRow(countQuery, countArgs...).Scan(&totalMatched)

		mainQuery := `
			WITH session_pool AS (` + intersectSQL + `),
			matches AS (
				SELECT sp.session_id, COUNT(*) as match_count
				FROM session_pool sp
				JOIN content_map m ON sp.session_id = m.session_id
				WHERE m.rowid IN (SELECT rowid FROM content_fts WHERE content_fts MATCH ?)
				GROUP BY sp.session_id
			)
			SELECT
				s.id, s.file_path, s.project_name, s.project_path,
				s.is_agent, s.modified_at,
				s.first_prompt, s.created_at, s.git_branch, s.message_count,
				s.custom_title, s.agent_type, s.agent_description,
				m.match_count
			FROM sessions s
			JOIN matches m ON s.id = m.session_id
			WHERE (? = 1 OR s.is_agent = 0)
			  AND (? = '' OR LOWER(s.project_dir) LIKE '%' || ? || '%')
			ORDER BY ` + orderBy + limitClause(ftsLimit) + `
		`
		mainArgs := make([]any, 0, len(intersectArgs)+5)
		mainArgs = append(mainArgs, intersectArgs...)
		mainArgs = append(mainArgs, orQuery, boolToInt(opts.IncludeAgents), projectFilter, projectFilter)
		mainArgs = appendLimit(mainArgs, ftsLimit)

		var err error
		sessionIDs, sessions, err = idx.scanSessionRows(mainQuery, mainArgs)
		if err != nil {
			return nil, 0, err
		}
	} else {
		ftsQuery := tokens[0] + "*"

		countQuery := `
			SELECT COUNT(DISTINCT m.session_id)
			FROM content_fts f
			JOIN content_map m ON f.rowid = m.rowid
			JOIN sessions s ON m.session_id = s.id
			WHERE content_fts MATCH ?
			  AND (? = 1 OR s.is_agent = 0)
			  AND (? = '' OR LOWER(s.project_dir) LIKE '%' || ? || '%')
		`
		_ = idx.db.QueryRow(countQuery, ftsQuery, boolToInt(opts.IncludeAgents), projectFilter, projectFilter).Scan(&totalMatched)

		mainQuery := `
			WITH matches AS (
				SELECT m.session_id, COUNT(*) as match_count
				FROM content_fts f
				JOIN content_map m ON f.rowid = m.rowid
				WHERE content_fts MATCH ?
				GROUP BY m.session_id
			)
			SELECT
				s.id, s.file_path, s.project_name, s.project_path,
				s.is_agent, s.modified_at,
				s.first_prompt, s.created_at, s.git_branch, s.message_count,
				s.custom_title, s.agent_type, s.agent_description,
				m.match_count
			FROM sessions s
			JOIN matches m ON s.id = m.session_id
			WHERE (? = 1 OR s.is_agent = 0)
			  AND (? = '' OR LOWER(s.project_dir) LIKE '%' || ? || '%')
			ORDER BY ` + orderBy + limitClause(ftsLimit) + `
		`

		mainArgs := []any{ftsQuery, boolToInt(opts.IncludeAgents), projectFilter, projectFilter}
		mainArgs = appendLimit(mainArgs, ftsLimit)

		var err error
		sessionIDs, sessions, err = idx.scanSessionRows(mainQuery, mainArgs)
		if err != nil {
			return nil, 0, err
		}
	}

	if len(sessionIDs) == 0 {
		return nil, 0, nil
	}

	maxMatches := opts.MaxMatches
	if maxMatches <= 0 {
		maxMatches = 3
	}
	snippetWidth := opts.SnippetWidth
	if snippetWidth <= 0 {
		snippetWidth = 80
	}

	snippetQuery := buildFTSQuery(opts.Query)
	if multiTerm {
		snippetQuery = buildOrQuery(tokens)
	}

	snippetMap := idx.batchGetSnippets(sessionIDs, snippetQuery, maxMatches, snippetWidth, opts.Query)

	results := make([]SearchResult, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		matches := snippetMap[id]
		if len(matches) == 0 && len(compounds) > 0 {
			continue
		}
		info := sessions[id]
		results = append(results, SearchResult{
			Session: info.sess,
			Matches: matches,
			Score:   info.score,
		})
	}

	if len(compounds) > 0 {
		totalMatched = len(results)
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, totalMatched, nil
}

func (idx *Index) scanSessionRows(query string, args []any) ([]string, map[string]sessionInfo, error) {
	rows, err := idx.db.Query(query, args...)
	if err != nil {
		return nil, nil, err
	}

	var sessionIDs []string
	sessions := make(map[string]sessionInfo)

	for rows.Next() {
		var id, filePath, projectName, projectPath, modifiedStr string
		var firstPrompt, createdAtStr, gitBranch, customTitle, agentType, agentDescription sql.NullString
		var isAgent, messageCount, matchCount int

		if err := rows.Scan(&id, &filePath, &projectName, &projectPath, &isAgent, &modifiedStr,
			&firstPrompt, &createdAtStr, &gitBranch, &messageCount, &customTitle,
			&agentType, &agentDescription, &matchCount); err != nil {
			_ = rows.Close()
			return nil, nil, err
		}

		modified, _ := time.Parse(time.RFC3339, modifiedStr)
		var created time.Time
		if createdAtStr.Valid {
			created, _ = time.Parse(time.RFC3339, createdAtStr.String)
		}

		sess := &session.Session{
			ID:               id,
			ShortID:          session.ShortID(id),
			IsAgent:          isAgent == 1,
			ProjectPath:      projectPath,
			ProjectName:      projectName,
			FilePath:         filePath,
			Modified:         modified,
			FirstPrompt:      firstPrompt.String,
			CustomTitle:      customTitle.String,
			Created:          created,
			GitBranch:        gitBranch.String,
			MessageCount:     messageCount,
			AgentType:        agentType.String,
			AgentDescription: agentDescription.String,
		}

		sessionIDs = append(sessionIDs, id)
		sessions[id] = sessionInfo{
			sess:  sess,
			score: float64(matchCount),
		}
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return sessionIDs, sessions, nil
}

func (idx *Index) batchGetSnippets(sessionIDs []string, ftsQuery string, maxPerSession, width int, originalQuery string) map[string][]session.Match {
	if len(sessionIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(sessionIDs))
	args := make([]any, len(sessionIDs)+1)
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	args[len(sessionIDs)] = ftsQuery

	query := `
		SELECT m.session_id, s.file_path, m.role, m.source, m.byte_offset, m.byte_length,
		       COALESCE(s.agent_description, '')
		FROM content_map m
		JOIN sessions s ON m.session_id = s.id
		WHERE m.session_id IN (` + strings.Join(placeholders, ",") + `)
		  AND m.rowid IN (SELECT rowid FROM content_fts WHERE content_fts MATCH ?)
		ORDER BY m.session_id, m.rowid
	`

	rows, err := idx.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var locations []snippetLocation
	for rows.Next() {
		var loc snippetLocation
		var agentDesc string
		if err := rows.Scan(&loc.sessionID, &loc.filePath, &loc.role, &loc.source, &loc.byteOffset, &loc.byteLength, &agentDesc); err != nil {
			continue
		}
		if loc.role == "description" {
			loc.inlineText = agentDesc
		}
		locations = append(locations, loc)
	}
	if err := rows.Err(); err != nil {
		return nil
	}

	queryTerms := strings.Fields(strings.ToLower(originalQuery))
	firstTerm := ""
	if len(queryTerms) > 0 {
		firstTerm = queryTerms[0]
	}
	compounds := compoundTerms(originalQuery)

	// Split synthetic (inline text) rows from file-backed rows so we only open
	// files for rows that actually need them.
	byFile := make(map[string][]snippetLocation)
	var inline []snippetLocation
	for _, loc := range locations {
		if loc.inlineText != "" {
			inline = append(inline, loc)
			continue
		}
		byFile[loc.filePath] = append(byFile[loc.filePath], loc)
	}

	result := make(map[string][]session.Match)

	addMatch := func(loc snippetLocation, text string) {
		if len(result[loc.sessionID]) >= maxPerSession {
			return
		}
		if len(compounds) > 0 {
			textLower := strings.ToLower(text)
			found := false
			for _, ct := range compounds {
				if strings.Contains(textLower, ct) {
					found = true
					break
				}
			}
			if !found {
				return
			}
		}
		snippet := output.ExtractSnippet(text, firstTerm, width)
		result[loc.sessionID] = append(result[loc.sessionID], session.Match{
			Role:    loc.role,
			Source:  loc.source,
			Snippet: snippet,
		})
	}

	for _, loc := range inline {
		addMatch(loc, loc.inlineText)
	}

	for filePath, fileLocs := range byFile {
		f, err := os.Open(filePath)
		if err != nil {
			continue
		}
		for _, loc := range fileLocs {
			if len(result[loc.sessionID]) >= maxPerSession {
				continue
			}
			text, err := readTextAt(f, loc.byteOffset, loc.byteLength)
			if err != nil {
				continue
			}
			addMatch(loc, text)
		}
		_ = f.Close()
	}

	return result
}

func readTextAt(f *os.File, offset int64, length int) (string, error) {
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", err
	}
	buf := make([]byte, length)
	n, err := io.ReadFull(f, buf)
	if err != nil && n == 0 {
		return "", err
	}
	var obj map[string]any
	if err := json.Unmarshal(buf[:n], &obj); err != nil {
		return "", err
	}
	return session.ExtractPromptText(obj), nil
}

// ftsTokens returns the individual sanitized FTS tokens for a query.
func ftsTokens(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	var allTokens []string
	for _, term := range strings.Fields(query) {
		sanitized := sanitizeFTSTerm(term)
		if sanitized != "" {
			allTokens = append(allTokens, strings.Fields(sanitized)...)
		}
	}
	return allTokens
}

// buildFTSQuery builds an implicit-AND FTS5 query with prefix matching on the last token.
func buildFTSQuery(query string) string {
	tokens := ftsTokens(query)
	if len(tokens) == 0 {
		return ""
	}

	tokens[len(tokens)-1] += "*"
	return strings.Join(tokens, " ")
}

// buildOrQuery builds an OR FTS5 query (any term matches) with prefix on the last token.
func buildOrQuery(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	out := make([]string, len(tokens))
	copy(out, tokens)
	out[len(out)-1] += "*"
	return strings.Join(out, " OR ")
}

// buildIntersectSQL builds a per-term INTERSECT query that finds sessions
// containing ALL terms, even if the terms appear in different messages.
func buildIntersectSQL(tokens []string) (string, []any) {
	parts := make([]string, 0, len(tokens))
	args := make([]any, 0, len(tokens))
	for i, t := range tokens {
		if i == len(tokens)-1 {
			t += "*"
		}
		parts = append(parts, `
			SELECT DISTINCT m.session_id
			FROM content_fts f
			JOIN content_map m ON f.rowid = m.rowid
			WHERE content_fts MATCH ?`)
		args = append(args, t)
	}
	return strings.Join(parts, "\nINTERSECT"), args
}

func limitClause(limit int) string {
	if limit <= 0 {
		return ""
	}
	return "\nLIMIT ?"
}

func appendLimit(args []any, limit int) []any {
	if limit <= 0 {
		return args
	}
	return append(args, limit)
}

func sanitizeFTSTerm(term string) string {
	var tokens []string
	var current strings.Builder
	for _, r := range term {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return strings.ToLower(strings.Join(tokens, " "))
}
