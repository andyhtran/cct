package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/andyhtran/cct/internal/session"
)

type fileInfo struct {
	modified time.Time
	size     int64
}

type indexedFile struct {
	sessionID  string
	modifiedAt time.Time
	fileSize   int64
}

type indexedMessage struct {
	role       string
	source     string
	text       string
	byteOffset int64
	byteLength int
}

type indexedSession struct {
	session  *session.Session
	messages []indexedMessage
	fileSize int64
}

type SyncResult struct {
	Added     int
	Updated   int
	Deleted   int
	Unchanged int
}

func (r *SyncResult) UpToDate() bool {
	return r.Added == 0 && r.Updated == 0 && r.Deleted == 0
}

const (
	// Skip filesystem scan if synced recently. The --sync flag bypasses this.
	syncCacheDuration = 5 * time.Minute
	maxWorkers        = 4  // Cap concurrent file parsers to limit memory
	batchSize         = 50 // Process sessions in batches to limit memory
)

func (idx *Index) Sync(includeAgents bool) error {
	_, err := idx.syncInternal(includeAgents, false, nil)
	return err
}

func (idx *Index) ForceSync(includeAgents bool) error {
	_, err := idx.syncInternal(includeAgents, true, nil)
	return err
}

func (idx *Index) SyncWithProgress(includeAgents bool, force bool, w io.Writer) (*SyncResult, error) {
	return idx.syncInternal(includeAgents, force, w)
}

func (idx *Index) syncInternal(includeAgents bool, force bool, progress io.Writer) (*SyncResult, error) {
	idx.syncMu.Lock()
	defer idx.syncMu.Unlock()

	if !force && idx.recentlySynced() {
		return &SyncResult{}, nil
	}

	lock, err := acquireLock(idx.path + ".lock")
	if err != nil {
		return nil, err
	}
	defer lock.release()

	return idx.syncLocked(includeAgents, progress)
}

func (idx *Index) syncLocked(includeAgents bool, progress io.Writer) (*SyncResult, error) {
	current := discoverCurrentFiles(includeAgents)
	indexed, err := idx.getIndexedFiles()
	if err != nil {
		return nil, fmt.Errorf("get indexed files: %w", err)
	}

	toAdd, toUpdate, toDelete := computeChanges(current, indexed)
	unchanged := len(indexed) - len(toUpdate) - len(toDelete)
	if unchanged < 0 {
		unchanged = 0
	}

	result := &SyncResult{
		Added:     len(toAdd),
		Updated:   len(toUpdate),
		Deleted:   len(toDelete),
		Unchanged: unchanged,
	}

	if result.UpToDate() {
		idx.updateSyncTime()
		return result, nil
	}

	total := len(toAdd) + len(toUpdate)
	if progress != nil && total > 0 {
		_, _ = fmt.Fprintf(progress, "Indexing %d session(s)...\n", total)
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := idx.deleteRemovedSessions(tx, toDelete, indexed); err != nil {
		return nil, err
	}

	allPaths := make([]string, 0, len(toAdd)+len(toUpdate))
	allPaths = append(allPaths, toAdd...)
	allPaths = append(allPaths, toUpdate...)
	if err := idx.indexBatches(tx, allPaths, indexed, total, progress); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	idx.updateSyncTime()
	return result, nil
}

func discoverCurrentFiles(includeAgents bool) map[string]fileInfo {
	files := session.DiscoverFiles("", includeAgents)
	current := make(map[string]fileInfo, len(files))
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		current[path] = fileInfo{
			modified: info.ModTime().Truncate(time.Second),
			size:     info.Size(),
		}
	}
	return current
}

func computeChanges(current map[string]fileInfo, indexed map[string]indexedFile) (toAdd, toUpdate, toDelete []string) {
	for path, info := range current {
		if existing, ok := indexed[path]; !ok {
			toAdd = append(toAdd, path)
		} else if info.modified.After(existing.modifiedAt) || info.size != existing.fileSize {
			toUpdate = append(toUpdate, path)
		}
	}
	for path := range indexed {
		if _, ok := current[path]; !ok {
			toDelete = append(toDelete, path)
		}
	}
	return
}

func (idx *Index) deleteRemovedSessions(tx *sql.Tx, toDelete []string, indexed map[string]indexedFile) error {
	for _, path := range toDelete {
		sessionID := indexed[path].sessionID
		if err := idx.deleteSessionData(tx, sessionID); err != nil {
			return fmt.Errorf("delete session %s: %w", sessionID, err)
		}
	}
	return nil
}

