package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/core"
)

type Account struct {
	Email               string     `json:"email"`
	Password            string     `json:"password"`
	Token               string     `json:"token"`
	Cookies             string     `json:"cookies,omitempty"`
	Username            string     `json:"username"`
	Provider            string     `json:"provider"`
	Status              string     `json:"status"`
	StatusCode          string     `json:"status_code,omitempty"`
	Inflight            int        `json:"inflight"`
	RateLimitedUntil    float64    `json:"rate_limited_until"`
	RateLimitCount      int        `json:"rate_limit_count"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	CircuitOpenCount    int        `json:"circuit_open_count"`
	LastRequestTime     time.Time  `json:"last_request_time"`
	LastError           string     `json:"last_error"`
	ActivationPending   bool       `json:"activation_pending"`
	Score               float64    `json:"score"`
	Valid               bool       `json:"valid,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	DeletedAt           *time.Time `json:"deleted_at,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  migrate <accounts.json>      — add provider/created_at defaults")
		fmt.Println("  migrate <usage.jsonl>        — import into <dirname>/qwen-go.db")
		os.Exit(1)
	}

	filePath := os.Args[1]
	switch {
	case strings.HasSuffix(filePath, ".jsonl"):
		if err := migrateUsageJSONL(filePath); err != nil {
			fmt.Printf("usage migration failed: %v\n", err)
			os.Exit(1)
		}
	default:
		if err := migrateAccountsJSON(filePath); err != nil {
			fmt.Printf("accounts migration failed: %v\n", err)
			os.Exit(1)
		}
	}
}

func migrateAccountsJSON(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var accounts []Account
	if err := json.Unmarshal(data, &accounts); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	now := time.Now()
	updated := 0
	for i := range accounts {
		if accounts[i].Provider == "" {
			accounts[i].Provider = "qwen"
			updated++
		}
		if accounts[i].CreatedAt.IsZero() {
			accounts[i].CreatedAt = now
			updated++
		}
		if accounts[i].Status == "" && accounts[i].Valid {
			accounts[i].Status = "VALID"
		}
	}

	output, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	backupPath := filePath + ".backup"
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	if err := os.WriteFile(filePath, output, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Printf("Migration completed: %d fields updated\n", updated)
	fmt.Printf("Backup saved to: %s\n", backupPath)
	return nil
}

// migrateUsageJSONL imports each JSON line from <usage.jsonl> into the
// usage table of <dirname>/qwen-go.db, then renames the source file to
// <filename>.archive so subsequent runs no-op. All inserts run inside a
// single transaction for speed (100k rows ≈ a few seconds, not minutes).
func migrateUsageJSONL(filePath string) error {
	archivePath := filePath + ".archive"
	if _, err := os.Stat(archivePath); err == nil {
		fmt.Printf("Already archived at %s — skipping.\n", archivePath)
		return nil
	}

	dir := filepath.Dir(filePath)
	dbPath := filepath.Join(dir, "qwen-go.db")

	db, err := core.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("open db %s: %w", dbPath, err)
	}
	defer db.Close()

	if err := ensureUsageSchema(db); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open jsonl: %w", err)
	}
	defer f.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO usage (ts, model, feature, prompt_tokens, completion_tokens, success)
		 VALUES (?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	inserted := 0
	skipped := 0
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec core.UsageRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			skipped++
			continue
		}
		successInt := 0
		if rec.Success {
			successInt = 1
		}
		if _, err := stmt.Exec(rec.Timestamp, rec.Model, rec.Feature,
			rec.PromptTokens, rec.CompletionTokens, successInt); err != nil {
			return fmt.Errorf("insert line %d: %w", lineNum, err)
		}
		inserted++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan jsonl: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if err := os.Rename(filePath, archivePath); err != nil {
		return fmt.Errorf("archive source: %w", err)
	}

	fmt.Printf("Usage migration completed: %d rows inserted, %d skipped (parse errors)\n", inserted, skipped)
	fmt.Printf("Source archived to: %s\n", archivePath)
	fmt.Printf("DB updated:         %s\n", dbPath)
	return nil
}

func ensureUsageSchema(db *sql.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS usage (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    ts                INTEGER NOT NULL,
    model             TEXT    NOT NULL,
    feature           TEXT    NOT NULL,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    success           INTEGER NOT NULL DEFAULT 1,
    duration_ms       INTEGER
);
CREATE INDEX IF NOT EXISTS idx_usage_ts          ON usage(ts);
CREATE INDEX IF NOT EXISTS idx_usage_feature_ts  ON usage(feature, ts);
CREATE INDEX IF NOT EXISTS idx_usage_model_ts    ON usage(model, ts);
`
	_, err := db.Exec(ddl)
	return err
}
