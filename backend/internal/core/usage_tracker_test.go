package core

import (
	"path/filepath"
	"testing"
)

// TestUsageTrackerRoundTrip exercises the SQLite-backed tracker: open,
// insert 3 rows, aggregate without filter, aggregate with model filter.
// Catches regressions in schema DDL + Query/Record wiring.
func TestUsageTrackerRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	ut := NewUsageTracker(db)
	defer ut.Close()

	ut.Record(UsageRecord{Timestamp: 1700000000, Model: "qwen-max", Feature: "chat", PromptTokens: 10, CompletionTokens: 20, Success: true})
	ut.Record(UsageRecord{Timestamp: 1700000100, Model: "qwen-max", Feature: "chat", PromptTokens: 5, CompletionTokens: 15, Success: false})
	ut.Record(UsageRecord{Timestamp: 1700000200, Model: "qwen-turbo", Feature: "t2i", PromptTokens: 0, CompletionTokens: 0, Success: true})

	all := ut.Query(1699000000, 1701000000, "")
	if all.TotalRequests != 3 {
		t.Errorf("TotalRequests = %d, want 3", all.TotalRequests)
	}
	if all.TotalTokens != 50 {
		t.Errorf("TotalTokens = %d, want 50 (10+20+5+15)", all.TotalTokens)
	}
	if all.SuccessCount != 2 || all.ErrorCount != 1 {
		t.Errorf("success/error = %d/%d, want 2/1", all.SuccessCount, all.ErrorCount)
	}
	if len(all.Models) != 2 {
		t.Errorf("Models = %v, want 2 entries", all.Models)
	}
	if all.ByFeature["chat"].Requests != 2 || all.ByFeature["t2i"].Requests != 1 {
		t.Errorf("ByFeature = %+v, want chat=2 t2i=1", all.ByFeature)
	}

	filtered := ut.Query(1699000000, 1701000000, "qwen-max")
	if filtered.TotalRequests != 2 {
		t.Errorf("filtered TotalRequests = %d, want 2", filtered.TotalRequests)
	}
	if filtered.TotalTokens != 50 {
		t.Errorf("filtered TotalTokens = %d, want 50", filtered.TotalTokens)
	}
}

// TestUsageTrackerDegradedNilDB ensures Record/Query no-op (don't panic)
// when the tracker was constructed with a nil DB — the graceful-degradation
// path documented in NewUsageTracker.
func TestUsageTrackerDegradedNilDB(t *testing.T) {
	ut := NewUsageTracker(nil)
	ut.Record(UsageRecord{Timestamp: 1, Model: "x", Feature: "chat", Success: true})
	stats := ut.Query(0, 1<<31, "")
	if stats == nil {
		t.Fatal("Query returned nil with degraded tracker")
	}
	if stats.TotalRequests != 0 {
		t.Errorf("degraded TotalRequests = %d, want 0", stats.TotalRequests)
	}
}
