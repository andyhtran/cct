package index

import "database/sql"

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
	message_count INTEGER NOT NULL DEFAULT 0
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

func (idx *Index) ensureSchema() error {
	var version int
	err := idx.db.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		return err
	}

	if version == 0 {
		if _, err := idx.db.Exec(schemaSQL); err != nil {
			return err
		}
		if _, err := idx.db.Exec("PRAGMA user_version = 6"); err != nil {
			return err
		}
		return nil
	}

	if version < 5 {
		tx, err := idx.db.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		for _, stmt := range []string{
			"DROP TABLE IF EXISTS content_fts",
			"DROP TABLE IF EXISTS content_raw",
			"DROP TABLE IF EXISTS content_map",
			"DELETE FROM sessions",
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`
			CREATE TABLE content_map (
				rowid INTEGER PRIMARY KEY,
				session_id TEXT NOT NULL,
				role TEXT NOT NULL,
				source TEXT,
				byte_offset INTEGER NOT NULL,
				byte_length INTEGER NOT NULL
			)
		`); err != nil {
			return err
		}
		if _, err := tx.Exec("CREATE INDEX idx_content_map_session ON content_map(session_id)"); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS index_meta (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL
			)
		`); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM index_meta WHERE key = 'last_sync_time'"); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}

		if _, err := idx.db.Exec(`
			CREATE VIRTUAL TABLE content_fts USING fts5(
				text,
				content='',
				contentless_delete=1,
				tokenize='porter unicode61'
			)
		`); err != nil {
			return err
		}
		if _, err := idx.db.Exec("VACUUM"); err != nil {
			return err
		}
		if _, err := idx.db.Exec("PRAGMA user_version = 5"); err != nil {
			return err
		}
		version = 5
	}

	if version < 6 {
		tx, err := idx.db.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		for _, col := range []string{
			"ALTER TABLE sessions ADD COLUMN first_prompt TEXT",
			"ALTER TABLE sessions ADD COLUMN created_at TEXT",
			"ALTER TABLE sessions ADD COLUMN git_branch TEXT",
			"ALTER TABLE sessions ADD COLUMN message_count INTEGER NOT NULL DEFAULT 0",
		} {
			_, _ = tx.Exec(col)
		}
		if _, err := tx.Exec("DELETE FROM index_meta WHERE key = 'last_sync_time'"); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		if _, err := idx.db.Exec("PRAGMA user_version = 6"); err != nil {
			return err
		}
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
