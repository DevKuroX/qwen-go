package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

var batchManager *core.BatchManager

func InitBatchHandler(bm *core.BatchManager) {
	batchManager = bm
}

func RegisterBatchRoutes(r *gin.Engine, bm *core.BatchManager) {
	batchManager = bm

	batch := r.Group("/api/admin/batch")
	batch.Use(AdminMiddleware())

	batch.POST("/start", handleBatchStart)
	batch.GET("/:id", handleBatchStatus)
	batch.GET("/:id/logs", handleBatchLogsSSE)
	batch.POST("/:id/stop", handleBatchStop)
	batch.GET("/jobs", handleBatchJobsList)
}

func handleBatchStart(c *gin.Context) {
	var req models.BatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Provider == "" {
		req.Provider = "qwen"
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Threads <= 0 {
		req.Threads = 1
	}
	if req.MailProvider == "" {
		req.MailProvider = "guerrilla"
	}

	jobID, err := batchManager.StartBatch(context.Background(), req)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":    true,
		"job_id": jobID,
		"message": fmt.Sprintf("Batch registration started: %d accounts", req.Count),
	})
}

func handleBatchStatus(c *gin.Context) {
	jobID := c.Param("id")

	job, err := batchManager.GetJob(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, job)
}

func handleBatchLogsSSE(c *gin.Context) {
	jobID := c.Param("id")

	logChan, err := batchManager.StreamLogs(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	for log := range logChan {
		data, err := json.Marshal(log)
		if err != nil {
			continue
		}

		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
	}
}

func handleBatchStop(c *gin.Context) {
	jobID := c.Param("id")

	if err := batchManager.StopJob(jobID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Stop signal sent to batch job",
	})
}

func handleBatchJobsList(c *gin.Context) {
	jobs := batchManager.GetAllJobs()

	c.JSON(http.StatusOK, gin.H{
		"jobs":  jobs,
		"total": len(jobs),
	})
}
