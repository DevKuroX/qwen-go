package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// History items are opaque JSON objects keyed by the frontend; the only
// invariant the backend cares about is a stable `id` string for delete-by-id.
type historyItem map[string]interface{}

type historyStore struct {
	mu   sync.Mutex
	path string
}

func (s *historyStore) load() []historyItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *historyStore) loadLocked() []historyItem {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return []historyItem{}
	}
	var items []historyItem
	if err := json.Unmarshal(data, &items); err != nil {
		return []historyItem{}
	}
	return items
}

func (s *historyStore) save(items []historyItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(items)
}

func (s *historyStore) saveLocked(items []historyItem) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	buf, err := json.Marshal(items)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// prependCap inserts `incoming` at the head (newest-first) and trims to maxItems.
func (s *historyStore) prependCap(incoming []historyItem, maxItems int) ([]historyItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.loadLocked()
	merged := make([]historyItem, 0, len(incoming)+len(current))
	merged = append(merged, incoming...)
	merged = append(merged, current...)
	if maxItems > 0 && len(merged) > maxItems {
		merged = merged[:maxItems]
	}
	if err := s.saveLocked(merged); err != nil {
		return nil, err
	}
	return merged, nil
}

func (s *historyStore) deleteByID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.loadLocked()
	out := make([]historyItem, 0, len(current))
	for _, it := range current {
		if v, ok := it["id"].(string); ok && v == id {
			continue
		}
		out = append(out, it)
	}
	return s.saveLocked(out)
}

var (
	imageHistory *historyStore
	videoHistory *historyStore
)

func InitHistoryHandler(imagesPath, videosPath string) {
	imageHistory = &historyStore{path: imagesPath}
	videoHistory = &historyStore{path: videosPath}
}

func RegisterHistoryRoutes(r *gin.Engine) {
	g := r.Group("/api/admin")
	g.Use(AdminMiddleware())

	g.GET("/history/images", handleHistoryList(&imageHistory, "images"))
	g.POST("/history/images", handleHistoryPost(&imageHistory, "images", 100))
	g.DELETE("/history/images", handleHistoryClear(&imageHistory))
	g.DELETE("/history/images/:id", handleHistoryDelete(&imageHistory))

	g.GET("/history/videos", handleHistoryList(&videoHistory, "videos"))
	g.POST("/history/videos", handleHistoryPost(&videoHistory, "videos", 50))
	g.DELETE("/history/videos", handleHistoryClear(&videoHistory))
	g.DELETE("/history/videos/:id", handleHistoryDelete(&videoHistory))

	// Stubs to silence the dashboard layout EventSource retry loop and the
	// register page's log fetch. The python ref served real data; the go
	// backend doesn't have the same runtime event bus yet. Returning a valid
	// SSE keep-alive stream and an empty logs array keeps the UI quiet.
	g.GET("/events", handleAdminEventsStub)
	g.GET("/logs", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"logs": []interface{}{}})
	})
}

func handleHistoryList(store **historyStore, key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if *store == nil {
			c.JSON(http.StatusOK, gin.H{key: []interface{}{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{key: (*store).load()})
	}
}

func handleHistoryPost(store **historyStore, key string, defaultCap int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if *store == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history not initialised"})
			return
		}
		var body struct {
			Images   []historyItem `json:"images"`
			Videos   []historyItem `json:"videos"`
			MaxItems int           `json:"max_items"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		var incoming []historyItem
		switch key {
		case "images":
			incoming = body.Images
		case "videos":
			incoming = body.Videos
		}
		if incoming == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": key + " must be an array"})
			return
		}
		max := body.MaxItems
		if max <= 0 {
			max = defaultCap
		}
		merged, err := (*store).prependCap(incoming, max)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "count": len(merged)})
	}
}

func handleHistoryDelete(store **historyStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if *store == nil {
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
		id := c.Param("id")
		if err := (*store).deleteByID(id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func handleHistoryClear(store **historyStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if *store == nil {
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
		if err := (*store).save([]historyItem{}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// handleAdminEventsStub returns a minimal SSE stream so the dashboard's
// EventSource (frontend/app/(admin)/dashboard/layout.tsx) stays connected
// instead of looping with onerror→reconnect every 5s. We emit periodic
// `: keepalive` comments; real event publishing can be wired later when the
// go pool grows an event bus.
func handleAdminEventsStub(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.Status(http.StatusOK)
		return
	}

	// Initial keep-alive so the browser flips readyState to OPEN.
	_, _ = c.Writer.Write([]byte(": connected\n\n"))
	flusher.Flush()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := c.Writer.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
