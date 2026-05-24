package core

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

func TestAccountStoreSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	store := NewAccountStore(path)
	want := []*models.Account{{
		Email: "a@example.com", Provider: "qwen", Status: models.StatusValid, Valid: true,
		RateLimitCount: 2, ConsecutiveFailures: 1,
	}}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := NewAccountStore(path).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 1 || got[0].Email != want[0].Email || got[0].Status != want[0].Status || got[0].RateLimitCount != want[0].RateLimitCount {
		t.Fatalf("Load() got %+v, want %+v", got, want)
	}
}

func TestProxyStoreSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxies.json")
	store := NewProxyStore(path)
	now := time.Unix(1700000000, 0).UTC()
	want := []*models.Proxy{{
		ID: "p1", Enabled: true, Type: "http", Host: "127.0.0.1", Port: 8080,
		Status: models.ProxyStatusLive, Region: "id", LatencyMs: 42, LastCheck: now,
	}}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := NewProxyStore(path).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 1 || got[0].Host != want[0].Host || got[0].Port != want[0].Port || got[0].Status != want[0].Status {
		t.Fatalf("Load() got %+v, want %+v", got, want)
	}
}

func TestStoreSaveCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "accounts.json")
	store := NewAccountStore(path)

	if err := store.Save([]*models.Account{{Email: "mkdir@example.com"}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}
