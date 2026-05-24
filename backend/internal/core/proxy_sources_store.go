package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

const (
	autoDisableThreshold    = 0.05 // 5%
	autoDisableMinScrapes   = 3
)

type ProxySourceStore struct {
	path string
	mu   sync.RWMutex
	data []*models.ProxySource
}

func NewProxySourceStore(path string) *ProxySourceStore {
	return &ProxySourceStore{path: path}
}

func (s *ProxySourceStore) Load(defaults []*models.ProxySource) []*models.ProxySource {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if err != nil {
		// fresh install — seed with defaults
		s.data = cloneSources(defaults)
		return cloneSources(s.data)
	}

	var saved []*models.ProxySource
	if err := json.Unmarshal(raw, &saved); err != nil {
		s.data = cloneSources(defaults)
		return cloneSources(s.data)
	}

	// merge: keep persisted stats for known URLs, add any new defaults not yet seen
	byURL := make(map[string]*models.ProxySource, len(saved))
	for _, src := range saved {
		byURL[src.URL] = src
	}
	merged := make([]*models.ProxySource, 0, len(defaults)+len(saved))
	seen := make(map[string]bool, len(defaults))
	for _, def := range defaults {
		if existing, ok := byURL[def.URL]; ok {
			// keep persisted enabled state + stats, refresh metadata from defaults
			existing.SourceType = def.SourceType
			if existing.APIKey == "" {
				existing.APIKey = def.APIKey
			}
			merged = append(merged, existing)
		} else {
			merged = append(merged, cloneSource(def))
		}
		seen[def.URL] = true
	}
	for _, src := range saved {
		if !seen[src.URL] {
			merged = append(merged, src)
		}
	}

	s.data = merged
	return cloneSources(s.data)
}

func (s *ProxySourceStore) Save(sources []*models.ProxySource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = cloneSources(sources)

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o644)
}

func cloneSource(src *models.ProxySource) *models.ProxySource {
	c := *src
	return &c
}

func cloneSources(src []*models.ProxySource) []*models.ProxySource {
	out := make([]*models.ProxySource, len(src))
	for i, s := range src {
		out[i] = cloneSource(s)
	}
	return out
}

// updateStats merges this run's outcome into the source's lifetime counters.
// If alive rate stays below the threshold for at least N runs, auto-disable.
func updateStats(src *models.ProxySource, found, alive int) {
	src.Stats.TotalFound += found
	src.Stats.TotalAlive += alive
	src.Stats.ScrapeCount++
	src.Stats.LastScraped = time.Now()
	if src.Stats.TotalFound > 0 {
		src.Stats.AliveRate = float64(src.Stats.TotalAlive) / float64(src.Stats.TotalFound)
	}
	if src.Stats.ScrapeCount >= autoDisableMinScrapes &&
		src.Stats.AliveRate < autoDisableThreshold {
		src.Enabled = false
	}
}
