package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
	"go.uber.org/zap"
)

type BatchManager struct {
	jobs        map[string]*models.BatchJob
	activeJobs  map[string]string
	cancelFuncs map[string]context.CancelFunc
	mu          sync.RWMutex
	pool        *AccountPool
	engine      RegistrationEngine
	accountsDB  *JSONDatabase
	logger      *zap.Logger
}

func NewBatchManager(pool *AccountPool, engine RegistrationEngine, accountsDB *JSONDatabase) *BatchManager {
	return &BatchManager{
		jobs:        make(map[string]*models.BatchJob),
		activeJobs:  make(map[string]string),
		cancelFuncs: make(map[string]context.CancelFunc),
		pool:        pool,
		engine:      engine,
		accountsDB:  accountsDB,
		logger:      zap.L(),
	}
}

func (bm *BatchManager) StartBatch(ctx context.Context, req models.BatchRequest) (string, error) {
	bm.mu.Lock()

	if _, exists := bm.activeJobs[req.Provider]; exists {
		bm.mu.Unlock()
		return "", fmt.Errorf("provider %s already has an active batch job", req.Provider)
	}

	jobID := generateJobID()
	ctx, cancel := context.WithCancel(ctx)

	job := &models.BatchJob{
		ID:        jobID,
		Provider:  req.Provider,
		Count:     req.Count,
		Status:    models.BatchStatusRunning,
		Logs:      make([]models.LogEntry, 0),
		StartedAt: time.Now(),
	}

	bm.jobs[jobID] = job
	bm.activeJobs[req.Provider] = jobID
	bm.cancelFuncs[jobID] = cancel
	bm.mu.Unlock()

	go bm.runBatch(ctx, cancel, job, req)

	return jobID, nil
}

func (bm *BatchManager) runBatch(ctx context.Context, cancel context.CancelFunc, job *models.BatchJob, req models.BatchRequest) {
	defer func() {
		if r := recover(); r != nil {
			job.AddLog("error", fmt.Sprintf("Batch panicked: %v", r))
			job.Status = models.BatchStatusFailed
			now := time.Now()
			job.CompletedAt = &now
			bm.logger.Error("batch panicked", zap.Any("recover", r), zap.String("job_id", job.ID))
		}
		bm.persistAutoScript(job, "manual")
		bm.mu.Lock()
		delete(bm.activeJobs, job.Provider)
		delete(bm.cancelFuncs, job.ID)
		bm.mu.Unlock()
		cancel()
	}()

	job.AddLog("info", fmt.Sprintf("Starting batch registration: %d accounts via %s [engine: %s]", req.Count, req.MailProvider, bm.engine.Name()))
	bm.logger.Info("batch started",
		zap.String("job_id", job.ID),
		zap.String("provider", job.Provider),
		zap.String("engine", bm.engine.Name()),
		zap.Int("count", req.Count),
	)
	bm.persistAutoScript(job, "manual")

	workers := req.Threads
	if workers <= 0 {
		workers = 1
	}
	if workers > req.Count {
		workers = req.Count
	}

	regReq := models.RegistrationRequest{Count: 1, Threads: req.Threads, Provider: req.MailProvider}
	jobs := make(chan int)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for idx := range jobs {
			select {
			case <-ctx.Done():
				return
			default:
			}

			job.AddLog("info", fmt.Sprintf("[%d/%d] Starting registration...", idx, req.Count))
			onLog := func(msg string) { job.AddLog("info", fmt.Sprintf("[%d/%d] %s", idx, req.Count, msg)) }

			acc, err := bm.engine.Register(ctx, regReq, onLog)
			if err != nil {
				job.IncFailed()
				job.AddLog("error", fmt.Sprintf("[%d/%d] Failed: %s", idx, req.Count, err.Error()))
				bm.logger.Error("registration failed", zap.String("job_id", job.ID), zap.Int("attempt", idx), zap.Error(err))
				continue
			}

			acc.Provider = job.Provider
			if acc.CreatedAt.IsZero() { acc.CreatedAt = time.Now() }
			bm.pool.AddAccount(acc)
			if bm.accountsDB != nil {
				bm.accountsDB.Set(bm.pool.ListAccounts())
				if err := bm.accountsDB.Save(); err != nil { bm.logger.Error("failed to save accounts", zap.Error(err)) }
			}

			job.IncProgress()
			job.IncSuccess()
			job.AddLog("success", fmt.Sprintf("[%d/%d] ✓ Account registered: %s", idx, req.Count, acc.Email))
			bm.logger.Info("account registered", zap.String("job_id", job.ID), zap.String("email", acc.Email), zap.Int("progress", job.ProgressValue()))
		}
	}

	for i := 0; i < workers; i++ { wg.Add(1); go worker() }
	for i := 1; i <= req.Count; i++ {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			job.SetStatus(models.BatchStatusStopped)
			now := time.Now(); job.SetCompletedAt(&now)
			job.AddLog("warning", fmt.Sprintf("Batch stopped at %d/%d accounts", job.ProgressValue(), req.Count))
			bm.logger.Info("batch stopped", zap.String("job_id", job.ID), zap.Int("progress", job.ProgressValue()))
			return
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()

	job.SetStatus(models.BatchStatusCompleted)
	now := time.Now()
	job.SetCompletedAt(&now)
	job.AddLog("info", fmt.Sprintf("Batch completed: %d/%d accounts registered successfully", job.SuccessValue(), req.Count))

	bm.logger.Info("batch completed",
		zap.String("job_id", job.ID),
		zap.Int("success", job.SuccessValue()),
		zap.Int("failed", job.FailedValue()),
	)
}

