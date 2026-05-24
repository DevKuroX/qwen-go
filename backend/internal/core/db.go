package core

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenDB opens (or creates) the SQLite database at the given path with the
// pragmas qwen-go needs for the usage tracker and any future tables:
//
//	journal_mode=WAL       — concurrent readers + writer
//	busy_timeout=5000      — retry locked writes for 5s before erroring
//	synchronous=NORMAL     — WAL-friendly durability/speed balance
//	foreign_keys=ON        — defensive; cheap to enable globally
//
// Caller owns the returned handle and must Close() it on shutdown.
func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	return db, nil
}
