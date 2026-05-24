package core

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// RequestLog is one detailed row written per inbound /v1/chat|images|videos
// request. Captures everything needed for the dashboard "Request Log" drill-
// down: body, token-saver stats, error message, latency.
type RequestLog struct {
	ID                string `json:"id"`
	Timestamp         int64  `json:"ts"`
	Model             string `json:"model"`
	Provider          string `json:"provider"`
	Feature           string `json:"feature"`
	Account           string `json:"account,omitempty"`
	LatencyMs         int    `json:"latency_ms"`
	Status            string `json:"status"`
	HTTPStatus        int    `json:"http_status"`
	ErrorMessage      string `json:"error_message,omitempty"`
	RequestBody       string `json:"request_body,omitempty"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionTokens  int    `json:"completion_tokens"`
	PromptTokensPre   int    `json:"prompt_tokens_pre"`
	CompactionMode    string `json:"compaction_mode,omitempty"`
	SaverReductionPct int    `json:"saver_reduction_pct"`
	CavemanLevel      string `json:"caveman_level,omitempty"`
	RtkBytesBefore    int    `json:"rtk_bytes_before,omitempty"`
	RtkBytesAfter     int    `json:"rtk_bytes_after,omitempty"`
	RtkFilters        string `json:"rtk_filters,omitempty"`
}

// AutoScriptRun is one batch-registration / auto-replenish run. Persisted
// from BatchManager when a job transitions to a terminal state so the
// "Auto Script Log" page survives restarts.
type AutoScriptRun struct {
	ID          string `json:"id"`
	TimestampStart int64  `json:"ts_start"`
	TimestampEnd   int64  `json:"ts_end,omitempty"`
	Trigger     string `json:"trigger"` // manual | auto
	Provider    string `json:"provider"`
	Attempted   int    `json:"attempted"`
	Succeeded   int    `json:"succeeded"`
	Failed      int    `json:"failed"`
	Status      string `json:"status"`
	LogsJSON    string `json:"logs_json,omitempty"`
}

const (
	requestLogBodyCap     = 65536
	requestLogRingSize    = 1000
	autoScriptLogsCap     = 32768
	autoScriptRingSize    = 200
	pruneInterval         = 60 * time.Second
)

const requestLogSchema = `
CREATE TABLE IF NOT EXISTS request_logs (
    id                   TEXT PRIMARY KEY,
    ts                   INTEGER NOT NULL,
    model                TEXT,
    provider             TEXT,
    feature              TEXT,
    account              TEXT,
    latency_ms           INTEGER,
    status               TEXT NOT NULL,
    http_status          INTEGER,
    error_message        TEXT,
    request_body         TEXT,
    prompt_tokens        INTEGER DEFAULT 0,
    completion_tokens    INTEGER DEFAULT 0,
    prompt_tokens_pre    INTEGER DEFAULT 0,
    compaction_mode      TEXT,
    saver_reduction_pct  INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_request_logs_ts     ON request_logs(ts DESC);
CREATE INDEX IF NOT EXISTS idx_request_logs_status ON request_logs(status, ts DESC);

CREATE TABLE IF NOT EXISTS auto_script_runs (
    id         TEXT PRIMARY KEY,
    ts_start   INTEGER NOT NULL,
    ts_end     INTEGER,
    trigger    TEXT,
    provider   TEXT,
    attempted  INTEGER DEFAULT 0,
    succeeded  INTEGER DEFAULT 0,
    failed     INTEGER DEFAULT 0,
    status     TEXT,
    logs_json  TEXT
);
CREATE INDEX IF NOT EXISTS idx_auto_runs_ts ON auto_script_runs(ts_start DESC);
`

// RequestLogTracker owns SQLite reads/writes for both request_logs and
// auto_script_runs. Shares the same *sql.DB handle with UsageTracker — do
// not close from here, the server lifecycle owns it.
type RequestLogTracker struct {
	mu       sync.Mutex
	db       *sql.DB
	pruneStop chan struct{}
}

var GlobalRequestLogTracker *RequestLogTracker

func NewRequestLogTracker(db *sql.DB) *RequestLogTracker {
	rt := &RequestLogTracker{db: db, pruneStop: make(chan struct{})}
	if db != nil {
		if _, err := db.Exec(requestLogSchema); err != nil {
			rt.db = nil
			return rt
		}
		rt.migrate()
		go rt.pruneLoop()
	}
	GlobalRequestLogTracker = rt
	return rt
}

// migrate adds new request_logs columns to pre-existing databases. Uses
// PRAGMA table_info to feature-detect — ALTER TABLE ADD COLUMN is idempotent
// in practice but errors if the column already exists, so we check first.
func (rt *RequestLogTracker) migrate() {
	if rt.db == nil {
		return
	}
	rows, err := rt.db.Query(`PRAGMA table_info(request_logs)`)
	if err != nil {
		return
	}
	have := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err == nil {
			have[name] = true
		}
	}
	rows.Close()

	wants := []struct {
		name string
		ddl  string
	}{
		{"caveman_level", "ALTER TABLE request_logs ADD COLUMN caveman_level TEXT"},
		{"rtk_bytes_before", "ALTER TABLE request_logs ADD COLUMN rtk_bytes_before INTEGER DEFAULT 0"},
		{"rtk_bytes_after", "ALTER TABLE request_logs ADD COLUMN rtk_bytes_after INTEGER DEFAULT 0"},
		{"rtk_filters", "ALTER TABLE request_logs ADD COLUMN rtk_filters TEXT"},
	}
	for _, c := range wants {
		if !have[c.name] {
			_, _ = rt.db.Exec(c.ddl)
		}
	}
}

// uuidv4 — RFC 4122 v4, no external dependency.
func uuidv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// RecordRequest inserts one row. Body is truncated to requestLogBodyCap; we
// allocate an ID if the caller didn't already (so a handler that builds the
// log progressively can pass the same ID twice and use UpdateRequest).
func (rt *RequestLogTracker) RecordRequest(r RequestLog) string {
	if rt == nil || rt.db == nil {
		return r.ID
	}
	if r.ID == "" {
		r.ID = uuidv4()
	}
	if r.Timestamp == 0 {
		r.Timestamp = time.Now().Unix()
	}
	if len(r.RequestBody) > requestLogBodyCap {
		r.RequestBody = r.RequestBody[:requestLogBodyCap] + "\n...[truncated]"
	}
	if r.PromptTokensPre > 0 && r.PromptTokens > 0 && r.PromptTokensPre > r.PromptTokens {
		r.SaverReductionPct = 100 - (r.PromptTokens*100)/r.PromptTokensPre
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	_, _ = rt.db.Exec(`
		INSERT INTO request_logs
		(id, ts, model, provider, feature, account, latency_ms, status, http_status,
		 error_message, request_body, prompt_tokens, completion_tokens,
		 prompt_tokens_pre, compaction_mode, saver_reduction_pct,
		 caveman_level, rtk_bytes_before, rtk_bytes_after, rtk_filters)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Timestamp, r.Model, r.Provider, r.Feature, r.Account, r.LatencyMs,
		r.Status, r.HTTPStatus, r.ErrorMessage, r.RequestBody,
		r.PromptTokens, r.CompletionTokens, r.PromptTokensPre,
		r.CompactionMode, r.SaverReductionPct,
		r.CavemanLevel, r.RtkBytesBefore, r.RtkBytesAfter, r.RtkFilters,
	)
	return r.ID
}

