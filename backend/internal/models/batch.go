package models

import (
	"sync"
	"time"
)

type BatchJobStatus string

const (
	BatchStatusRunning   BatchJobStatus = "running"
	BatchStatusCompleted BatchJobStatus = "completed"
	BatchStatusStopped   BatchJobStatus = "stopped"
	BatchStatusFailed    BatchJobStatus = "failed"
)

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

type BatchJob struct {
	ID          string         `json:"id"`
	Provider    string         `json:"provider"`
	Count       int            `json:"count"`
	Progress    int            `json:"progress"`
	Status      BatchJobStatus `json:"status"`
	Logs        []LogEntry     `json:"logs"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Success     int            `json:"success"`
	Failed      int            `json:"failed"`
	mu          sync.Mutex     `json:"-"`
}

type BatchRequest struct {
	Provider     string `json:"provider"`
	Count        int    `json:"count"`
	Threads      int    `json:"threads"`
	MailProvider string `json:"mail_provider"`
}

type ProviderStats struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Total  int    `json:"total"`
	Live   int    `json:"live"`
	Error  int    `json:"error"`
	Banned int    `json:"banned"`
}

func (j *BatchJob) AddLog(level, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}
	j.Logs = append(j.Logs, entry)
}

func (j *BatchJob) Snapshot() BatchJob {
	j.mu.Lock()
	defer j.mu.Unlock()

	copyLogs := make([]LogEntry, len(j.Logs))
	copy(copyLogs, j.Logs)
	return BatchJob{
		ID:          j.ID,
		Provider:    j.Provider,
		Count:       j.Count,
		Progress:    j.Progress,
		Status:      j.Status,
		Logs:        copyLogs,
		StartedAt:   j.StartedAt,
		CompletedAt: j.CompletedAt,
		Success:     j.Success,
		Failed:      j.Failed,
	}
}

func (j *BatchJob) SetStatus(status BatchJobStatus) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = status
}

func (j *BatchJob) SetCompletedAt(t *time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.CompletedAt = t
}

func (j *BatchJob) IncProgress() { j.mu.Lock(); j.Progress++; j.mu.Unlock() }
func (j *BatchJob) IncSuccess() { j.mu.Lock(); j.Success++; j.mu.Unlock() }
func (j *BatchJob) IncFailed() { j.mu.Lock(); j.Failed++; j.mu.Unlock() }
func (j *BatchJob) StatusValue() BatchJobStatus { j.mu.Lock(); defer j.mu.Unlock(); return j.Status }
func (j *BatchJob) ProgressValue() int { j.mu.Lock(); defer j.mu.Unlock(); return j.Progress }
func (j *BatchJob) SuccessValue() int { j.mu.Lock(); defer j.mu.Unlock(); return j.Success }
func (j *BatchJob) FailedValue() int { j.mu.Lock(); defer j.mu.Unlock(); return j.Failed }
