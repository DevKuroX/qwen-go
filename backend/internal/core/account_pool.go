package core

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

var (
	ErrNoAccounts     = errors.New("no available accounts in pool")
	ErrPoolEmpty      = errors.New("account pool is empty")
)

type AccountHeap []*models.Account

func (h AccountHeap) Len() int           { return len(h) }
func (h AccountHeap) Less(i, j int) bool { return h[i].Score > h[j].Score }
func (h AccountHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *AccountHeap) Push(x interface{}) {
	*h = append(*h, x.(*models.Account))
}

func (h *AccountHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type AccountPool struct {
	mu         sync.RWMutex
	heap       AccountHeap
	accounts   map[string]*models.Account
	exclusions map[string]time.Time
	recoveryInterval time.Duration
	stopRecovery     chan struct{}
	stopOnce         sync.Once

	// Snapshot loop (BACKLOG #22). Owned by StartSnapshotLoop; nil when not
	// running. flushNow lets event-driven critical transitions (Banned /
	// CircuitOpen) bypass the periodic tick.
	snapshotStore    *AccountStore
	stopSnapshot     chan struct{}
	stopSnapshotOnce sync.Once
	flushNow         chan struct{}
}

func NewAccountPool() *AccountPool {
	return &AccountPool{
		accounts:         make(map[string]*models.Account),
		exclusions:       make(map[string]time.Time),
		recoveryInterval: 30 * time.Second,
		stopRecovery:     make(chan struct{}),
	}
}

func (p *AccountPool) Load(accounts []*models.Account) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.heap = make(AccountHeap, 0)
	p.accounts = make(map[string]*models.Account)
	
	for _, acc := range accounts {
		if acc.DeletedAt != nil {
			continue
		}
		if acc.Provider == "" {
			acc.Provider = "qwen"
		}
		if acc.Status == "" && acc.StatusCode != "" {
			acc.Status = models.AccountStatus(acc.StatusCode)
		}
		if acc.Status == "" && acc.Valid {
			acc.Status = models.StatusValid
		}
		acc.Score = acc.ComputeScore()
		p.accounts[acc.Email] = acc
		heap.Push(&p.heap, acc)
	}
	
	heap.Init(&p.heap)
}

func (p *AccountPool) Acquire(ctx context.Context, providerName string, exclude map[string]bool) (*models.Account, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if p.heap.Len() == 0 {
		return nil, ErrNoAccounts
	}

	skipped := make(AccountHeap, 0, p.heap.Len())
	for p.heap.Len() > 0 {
		acc := heap.Pop(&p.heap).(*models.Account)
		if providerName != "" && acc.Provider != providerName {
			skipped = append(skipped, acc)
			continue
		}
		if !p.isAvailable(acc, now, exclude) {
			skipped = append(skipped, acc)
			continue
		}

		acc.Inflight++
		acc.LastRequestTime = now
		p.accounts[acc.Email] = acc

		for _, skippedAcc := range skipped {
			heap.Push(&p.heap, skippedAcc)
		}

		return acc, nil
	}

	for _, skippedAcc := range skipped {
		heap.Push(&p.heap, skippedAcc)
	}

	return nil, ErrNoAccounts
}

// Touch persists auth-material updates the provider just applied to acc
// (rotated refresh token, new access token, updated metadata). The pool
// itself stays in-memory; persistence is delegated to the snapshot loop
// (BACKLOG #22). Today this is a no-op marker — it exists so providers
// can call it without conditional compilation when the snapshot loop lands.
func (p *AccountPool) Touch(acc *models.Account) {
	if acc == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// Already a pointer reference in p.accounts — fields the provider
	// mutated are visible. Refresh score in case ExpiresAt rotation
	// touched anything score-relevant.
	if existing, ok := p.accounts[acc.Email]; ok {
		existing.Score = existing.ComputeScore()
	}
}

func (p *AccountPool) Release(acc *models.Account) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if existing, ok := p.accounts[acc.Email]; ok {
		existing.Inflight--
		if existing.Inflight < 0 {
			existing.Inflight = 0
		}
		existing.Score = existing.ComputeScore()
		if existing.DeletedAt == nil {
			heap.Push(&p.heap, existing)
		}
	}
}

func (p *AccountPool) isAvailable(acc *models.Account, now time.Time, exclude map[string]bool) bool {
	if acc == nil {
		return false
	}
	if exclude != nil && exclude[acc.Email] {
		return false
	}
	if excludedUntil, ok := p.exclusions[acc.Email]; ok && now.Before(excludedUntil) {
		return false
	}
	if acc.DeletedAt != nil {
		return false
	}
	if acc.Status == models.StatusBanned {
		return false
	}
	if acc.Status == models.StatusRateLimited && acc.RateLimitedUntil > float64(now.Unix()) {
		return false
	}
	if acc.Inflight >= p.getMaxInflight(acc) {
		return false
	}
	return true
}