func (bm *BatchManager) GetJob(id string) (*models.BatchJob, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	job, exists := bm.jobs[id]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	return job, nil
}

func (bm *BatchManager) GetAllJobs() []*models.BatchJob {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	jobs := make([]*models.BatchJob, 0, len(bm.jobs))
	for _, job := range bm.jobs {
		jobs = append(jobs, job)
	}

	return jobs
}

func (bm *BatchManager) StopJob(id string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	job, exists := bm.jobs[id]
	if !exists {
		return fmt.Errorf("job not found: %s", id)
	}

	if job.Status != models.BatchStatusRunning {
		return fmt.Errorf("job is not running: %s", job.Status)
	}

	cancel, exists := bm.cancelFuncs[id]
	if exists {
		cancel()
	}

	job.AddLog("warning", "Stop signal sent by user")
	job.SetStatus(models.BatchStatusStopped)
	now := time.Now()
	job.SetCompletedAt(&now)

	return nil
}

func (bm *BatchManager) StreamLogs(id string) (<-chan models.LogEntry, error) {
	bm.mu.RLock()
	_, exists := bm.jobs[id]
	bm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	out := make(chan models.LogEntry, 100)

	go func() {
		defer close(out)
		lastIdx := 0

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			<-ticker.C

			bm.mu.RLock()
			currentJob := bm.jobs[id]
			bm.mu.RUnlock()

			if currentJob == nil {
				return
			}

			for i := lastIdx; i < len(currentJob.Logs); i++ {
				out <- currentJob.Logs[i]
			}
			lastIdx = len(currentJob.Logs)

			if currentJob.Status != models.BatchStatusRunning {
				return
			}
		}
	}()

	return out, nil
}

func generateJobID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// persistAutoScript writes the current batch state to request_log_tracker so
// the "Auto Script Log" page survives restarts. Called once at start (status=
// running, ts_end=0) and again at each terminal transition.
func (bm *BatchManager) persistAutoScript(job *models.BatchJob, trigger string) {
	if GlobalRequestLogTracker == nil {
		return
	}
	logsJSON := ""
	if len(job.Logs) > 0 {
		if b, err := json.Marshal(job.Logs); err == nil {
			logsJSON = string(b)
		}
	}
	var tsEnd int64
	if job.CompletedAt != nil {
		tsEnd = job.CompletedAt.Unix()
	}
	GlobalRequestLogTracker.RecordAutoScript(AutoScriptRun{
		ID:             job.ID,
		TimestampStart: job.StartedAt.Unix(),
		TimestampEnd:   tsEnd,
		Trigger:        trigger,
		Provider:       job.Provider,
		Attempted:      job.Count,
		Succeeded:      job.SuccessValue(),
		Failed:         job.FailedValue(),
		Status:         string(job.StatusValue()),
		LogsJSON:       logsJSON,
	})
}
