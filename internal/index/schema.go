package index

import (
	"database/sql"
	"fmt"
)

// schemaVersion is bumped whenever the table layout or the JSONL parsing
// logic changes in a way that invalidates existing rows. The index is a
// pure derived cache of ~/.claude/projects/**/*.jsonl, so any version
// mismatch is resolved by dropping all tables and letting the next Sync()
// repopulate from disk. Adding a new field becomes: edit schemaSQL, bump
// this constant.
const schemaVersion = 9

const schemaSQL = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	file_path TEXT NOT NULL UNIQUE,
	project_dir TEXT NOT NULL,
	project_name TEXT NOT NULL,
	project_path TEXT NOT NULL,
	is_agent INTEGER NOT NULL DEFAULT 0,
	modified_at TEXT NOT NULL,
	file_size INTEGER NOT NULL,
	first_prompt TEXT,
	created_at TEXT,
	git_branch TEXT,
	message_count INTEGER NOT NULL DEFAULT 0,
	custom_title TEXT,
	agent_type TEXT,
	agent_description TEXT
);

CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_dir);
CREATE INDEX IF NOT EXISTS idx_sessions_modified ON sessions(modified_at DESC);

CREATE TABLE IF NOT EXISTS content_map (
	rowid INTEGER PRIMARY KEY,
	session_id TEXT NOT NULL,
	role TEXT NOT NULL,
	source TEXT,
	byte_offset INTEGER NOT NULL,
	byte_length INTEGER NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS content_fts USING fts5(
	text,
	content='',
	contentless_delete=1,
	tokenize='porter unicode61'
);

CREATE INDEX IF NOT EXISTS idx_content_map_session ON content_map(session_id);

CREATE TABLE IF NOT EXISTS index_meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

// derivedTables lists every table we own. All of them are reconstructable
// from JSONL on disk, so ensureSchema drops them wholesale on any version
// mismatch. content_raw is a legacy pre-v5 table kept in the drop list so
// old DBs upgrading to the rebuild model don't leave orphaned tables behind.
var derivedTables = []string{
	"sessions",
	"content_map",
	"content_fts",
	"content_raw",
	"index_meta",
}

func (idx *Index) ensureSchema() error {
	var version int
	if err := idx.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == schemaVersion {
		return nil
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, name := range derivedTables {
		if _, err := tx.Exec("DROP TABLE IF EXISTS " + name); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if _, err := idx.db.Exec(schemaSQL); err != nil {
		return err
	}
	if _, err := idx.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return err
	}
	return nil
}

func (idx *Index) deleteSessionData(tx *sql.Tx, sessionID string) error {
	// For contentless_delete=1 tables, use standard DELETE syntax
	rows, err := tx.Query("SELECT rowid FROM content_map WHERE session_id = ?", sessionID)
	if err != nil {
		return err
	}
	var rowIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		rowIDs = append(rowIDs, id)
	}
	_ = rows.Close()

	for _, rowID := range rowIDs {
		if _, err := tx.Exec("DELETE FROM content_fts WHERE rowid = ?", rowID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec("DELETE FROM content_map WHERE session_id = ?", sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM sessions WHERE id = ?", sessionID); err != nil {
		return err
	}
	return nil
}
