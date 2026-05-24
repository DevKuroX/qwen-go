package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
)

var usageTracker *core.UsageTracker

func InitUsageTracker(ut *core.UsageTracker) {
	usageTracker = ut
}

func RecordUsage(rec core.UsageRecord) {
	if usageTracker != nil {
		usageTracker.Record(rec)
	}
}

func RegisterUsageRoutes(r *gin.Engine) {
	usage := r.Group("/api/admin/stats")
	usage.Use(AdminMiddleware())
	usage.GET("/usage", func(c *gin.Context) {
		if usageTracker == nil {
			c.JSON(http.StatusOK, emptyStats())
			return
		}

		now := time.Now().Unix()
		start := int64(0)
		end := now

		if s := c.Query("start"); s != "" {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil {
				start = v
			}
		}
		if e := c.Query("end"); e != "" {
			if v, err := strconv.ParseInt(e, 10, 64); err == nil {
				end = v
			}
		}

		modelFilter := c.Query("model")

		stats := usageTracker.Query(start, end, modelFilter)
		c.JSON(http.StatusOK, stats)
	})

	logs := r.Group("/api/admin")
	logs.Use(AdminMiddleware())

	logs.GET("/request-logs", func(c *gin.Context) {
		if core.GlobalRequestLogTracker == nil {
			c.JSON(http.StatusOK, gin.H{"items": []any{}, "total": 0})
			return
		}
		limit, _ := strconv.Atoi(c.Query("limit"))
		offset, _ := strconv.Atoi(c.Query("offset"))
		items, total := core.GlobalRequestLogTracker.ListRequests(limit, offset, c.Query("status"), c.Query("model"))
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
	})

	logs.GET("/request-logs/:id", func(c *gin.Context) {
		if core.GlobalRequestLogTracker == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "request log not found"})
			return
		}
		row, ok := core.GlobalRequestLogTracker.GetRequest(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "request log not found"})
			return
		}
		c.JSON(http.StatusOK, row)
	})

	logs.GET("/auto-script-runs", func(c *gin.Context) {
		if core.GlobalRequestLogTracker == nil {
			c.JSON(http.StatusOK, gin.H{"items": []any{}})
			return
		}
		limit, _ := strconv.Atoi(c.Query("limit"))
		items := core.GlobalRequestLogTracker.ListAutoScripts(limit)
		c.JSON(http.StatusOK, gin.H{"items": items})
	})

	logs.GET("/auto-script-runs/:id", func(c *gin.Context) {
		if core.GlobalRequestLogTracker == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "auto-script run not found"})
			return
		}
		row, ok := core.GlobalRequestLogTracker.GetAutoScript(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "auto-script run not found"})
			return
		}
		// Parse logs_json so frontend doesn't need to double-parse.
		resp := gin.H{
			"id":         row.ID,
			"ts_start":   row.TimestampStart,
			"ts_end":     row.TimestampEnd,
			"trigger":    row.Trigger,
			"provider":   row.Provider,
			"attempted":  row.Attempted,
			"succeeded":  row.Succeeded,
			"failed":     row.Failed,
			"status":     row.Status,
		}
		if row.LogsJSON != "" {
			var logsParsed any
			if err := json.Unmarshal([]byte(row.LogsJSON), &logsParsed); err == nil {
				resp["logs"] = logsParsed
			} else {
				resp["logs_raw"] = row.LogsJSON
			}
		}
		c.JSON(http.StatusOK, resp)
	})
}

func emptyStats() *core.UsageStats {
	return &core.UsageStats{
		ByFeature:  make(map[string]core.FeatureStats),
		ModelStats: make(map[string]core.ModelStats),
		Timeline:   make([]core.TimelineBucket, 0),
		Models:     make([]string, 0),
	}
}
