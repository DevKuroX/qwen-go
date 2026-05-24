package core

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"sync"
)

// UsageRecord is the per-request row written by chat/image/video handlers.
// JSON tags stay identical to the legacy jsonl shape so the migrate command
// can re-use them when importing data/usage.jsonl rows.
type UsageRecord struct {
	Timestamp        int64  `json:"ts"`
	Model            string `json:"model"`
	Feature          string `json:"feature"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	Success          bool   `json:"success"`
}

type FeatureStats struct {
	Requests int `json:"requests"`
	Tokens   int `json:"tokens"`
}

type ModelStats struct {
	Requests         int `json:"requests"`
	Tokens           int `json:"tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type TimelineBucket struct {
	Timestamp        int64 `json:"timestamp"`
	Requests         int   `json:"requests"`
	Tokens           int   `json:"tokens"`
	PromptTokens     int   `json:"prompt_tokens"`
	CompletionTokens int   `json:"completion_tokens"`
}

type UsageStats struct {
	TotalRequests         int                     `json:"total_requests"`
	TotalTokens           int                     `json:"total_tokens"`
	TotalPromptTokens     int                     `json:"total_prompt_tokens"`
	TotalCompletionTokens int                     `json:"total_completion_tokens"`
	RPM                   float64                 `json:"rpm"`
	TPM                   float64                 `json:"tpm"`
	SuccessCount          int                     `json:"success_count"`
	ErrorCount            int                     `json:"error_count"`
	ByFeature             map[string]FeatureStats `json:"by_feature"`
	Timeline              []TimelineBucket        `json:"timeline"`
	Models                []string                `json:"models"`
	ModelStats            map[string]ModelStats   `json:"model_stats"`
}

// UsageTracker persists per-request usage rows to SQLite. The schema is
// created on first construction; subsequent boots skip the DDL.
//
// `duration_ms` is reserved in the schema for forward compatibility (no
// caller populates it today), so adding it later is a single ALTER away
// without an export/import cycle.
type UsageTracker struct {
	mu sync.Mutex
	db *sql.DB
}

const usageSchema = `
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

// NewUsageTracker takes an already-opened SQLite handle (see OpenDB). It
// runs the schema DDL idempotently. Returns a non-nil tracker even if DDL
// fails — Record/Query become no-ops in that degraded state so the rest of
// the gateway keeps serving requests.
func NewUsageTracker(db *sql.DB) *UsageTracker {
	ut := &UsageTracker{db: db}
	if db != nil {
		if _, err := db.Exec(usageSchema); err != nil {
			// Degraded mode: leave db nil so Record/Query short-circuit.
			ut.db = nil
		}
	}
	return ut
}

// Record inserts one row. We swallow the error rather than propagating
// because callers are best-effort fire-and-forget (RecordUsage in api/usage.go
// has no return path). Per-request failures should not crash chat.
func (ut *UsageTracker) Record(r UsageRecord) {
	ut.mu.Lock()
	defer ut.mu.Unlock()
	if ut.db == nil {
		return
	}
	successInt := 0
	if r.Success {
		successInt = 1
	}
	_, _ = ut.db.Exec(
		`INSERT INTO usage (ts, model, feature, prompt_tokens, completion_tokens, success)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.Timestamp, r.Model, r.Feature, r.PromptTokens, r.CompletionTokens, successInt,
	)
}