func (idx *Index) indexBatches(tx *sql.Tx, allPaths []string, indexed map[string]indexedFile, total int, progress io.Writer) error {
	var processed int64
	for i := 0; i < len(allPaths); i += batchSize {
		end := min(i+batchSize, len(allPaths))
		batch := allPaths[i:end]

		results := parallelIndex(batch)

		for _, r := range results {
			if r.err != nil {
				continue
			}
			if _, ok := indexed[r.session.session.FilePath]; ok {
				if err := idx.deleteSessionData(tx, r.session.session.ID); err != nil {
					return fmt.Errorf("delete for update %s: %w", r.session.session.ID, err)
				}
			}
			if err := idx.insertSession(tx, r.session); err != nil {
				return fmt.Errorf("insert session %s: %w", r.session.session.ID, err)
			}
			processed++
			if progress != nil && (processed%25 == 0 || int(processed) == total) {
				_, _ = fmt.Fprintf(progress, "\r  %d/%d sessions indexed", processed, total)
			}
		}
	}
	if progress != nil && total > 0 {
		_, _ = fmt.Fprintln(progress)
	}
	return nil
}

func (idx *Index) RebuildWithProgress(includeAgents bool, progress io.Writer) (*SyncResult, error) {
	idx.syncMu.Lock()
	defer idx.syncMu.Unlock()

	lock, err := acquireLock(idx.path + ".lock")
	if err != nil {
		return nil, err
	}
	defer lock.release()

	if progress != nil {
		_, _ = fmt.Fprintln(progress, "Dropping old index...")
	}
	if _, err := idx.db.Exec("DROP TABLE IF EXISTS content_fts"); err != nil {
		return nil, err
	}
	if _, err := idx.db.Exec("DROP TABLE IF EXISTS content_map"); err != nil {
		return nil, err
	}
	if _, err := idx.db.Exec(`
		CREATE TABLE content_map (
			rowid INTEGER PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			source TEXT,
			byte_offset INTEGER NOT NULL,
			byte_length INTEGER NOT NULL
		)
	`); err != nil {
		return nil, err
	}
	if _, err := idx.db.Exec("CREATE INDEX idx_content_map_session ON content_map(session_id)"); err != nil {
		return nil, err
	}
	if _, err := idx.db.Exec("DELETE FROM sessions"); err != nil {
		return nil, err
	}
	if _, err := idx.db.Exec(`
		CREATE VIRTUAL TABLE content_fts USING fts5(
			text,
			content='',
			contentless_delete=1,
			tokenize='porter unicode61'
		)
	`); err != nil {
		return nil, err
	}
	if _, err := idx.db.Exec("DELETE FROM index_meta WHERE key = 'last_sync_time'"); err != nil {
		return nil, err
	}
	if _, err := idx.db.Exec("VACUUM"); err != nil {
		return nil, err
	}
	idx.lastSyncTime = time.Time{}
	return idx.syncLocked(includeAgents, progress)
}

func (idx *Index) getIndexedFiles() (map[string]indexedFile, error) {
	rows, err := idx.db.Query("SELECT id, file_path, modified_at, file_size FROM sessions")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]indexedFile)
	for rows.Next() {
		var id, path, modifiedStr string
		var size int64
		if err := rows.Scan(&id, &path, &modifiedStr, &size); err != nil {
			return nil, err
		}
		modified, _ := time.Parse(time.RFC3339, modifiedStr)
		result[path] = indexedFile{
			sessionID:  id,
			modifiedAt: modified,
			fileSize:   size,
		}
	}
	return result, rows.Err()
}