func (p *AccountPool) AddAccount(acc *models.Account) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	acc.Score = acc.ComputeScore()
	p.accounts[acc.Email] = acc
	heap.Push(&p.heap, acc)
}

func (p *AccountPool) RemoveAccount(email string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	delete(p.accounts, email)
	delete(p.exclusions, email)
	
	newHeap := make(AccountHeap, 0)
	for _, acc := range p.accounts {
		heap.Push(&newHeap, acc)
	}
	p.heap = newHeap
}

func (p *AccountPool) MarkError(email string, errorType string, errMsg string) {
	p.mu.Lock()
	acc, ok := p.accounts[email]
	if !ok {
		p.mu.Unlock()
		return
	}

	prevStatus := acc.Status
	acc.LastError = errMsg

	switch errorType {
	case "rate_limit":
		acc.Status = models.StatusRateLimited
		acc.RateLimitedUntil = float64(time.Now().Add(30 * time.Minute).Unix())
		acc.RateLimitCount++
	case "auth":
		acc.Status = models.StatusSoftError
		acc.ConsecutiveFailures++
	case "banned":
		acc.Status = models.StatusBanned
	case "transient":
		acc.ConsecutiveFailures++
		if acc.ConsecutiveFailures >= 5 {
			acc.Status = models.StatusCircuitOpen
			acc.CircuitOpenCount++
			acc.RateLimitedUntil = float64(time.Now().Add(30 * time.Second).Unix())
		}
	}

	acc.Score = acc.ComputeScore()

	// Capture whether this MarkError transitioned the account into a
	// terminal/blocking state. We want those to hit disk immediately so a
	// crash before the next periodic snapshot doesn't reset the account back
	// to Valid on restart.
	criticalTransition := prevStatus != acc.Status &&
		(acc.Status == models.StatusBanned || acc.Status == models.StatusCircuitOpen)
	p.mu.Unlock()

	if criticalTransition {
		p.triggerFlush()
	}
}

func (p *AccountPool) MarkSuccess(acc *models.Account) {
	p.mu.Lock()
	defer p.mu.Unlock()

	existing, ok := p.accounts[acc.Email]
	if !ok {
		return
	}

	existing.Status = models.StatusValid
	existing.ConsecutiveFailures = 0
	existing.RateLimitedUntil = 0
	existing.LastError = ""
	existing.Score = existing.ComputeScore()
}

func (p *AccountPool) StartRecoveryLoop() {
	go func() {
		ticker := time.NewTicker(p.recoveryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.recoverAccounts()
			case <-p.stopRecovery:
				return
			}
		}
	}()
}

func (p *AccountPool) StopRecoveryLoop() {
	p.stopOnce.Do(func() { close(p.stopRecovery) })
}

func (p *AccountPool) RecoveryStopped() <-chan struct{} {
	return p.stopRecovery
}

func (p *AccountPool) recoverAccounts() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	changed := false
	for _, acc := range p.accounts {
		switch acc.Status {
		case models.StatusRateLimited:
			if acc.RateLimitedUntil > 0 && acc.RateLimitedUntil <= float64(now.Unix()) {
				acc.Status = models.StatusValid
				acc.RateLimitedUntil = 0
				acc.Score = acc.ComputeScore()
				changed = true
			}
		case models.StatusCircuitOpen:
			if acc.RateLimitedUntil > 0 && acc.RateLimitedUntil <= float64(now.Unix()) {
				acc.Status = models.StatusHalfOpen
				acc.Score = acc.ComputeScore()
				changed = true
			}
		case models.StatusHalfOpen:
			if acc.Inflight == 0 {
				acc.Status = models.StatusValid
				acc.ConsecutiveFailures = 0
				acc.Score = acc.ComputeScore()
				changed = true
			}
		}
	}
	if changed {
		p.rebuildHeapLocked()
	}
}

func (p *AccountPool) rebuildHeapLocked() {
	p.heap = make(AccountHeap, 0, len(p.accounts))
	for _, acc := range p.accounts {
		if acc.DeletedAt == nil {
			heap.Push(&p.heap, acc)
		}
	}
	heap.Init(&p.heap)
}

