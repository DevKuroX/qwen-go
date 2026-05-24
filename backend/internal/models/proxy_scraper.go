package models

import "time"

type ScrapeStatus string

const (
	ScrapeStatusRunning   ScrapeStatus = "running"
	ScrapeStatusCompleted ScrapeStatus = "completed"
	ScrapeStatusStopped   ScrapeStatus = "stopped"
	ScrapeStatusFailed    ScrapeStatus = "failed"
)

type ScrapeJob struct {
	ID          string       `json:"id"`
	Status      ScrapeStatus `json:"status"`
	Sources     []string     `json:"sources"`
	Total       int          `json:"total"`
	Found       int          `json:"found"`
	Alive       int          `json:"alive"`
	Imported    int          `json:"imported"`
	Failed      int          `json:"failed"`
	Logs        []LogEntry   `json:"logs"`
	StartedAt   time.Time    `json:"started_at"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
}

func (j *ScrapeJob) AddLog(level, msg string) {
	j.Logs = append(j.Logs, LogEntry{Timestamp: time.Now(), Level: level, Message: msg})
}

type ScrapedProxy struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	Status        string    `json:"status"`
	LatencyMs     int       `json:"latency_ms"`
	Country       string    `json:"country"`
	City          string    `json:"city,omitempty"`
	ISP           string    `json:"isp"`
	Source        string    `json:"source"`
	LastCheckedAt time.Time `json:"last_checked_at,omitempty"`
}

type ProxySourceStats struct {
	TotalFound  int       `json:"total_found"`
	TotalAlive  int       `json:"total_alive"`
	LastScraped time.Time `json:"last_scraped,omitempty"`
	AliveRate   float64   `json:"alive_rate"`
	ScrapeCount int       `json:"scrape_count"`
}

type ProxySource struct {
	URL        string           `json:"url"`
	Enabled    bool             `json:"enabled"`
	SourceType string           `json:"source_type,omitempty"` // "supabase", "geonode", or "" (text scraping)
	APIKey     string           `json:"api_key,omitempty"`
	Stats      ProxySourceStats `json:"stats"`
}

type ScrapeRequest struct {
	Sources []string `json:"sources"`
	Count   int      `json:"count,omitempty"` // optional: cap proxies per source (0 = no cap)
}

type TransferRequest struct {
	IDs        []string `json:"ids,omitempty"`
	Type       string   `json:"type,omitempty"`
	Country    string   `json:"country,omitempty"`
	MaxLatency int      `json:"max_latency,omitempty"`
	MinLatency int      `json:"min_latency,omitempty"`
}