func (rt *RequestLogTracker) ListRequests(limit, offset int, statusFilter, modelFilter string) ([]RequestLog, int) {
	if rt == nil || rt.db == nil {
		return nil, 0
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	where := ""
	args := []interface{}{}
	if statusFilter != "" && statusFilter != "all" {
		where += " WHERE status = ?"
		args = append(args, statusFilter)
	}
	if modelFilter != "" {
		if where == "" {
			where = " WHERE model LIKE ?"
		} else {
			where += " AND model LIKE ?"
		}
		args = append(args, "%"+modelFilter+"%")
	}

	// Count
	var total int
	rt.db.QueryRow("SELECT COUNT(*) FROM request_logs"+where, args...).Scan(&total)

	args = append(args, limit, offset)
	rows, err := rt.db.Query(`
		SELECT id, ts, model, provider, feature, account, latency_ms, status, http_status,
		       error_message, prompt_tokens, completion_tokens, prompt_tokens_pre,
		       compaction_mode, saver_reduction_pct,
		       caveman_level, rtk_bytes_before, rtk_bytes_after, rtk_filters
		FROM request_logs`+where+` ORDER BY ts DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	out := make([]RequestLog, 0, limit)
	for rows.Next() {
		var r RequestLog
		var errMsg, compactMode, account sql.NullString
		var provider, model, feature sql.NullString
		var cavemanLevel, rtkFilters sql.NullString
		var httpStatus, promptPre, reduction sql.NullInt64
		var rtkBefore, rtkAfter sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Timestamp, &model, &provider, &feature, &account,
			&r.LatencyMs, &r.Status, &httpStatus, &errMsg,
			&r.PromptTokens, &r.CompletionTokens, &promptPre, &compactMode, &reduction,
			&cavemanLevel, &rtkBefore, &rtkAfter, &rtkFilters); err != nil {
			continue
		}
		r.Model = model.String
		r.Provider = provider.String
		r.Feature = feature.String
		r.Account = account.String
		r.HTTPStatus = int(httpStatus.Int64)
		r.ErrorMessage = errMsg.String
		r.PromptTokensPre = int(promptPre.Int64)
		r.CompactionMode = compactMode.String
		r.SaverReductionPct = int(reduction.Int64)
		r.CavemanLevel = cavemanLevel.String
		r.RtkBytesBefore = int(rtkBefore.Int64)
		r.RtkBytesAfter = int(rtkAfter.Int64)
		r.RtkFilters = rtkFilters.String
		out = append(out, r)
	}
	return out, total
}

func (rt *RequestLogTracker) GetRequest(id string) (*RequestLog, bool) {
	if rt == nil || rt.db == nil || id == "" {
		return nil, false
	}
	row := rt.db.QueryRow(`
		SELECT id, ts, model, provider, feature, account, latency_ms, status, http_status,
		       error_message, request_body, prompt_tokens, completion_tokens,
		       prompt_tokens_pre, compaction_mode, saver_reduction_pct,
		       caveman_level, rtk_bytes_before, rtk_bytes_after, rtk_filters
		FROM request_logs WHERE id = ?`, id)

	var r RequestLog
	var model, provider, feature, account, errMsg, body, compactMode sql.NullString
	var cavemanLevel, rtkFilters sql.NullString
	var httpStatus, promptPre, reduction sql.NullInt64
	var rtkBefore, rtkAfter sql.NullInt64
	err := row.Scan(&r.ID, &r.Timestamp, &model, &provider, &feature, &account,
		&r.LatencyMs, &r.Status, &httpStatus, &errMsg, &body,
		&r.PromptTokens, &r.CompletionTokens, &promptPre, &compactMode, &reduction,
		&cavemanLevel, &rtkBefore, &rtkAfter, &rtkFilters)
	if err != nil {
		return nil, false
	}
	r.Model = model.String
	r.Provider = provider.String
	r.Feature = feature.String
	r.Account = account.String
	r.HTTPStatus = int(httpStatus.Int64)
	r.ErrorMessage = errMsg.String
	r.RequestBody = body.String
	r.PromptTokensPre = int(promptPre.Int64)
	r.CompactionMode = compactMode.String
	r.SaverReductionPct = int(reduction.Int64)
	r.CavemanLevel = cavemanLevel.String
	r.RtkBytesBefore = int(rtkBefore.Int64)
	r.RtkBytesAfter = int(rtkAfter.Int64)
	r.RtkFilters = rtkFilters.String
	return &r, true
}

// RecordAutoScript upserts an auto-script run. Called twice per batch:
// once on start (status=running, ts_end=0) and once on completion (terminal
// status + populated counts/logs). Logs JSON capped to autoScriptLogsCap.
func (rt *RequestLogTracker) RecordAutoScript(r AutoScriptRun) {
	if rt == nil || rt.db == nil {
		return
	}
	if r.ID == "" {
		r.ID = uuidv4()
	}
	if r.TimestampStart == 0 {
		r.TimestampStart = time.Now().Unix()
	}
	if len(r.LogsJSON) > autoScriptLogsCap {
		// keep tail (most recent entries are most useful)
		idx := len(r.LogsJSON) - autoScriptLogsCap
		if i := strings.IndexByte(r.LogsJSON[idx:], ','); i >= 0 {
			idx += i + 1
		}
		r.LogsJSON = "[...truncated]" + r.LogsJSON[idx:]
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	_, _ = rt.db.Exec(`
		INSERT INTO auto_script_runs (id, ts_start, ts_end, trigger, provider,
		    attempted, succeeded, failed, status, logs_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		    ts_end = excluded.ts_end,
		    attempted = excluded.attempted,
		    succeeded = excluded.succeeded,
		    failed = excluded.failed,
		    status = excluded.status,
		    logs_json = excluded.logs_json`,
		r.ID, r.TimestampStart, r.TimestampEnd, r.Trigger, r.Provider,
		r.Attempted, r.Succeeded, r.Failed, r.Status, r.LogsJSON,
	)
}

func (rt *RequestLogTracker) ListAutoScripts(limit int) []AutoScriptRun {
	if rt == nil || rt.db == nil {
		return nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := rt.db.Query(`
		SELECT id, ts_start, ts_end, trigger, provider, attempted, succeeded, failed, status
		FROM auto_script_runs ORDER BY ts_start DESC LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]AutoScriptRun, 0, limit)
	for rows.Next() {
		var r AutoScriptRun
		var tsEnd sql.NullInt64
		var trigger, provider, status sql.NullString
		if err := rows.Scan(&r.ID, &r.TimestampStart, &tsEnd, &trigger, &provider,
			&r.Attempted, &r.Succeeded, &r.Failed, &status); err != nil {
			continue
		}
		r.TimestampEnd = tsEnd.Int64
		r.Trigger = trigger.String
		r.Provider = provider.String
		r.Status = status.String
		out = append(out, r)
	}
	return out
}

func (rt *RequestLogTracker) GetAutoScript(id string) (*AutoScriptRun, bool) {
	if rt == nil || rt.db == nil || id == "" {
		return nil, false
	}
	row := rt.db.QueryRow(`
		SELECT id, ts_start, ts_end, trigger, provider, attempted, succeeded, failed, status, logs_json
		FROM auto_script_runs WHERE id = ?`, id)

	var r AutoScriptRun
	var tsEnd sql.NullInt64
	var trigger, provider, status, logsJSON sql.NullString
	if err := row.Scan(&r.ID, &r.TimestampStart, &tsEnd, &trigger, &provider,
		&r.Attempted, &r.Succeeded, &r.Failed, &status, &logsJSON); err != nil {
		return nil, false
	}
	r.TimestampEnd = tsEnd.Int64
	r.Trigger = trigger.String
	r.Provider = provider.String
	r.Status = status.String
	r.LogsJSON = logsJSON.String
	return &r, true
}

// pruneLoop drops oldest rows beyond the ring size every 60s. Per-insert
// deletes would dominate write latency on busy gateways, so we batch.
func (rt *RequestLogTracker) pruneLoop() {
	t := time.NewTicker(pruneInterval)
	defer t.Stop()
	for {
		select {
		case <-rt.pruneStop:
			return
		case <-t.C:
			rt.prune()
		}
	}
}

func (rt *RequestLogTracker) prune() {
	if rt.db == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	_, _ = rt.db.Exec(`
		DELETE FROM request_logs WHERE id NOT IN (
		    SELECT id FROM request_logs ORDER BY ts DESC LIMIT ?
		)`, requestLogRingSize)
	_, _ = rt.db.Exec(`
		DELETE FROM auto_script_runs WHERE id NOT IN (
		    SELECT id FROM auto_script_runs ORDER BY ts_start DESC LIMIT ?
		)`, autoScriptRingSize)
}

// Stop signals the prune loop to exit. Safe to call multiple times.
func (rt *RequestLogTracker) Stop() {
	select {
	case <-rt.pruneStop:
	default:
		close(rt.pruneStop)
	}
}
