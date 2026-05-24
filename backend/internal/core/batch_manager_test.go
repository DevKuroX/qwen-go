package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

type fakeEngine struct {
	mu      sync.Mutex
	count   int
	blockCh chan struct{}
	stopped bool
}

func (f *fakeEngine) Name() string { return "fake" }

func (f *fakeEngine) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count
}

func (f *fakeEngine) Register(ctx context.Context, req models.RegistrationRequest, onLog func(string)) (*models.Account, error) {
	f.mu.Lock(); f.count++; f.mu.Unlock()
	if onLog != nil { onLog("log") }
	if f.blockCh != nil {
		select {
		case <-f.blockCh:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &models.Account{Email: time.Now().Format("20060102150405") + "@example.com", CreatedAt: time.Now()}, nil
}

func TestBatchManagerThreadsMeaningReal(t *testing.T) {
	engine := &fakeEngine{blockCh: make(chan struct{})}
	pool := NewAccountPool()
	bm := NewBatchManager(pool, engine, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobID, err := bm.StartBatch(ctx, models.BatchRequest{Provider: "qwen", Count: 3, Threads: 2, MailProvider: "guerrilla"})
	if err != nil { t.Fatalf("StartBatch() error = %v", err) }
	_ = jobID
	time.Sleep(50 * time.Millisecond)
	if engine.Count() < 2 { t.Fatalf("Register count = %d, want >= 2", engine.Count()) }
	close(engine.blockCh)
}

func TestBatchManagerStopTransitionsState(t *testing.T) {
	engine := &fakeEngine{blockCh: make(chan struct{})}
	bm := NewBatchManager(NewAccountPool(), engine, nil)
	jobID, err := bm.StartBatch(context.Background(), models.BatchRequest{Provider: "qwen", Count: 1, Threads: 1, MailProvider: "guerrilla"})
	if err != nil { t.Fatalf("StartBatch() error = %v", err) }
	if err := bm.StopJob(jobID); err != nil { t.Fatalf("StopJob() error = %v", err) }
	job, _ := bm.GetJob(jobID)
	if job.Snapshot().Status != models.BatchStatusStopped { t.Fatalf("Status = %s, want stopped", job.Snapshot().Status) }
	close(engine.blockCh)
}

func TestBatchJobAddLogRaceSafe(t *testing.T) {
	job := &models.BatchJob{}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); job.AddLog("info", "x") }()
	}
	wg.Wait()
	if len(job.Snapshot().Logs) != 20 { t.Fatalf("log len = %d, want 20", len(job.Snapshot().Logs)) }
}

func TestBatchManagerJobLifecycle(t *testing.T) {
	engine := &fakeEngine{}
	bm := NewBatchManager(NewAccountPool(), engine, nil)
	jobID, err := bm.StartBatch(context.Background(), models.BatchRequest{Provider: "qwen", Count: 1, Threads: 1, MailProvider: "guerrilla"})
	if err != nil { t.Fatalf("StartBatch() error = %v", err) }
	if _, err := bm.GetJob(jobID); err != nil { t.Fatalf("GetJob() error = %v", err) }
	time.Sleep(20 * time.Millisecond)
	job, _ := bm.GetJob(jobID)
	if job.Status != models.BatchStatusCompleted { t.Fatalf("Status = %s, want completed", job.Status) }
}

func TestBatchManagerMailProviderExplicitHybridPath(t *testing.T) {
	GlobalSettingsManager = &SettingsManager{settings: DynamicSettings{TempMailDomain: "tempmail.example", TempMailKey: "key"}}
	engine := NewRodEngine()
	if _, label, err := engine.createMailClient("tempmail"); err != nil || label != "TempMail" {
		t.Fatalf("createMailClient() label=%s err=%v", label, err)
	}
}

func TestBatchManagerMailProviderUnsupportedDoesNotFallbackSilently(t *testing.T) {
	engine := NewRodEngine()
	if _, _, err := engine.createMailClient("moemail"); err == nil {
		t.Fatal("createMailClient() error = nil, want explicit unsupported error")
	}
}

func TestRodEngineSupportsConfiguredMoeMailAndLocalMail(t *testing.T) {
	GlobalSettingsManager = &SettingsManager{settings: DynamicSettings{MoeMailDomain: "https://moemail.example", MoeMailKey: "key"}}
	engine := NewRodEngine()
	if _, label, err := engine.createMailClient("moemail"); err != nil || label != "MoeMail" {
		t.Fatalf("moemail label=%s err=%v", label, err)
	}
	if _, label, err := engine.createMailClient("local"); err != nil || label != "LocalMail" {
		t.Fatalf("local label=%s err=%v", label, err)
	}
}