func (idx *Index) insertSession(tx *sql.Tx, s *indexedSession) error {
	sess := s.session
	projectDir := filepath.Base(filepath.Dir(sess.FilePath))

	var createdAt string
	if !sess.Created.IsZero() {
		createdAt = sess.Created.Format(time.RFC3339)
	}

	_, err := tx.Exec(`
		INSERT OR REPLACE INTO sessions (id, file_path, project_dir, project_name, project_path, is_agent, modified_at, file_size,
			first_prompt, created_at, git_branch, message_count, custom_title)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sess.ID, sess.FilePath, projectDir, sess.ProjectName, sess.ProjectPath, boolToInt(sess.IsAgent),
		sess.Modified.Format(time.RFC3339), s.fileSize,
		sess.FirstPrompt, createdAt, sess.GitBranch, sess.MessageCount, sess.CustomTitle)
	if err != nil {
		return err
	}

	for _, m := range s.messages {
		res, err := tx.Exec(`
			INSERT INTO content_map (session_id, role, source, byte_offset, byte_length)
			VALUES (?, ?, ?, ?, ?)
		`, sess.ID, m.role, m.source, m.byteOffset, m.byteLength)
		if err != nil {
			return err
		}

		rowID, err := res.LastInsertId()
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
			INSERT INTO content_fts (rowid, text)
			VALUES (?, ?)
		`, rowID, m.text)
		if err != nil {
			return err
		}
	}

	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type indexResult struct {
	session *indexedSession
	err     error
}

func parallelIndex(files []string) []indexResult {
	return parallelIndexWithProgress(files, nil)
}

func parallelIndexWithProgress(files []string, progress io.Writer) []indexResult {
	if len(files) == 0 {
		return nil
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > maxWorkers {
		numWorkers = maxWorkers
	}
	if numWorkers > len(files) {
		numWorkers = len(files)
	}

	jobs := make(chan string, len(files))
	results := make(chan indexResult, len(files))
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				s, err := indexSession(path)
				results <- indexResult{session: s, err: err}
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

	total := len(files)
	var count int
	out := make([]indexResult, 0, len(files))
	for r := range results {
		out = append(out, r)
		if progress != nil {
			count++
			if count%25 == 0 || count == total {
				_, _ = fmt.Fprintf(progress, "\r  %d/%d sessions indexed", count, total)
			}
		}
	}
	if progress != nil {
		_, _ = fmt.Fprintln(progress)
	}
	return out
}

func indexSession(path string) (*indexedSession, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	s := &session.Session{
		ID:       session.ExtractIDFromFilename(path),
		FilePath: path,
		Modified: info.ModTime(),
	}
	s.ShortID = session.ShortID(s.ID)
	s.IsAgent = session.IsAgentSession(s.ID)

	scanner := session.NewOffsetScanner(f)
	var messages []indexedMessage
	var messageCount int

	for scanner.Scan() {
		line := scanner.Bytes()
		lineType := session.FastExtractType(line)

		// custom-title records carry the /rename title and are rewritten
		// each turn — latest wins. They don't contribute to FTS content.
		if lineType == "custom-title" {
			var obj map[string]any
			if json.Unmarshal(line, &obj) != nil {
				continue
			}
			if ct, _ := obj["customTitle"].(string); ct != "" {
				s.CustomTitle = ct
			}
			continue
		}

		if lineType != "user" && lineType != "assistant" {
			continue
		}

		messageCount++
		byteOffset := scanner.Offset()
		byteLength := scanner.Length()

		var obj map[string]any
		if json.Unmarshal(line, &obj) != nil {
			continue
		}

		if lineType == "user" {
			session.ExtractUserMetadata(s, obj)
		}

		blocks := session.ExtractPromptBlocks(obj)
		for _, block := range blocks {
			if block.Text == "" {
				continue
			}
			messages = append(messages, indexedMessage{
				role:       lineType,
				source:     block.Source,
				text:       block.Text,
				byteOffset: byteOffset,
				byteLength: byteLength,
			})
		}
	}

	s.MessageCount = messageCount

	return &indexedSession{
		session:  s,
		messages: messages,
		fileSize: info.Size(),
	}, nil
}

func (idx *Index) recentlySynced() bool {
	if !idx.lastSyncTime.IsZero() && time.Since(idx.lastSyncTime) < syncCacheDuration {
		return true
	}

	var lastSync string
	err := idx.db.QueryRow("SELECT value FROM index_meta WHERE key = 'last_sync_time'").Scan(&lastSync)
	if err != nil {
		return false
	}

	t, err := time.Parse(time.RFC3339Nano, lastSync)
	if err != nil {
		return false
	}

	idx.lastSyncTime = t
	return time.Since(t) < syncCacheDuration
}

func (idx *Index) updateSyncTime() {
	now := time.Now()
	idx.lastSyncTime = now
	_, _ = idx.db.Exec(
		"INSERT OR REPLACE INTO index_meta (key, value) VALUES ('last_sync_time', ?)",
		now.Format(time.RFC3339Nano),
	)
}
