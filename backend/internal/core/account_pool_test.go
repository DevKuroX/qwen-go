package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

func TestAccountPoolAcquireReturnsBestEligibleAccount(t *testing.T) {
	pool := NewAccountPool()
	pool.Load([]*models.Account{
		{Email: "low@example.com", Provider: "qwen", Status: models.StatusValid, Valid: true, Inflight: 4},
		{Email: "best@example.com", Provider: "qwen", Status: models.StatusValid, Valid: true},
	})

	acc, err := pool.Acquire(context.Background(), "qwen", map[string]bool{})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if acc.Email != "best@example.com" {
		t.Fatalf("Acquire() got %q, want %q", acc.Email, "best@example.com")
	}

	pool.Release(acc)

	accounts := pool.ListAccounts()
	if len(accounts) != 2 {
		t.Fatalf("ListAccounts() len = %d, want 2", len(accounts))
	}

	for _, listed := range accounts {
		if listed.Email == "best@example.com" && listed.Inflight != 0 {
			t.Fatalf("released account inflight = %d, want 0", listed.Inflight)
		}
	}
}

func TestAccountPoolAcquirePreservesSkippedAccounts(t *testing.T) {
	pool := NewAccountPool()
	pool.Load([]*models.Account{
		{Email: "blocked@example.com", Provider: "qwen", Status: models.StatusValid, Valid: true, Inflight: 1},
		{Email: "eligible@example.com", Provider: "qwen", Status: models.StatusValid, Valid: true},
	})

	acc, err := pool.Acquire(context.Background(), "qwen", nil)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if acc.Email != "eligible@example.com" {
		t.Fatalf("Acquire() got %q, want %q", acc.Email, "eligible@example.com")
	}
	pool.Release(acc)

	accounts := pool.ListAccounts()
	if len(accounts) != 2 {
		t.Fatalf("ListAccounts() len = %d, want 2", len(accounts))
	}

	for _, listed := range accounts {
		if listed.Email == "blocked@example.com" && listed.Inflight != 1 {
			t.Fatalf("blocked account inflight = %d, want 1", listed.Inflight)
		}
	}
}

func TestAccountPoolAcquireSkipsRateLimitedAndBanned(t *testing.T) {
	pool := NewAccountPool()
	pool.Load([]*models.Account{
		{Email: "banned@example.com", Provider: "qwen", Status: models.StatusBanned},
		{Email: "rate@example.com", Provider: "qwen", Status: models.StatusRateLimited, RateLimitedUntil: float64(time.Now().Add(time.Hour).Unix())},
		{Email: "good@example.com", Provider: "qwen", Status: models.StatusValid, Valid: true},
	})

	acc, err := pool.Acquire(context.Background(), "qwen", nil)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if acc.Email != "good@example.com" {
		t.Fatalf("Acquire() got %q, want %q", acc.Email, "good@example.com")
	}
}

func TestAccountPoolReleaseClampsInflightToZero(t *testing.T) {
	pool := NewAccountPool()
	pool.Load([]*models.Account{{Email: "solo@example.com", Provider: "qwen", Status: models.StatusValid, Valid: true}})

	acc, err := pool.Acquire(context.Background(), "qwen", nil)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	acc.Inflight = 0
	pool.Release(acc)

	listed := pool.ListAccounts()[0]
	if listed.Inflight != 0 {
		t.Fatalf("Inflight = %d, want 0", listed.Inflight)
	}
}

func TestAccountPoolAcquireReturnsErrNoAccountsWhenAllBlocked(t *testing.T) {
	pool := NewAccountPool()
	pool.Load([]*models.Account{{Email: "blocked@example.com", Provider: "qwen", Status: models.StatusBanned}})

	_, err := pool.Acquire(context.Background(), "qwen", map[string]bool{"blocked@example.com": true})
	if !errors.Is(err, ErrNoAccounts) {
		t.Fatalf("Acquire() error = %v, want ErrNoAccounts", err)
	}
}

func TestAccountPoolRateLimitedAutoRecovery(t *testing.T) {
	pool := NewAccountPool()
	pool.recoveryInterval = time.Millisecond
	pool.Load([]*models.Account{{
		Email: "rate@example.com", Provider: "qwen", Status: models.StatusRateLimited,
		RateLimitedUntil: float64(time.Now().Add(-time.Second).Unix()),
	}})

	pool.recoverAccounts()
	if got := pool.ListAccounts()[0].Status; got != models.StatusValid {
		t.Fatalf("Status = %s, want %s", got, models.StatusValid)
	}
}

func TestAccountPoolCircuitOpenTransitionsToHalfOpen(t *testing.T) {
	pool := NewAccountPool()
	pool.recoveryInterval = time.Millisecond
	pool.Load([]*models.Account{{
		Email: "circuit@example.com", Provider: "qwen", Status: models.StatusCircuitOpen,
		RateLimitedUntil: float64(time.Now().Add(-time.Second).Unix()),
	}})

	pool.recoverAccounts()
	if got := pool.ListAccounts()[0].Status; got != models.StatusHalfOpen {
		t.Fatalf("Status = %s, want %s", got, models.StatusHalfOpen)
	}
}

func TestAccountPoolHalfOpenSuccessReturnsValid(t *testing.T) {
	pool := NewAccountPool()
	pool.Load([]*models.Account{{Email: "probe@example.com", Provider: "qwen", Status: models.StatusHalfOpen}})

	pool.MarkSuccess(&models.Account{Email: "probe@example.com"})
	if got := pool.ListAccounts()[0].Status; got != models.StatusValid {
		t.Fatalf("Status = %s, want %s", got, models.StatusValid)
	}
}

func TestAccountPoolHalfOpenFailureReopensCircuit(t *testing.T) {
	pool := NewAccountPool()
	pool.Load([]*models.Account{{Email: "probe@example.com", Provider: "qwen", Status: models.StatusHalfOpen}})

	for i := 0; i < 5; i++ {
		pool.MarkError("probe@example.com", "transient", "temporary")
	}

	if got := pool.ListAccounts()[0].Status; got != models.StatusCircuitOpen {
		t.Fatalf("Status = %s, want %s", got, models.StatusCircuitOpen)
	}
}
