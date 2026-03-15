package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/andyhtran/cct/internal/paths"
	_ "modernc.org/sqlite"
)

type Index struct {
	db           *sql.DB
	path         string
	lastSyncTime time.Time
	syncMu       sync.Mutex
}

func Open() (*Index, error) {
	dbPath := paths.IndexPath()

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	idx, err := openDB(dbPath)
	if err != nil {
		if !strings.Contains(err.Error(), "ensure schema") {
			return nil, err
		}
		fmt.Fprintln(os.Stderr, "Recreating corrupted search index...")
		for _, ext := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(dbPath + ext)
		}
		idx, err = openDB(dbPath)
		if err != nil {
			return nil, err
		}
	}
	return idx, nil
}

func openDB(dbPath string) (*Index, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	idx := &Index{db: db, path: dbPath}

	if err := idx.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}

	return idx, nil
}

func (idx *Index) Close() error {
	return idx.db.Close()
}

func (idx *Index) Path() string {
	return idx.path
}

type IndexStatus struct {
	Path           string    `json:"path"`
	TotalSessions  int       `json:"total_sessions"`
	TotalMessages  int       `json:"total_messages"`
	LastSyncTime   time.Time `json:"last_sync_time"`
	IndexSizeBytes int64     `json:"index_size_bytes"`
}

func (idx *Index) Status() (*IndexStatus, error) {
	var sessions, messages int

	if err := idx.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessions); err != nil {
		return nil, err
	}

	if err := idx.db.QueryRow("SELECT COUNT(*) FROM content_map").Scan(&messages); err != nil {
		return nil, err
	}

	var size int64
	if info, err := os.Stat(idx.path); err == nil {
		size = info.Size()
	}

	var lastSync time.Time
	var lastSyncStr string
	if err := idx.db.QueryRow("SELECT value FROM index_meta WHERE key = 'last_sync_time'").Scan(&lastSyncStr); err == nil {
		lastSync, _ = time.Parse(time.RFC3339Nano, lastSyncStr)
	}

	return &IndexStatus{
		Path:           idx.path,
		TotalSessions:  sessions,
		TotalMessages:  messages,
		LastSyncTime:   lastSync,
		IndexSizeBytes: size,
	}, nil
}