func (p *AccountPool) GetStatus() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	stats := map[string]int{
		"total":         len(p.accounts),
		"valid":         0,
		"rate_limited":  0,
		"soft_error":    0,
		"circuit_open":  0,
		"banned":        0,
	}
	
	for _, acc := range p.accounts {
		switch acc.Status {
		case models.StatusValid:
			stats["valid"]++
		case models.StatusRateLimited:
			stats["rate_limited"]++
		case models.StatusSoftError:
			stats["soft_error"]++
		case models.StatusCircuitOpen:
			stats["circuit_open"]++
		case models.StatusBanned:
			stats["banned"]++
		}
	}
	
	return map[string]interface{}{
		"total":         stats["total"],
		"valid":         stats["valid"],
		"rate_limited":  stats["rate_limited"],
		"soft_error":    stats["soft_error"],
		"circuit_open":  stats["circuit_open"],
		"banned":        stats["banned"],
		"pressure":      float64(stats["valid"]) / float64(max(stats["total"], 1)),
	}
}

func (p *AccountPool) ListAccounts() []*models.Account {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	accounts := make([]*models.Account, 0, len(p.accounts))
	for _, acc := range p.accounts {
		accounts = append(accounts, acc)
	}
	return accounts
}

func (p *AccountPool) getMaxInflight(acc *models.Account) int {
	if GlobalConfig != nil {
		return GlobalConfig.MaxInflight
	}
	return 1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (p *AccountPool) CountByProvider(provider string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	count := 0
	for _, acc := range p.accounts {
		if acc.Provider == provider {
			count++
		}
	}
	return count
}

func (p *AccountPool) CountByStatus(provider string, status models.AccountStatus) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	count := 0
	for _, acc := range p.accounts {
		if acc.Provider == provider && acc.Status == status {
			count++
		}
	}
	return count
}

func (p *AccountPool) GetAccountsByProvider(provider string) []*models.Account {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	accounts := make([]*models.Account, 0)
	for _, acc := range p.accounts {
		if acc.Provider == provider {
			accounts = append(accounts, acc)
		}
	}
	return accounts
}

func (p *AccountPool) SoftDeleteAccount(email string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	acc, exists := p.accounts[email]
	if !exists {
		return fmt.Errorf("account not found: %s", email)
	}

	now := time.Now()
	acc.DeletedAt = &now
	acc.Status = models.StatusBanned

	newHeap := make(AccountHeap, 0)
	for _, a := range p.accounts {
		if a.DeletedAt == nil {
			heap.Push(&newHeap, a)
		}
	}
	p.heap = newHeap

	return nil
}

// StartSnapshotLoop spins a goroutine that persists ListAccounts() to the
// store every `interval`. Also drains an internal flushNow channel so
// MarkError can request an immediate write when a critical transition
// happens (Banned, CircuitOpen) — protects against losing those state
// changes if the process crashes between ticks.
//
// Safe to call once at boot. Subsequent calls no-op (a loop is already
// running). Caller must hold the store handle for the process lifetime.
func (p *AccountPool) StartSnapshotLoop(interval time.Duration, store *AccountStore) {
	if store == nil {
		return
	}
	p.mu.Lock()
	if p.snapshotStore != nil {
		p.mu.Unlock()
		return
	}
	p.snapshotStore = store
	p.stopSnapshot = make(chan struct{})
	p.flushNow = make(chan struct{}, 1)
	p.stopSnapshotOnce = sync.Once{}
	p.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.snapshotOnce()
			case <-p.flushNow:
				p.snapshotOnce()
			case <-p.stopSnapshot:
				p.snapshotOnce() // final flush on shutdown
				return
			}
		}
	}()
}

// StopSnapshotLoop signals the snapshot goroutine to drain and exit. The
// goroutine performs one final write before returning. Idempotent.
func (p *AccountPool) StopSnapshotLoop() {
	p.mu.RLock()
	stopCh := p.stopSnapshot
	p.mu.RUnlock()
	if stopCh == nil {
		return
	}
	p.stopSnapshotOnce.Do(func() { close(stopCh) })
}

// triggerFlush nudges the snapshot loop without blocking the caller. If the
// channel buffer is full a flush is already queued — coalescing multiple
// near-simultaneous critical transitions into one write is fine.
func (p *AccountPool) triggerFlush() {
	p.mu.RLock()
	ch := p.flushNow
	p.mu.RUnlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (p *AccountPool) snapshotOnce() {
	p.mu.RLock()
	store := p.snapshotStore
	p.mu.RUnlock()
	if store == nil {
		return
	}
	if err := store.Save(p.ListAccounts()); err != nil {
		// Best-effort. Log handled by caller via global zap if needed; pool
		// stays lean and dependency-free.
		_ = err
	}
}