// Query returns a UsageStats with the same shape as the legacy in-memory
// tracker — handler + frontend ride unchanged. Approach: pull rows in the
// requested window via the (ts) index, aggregate + bucket in Go using the
// same logic as before. For 100k rows the indexed range scan is fast and
// the in-Go pass is identical to the legacy hot path.
func (ut *UsageTracker) Query(start, end int64, modelFilter string) *UsageStats {
	stats := &UsageStats{
		ByFeature:  make(map[string]FeatureStats),
		ModelStats: make(map[string]ModelStats),
		Models:     make([]string, 0),
		Timeline:   make([]TimelineBucket, 0),
	}
	if ut.db == nil {
		return stats
	}
	if end < start {
		return stats
	}

	rows, err := ut.selectRange(start, end, modelFilter)
	if err != nil {
		return stats
	}

	modelSet := make(map[string]bool)
	for _, r := range rows {
		stats.TotalRequests++
		stats.TotalPromptTokens += r.PromptTokens
		stats.TotalCompletionTokens += r.CompletionTokens
		stats.TotalTokens += r.PromptTokens + r.CompletionTokens

		if r.Success {
			stats.SuccessCount++
		} else {
			stats.ErrorCount++
		}

		f := r.Feature
		if f == "" {
			f = "chat"
		}
		fStat := stats.ByFeature[f]
		fStat.Requests++
		fStat.Tokens += r.PromptTokens + r.CompletionTokens
		stats.ByFeature[f] = fStat

		mStat := stats.ModelStats[r.Model]
		mStat.Requests++
		mStat.Tokens += r.PromptTokens + r.CompletionTokens
		mStat.PromptTokens += r.PromptTokens
		mStat.CompletionTokens += r.CompletionTokens
		stats.ModelStats[r.Model] = mStat

		modelSet[r.Model] = true
	}

	for m := range modelSet {
		stats.Models = append(stats.Models, m)
	}
	sort.Strings(stats.Models)

	durationHours := (end - start) / 3600
	if durationHours < 1 {
		durationHours = 1
	}
	stats.RPM = float64(stats.TotalRequests) / float64(durationHours)
	stats.TPM = float64(stats.TotalTokens) / float64(durationHours)

	stats.Timeline = ut.buildTimeline(rows, start, end)
	return stats
}

// selectRange pulls the rows in [start,end] honouring the optional model
// filter. Parameterised query — never string-concat user input.
func (ut *UsageTracker) selectRange(start, end int64, modelFilter string) ([]UsageRecord, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if modelFilter != "" {
		rows, err = ut.db.Query(
			`SELECT ts, model, feature, prompt_tokens, completion_tokens, success
			 FROM usage WHERE ts >= ? AND ts <= ? AND model = ? ORDER BY ts ASC`,
			start, end, modelFilter,
		)
	} else {
		rows, err = ut.db.Query(
			`SELECT ts, model, feature, prompt_tokens, completion_tokens, success
			 FROM usage WHERE ts >= ? AND ts <= ? ORDER BY ts ASC`,
			start, end,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("usage select: %w", err)
	}
	defer rows.Close()

	out := make([]UsageRecord, 0, 256)
	for rows.Next() {
		var (
			rec       UsageRecord
			successIn int
		)
		if err := rows.Scan(&rec.Timestamp, &rec.Model, &rec.Feature,
			&rec.PromptTokens, &rec.CompletionTokens, &successIn); err != nil {
			return nil, err
		}
		rec.Success = successIn != 0
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (ut *UsageTracker) buildTimeline(records []UsageRecord, start, end int64) []TimelineBucket {
	bucketSize := bucketSizeFor(start, end)
	numBuckets := int(math.Ceil(float64(end-start) / float64(bucketSize)))
	if numBuckets > 500 {
		numBuckets = 500
	}
	if numBuckets < 1 {
		numBuckets = 1
	}
	actualBucketSize := float64(end-start) / float64(numBuckets)
	if actualBucketSize <= 0 {
		actualBucketSize = 1
	}

	buckets := make([]TimelineBucket, numBuckets)
	for i := 0; i < numBuckets; i++ {
		buckets[i].Timestamp = start + int64(float64(i)*actualBucketSize)
	}
	for _, r := range records {
		idx := int(float64(r.Timestamp-start) / actualBucketSize)
		if idx < 0 {
			idx = 0
		}
		if idx >= numBuckets {
			idx = numBuckets - 1
		}
		buckets[idx].Requests++
		buckets[idx].Tokens += r.PromptTokens + r.CompletionTokens
		buckets[idx].PromptTokens += r.PromptTokens
		buckets[idx].CompletionTokens += r.CompletionTokens
	}
	return buckets
}

func bucketSizeFor(start, end int64) int64 {
	duration := end - start
	switch {
	case duration <= 7200:
		return 60
	case duration <= 86400:
		return 3600
	case duration <= 604800:
		return 21600
	default:
		return 86400
	}
}

// Close releases the underlying handle. Safe to call multiple times.
// Callers that share the *sql.DB with other tables should NOT call this —
// close the DB at the server lifecycle boundary instead.
func (ut *UsageTracker) Close() {
	ut.mu.Lock()
	defer ut.mu.Unlock()
	if ut.db != nil {
		_ = ut.db.Close()
		ut.db = nil
	}
}
