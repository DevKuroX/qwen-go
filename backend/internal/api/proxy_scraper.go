package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

var proxyScraperManager *core.ProxyScraperManager

func InitProxyScraperHandler(psm *core.ProxyScraperManager) {
	proxyScraperManager = psm
}

func RegisterProxyScraperRoutes(r *gin.Engine, psm *core.ProxyScraperManager) {
	proxyScraperManager = psm

	scraper := r.Group("/api/admin/proxy-scraper")
	scraper.Use(AdminMiddleware())

	scraper.POST("/start", handleScraperStart)
	scraper.GET("/staging", handleScraperStaging)
	scraper.POST("/transfer", handleScraperTransfer)
	scraper.GET("/sources", handleScraperSources)
	scraper.PUT("/sources", handleScraperUpdateSources)
	scraper.GET("/:id", handleScraperStatus)
	scraper.GET("/:id/logs", handleScraperLogsSSE)
	scraper.POST("/:id/stop", handleScraperStop)
}

func handleScraperStart(c *gin.Context) {
	var req models.ScrapeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jobID, err := proxyScraperManager.StartScrape(context.Background(), req)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"job_id":  jobID,
		"message": "Proxy scrape started",
	})
}

func handleScraperStatus(c *gin.Context) {
	jobID := c.Param("id")

	job, err := proxyScraperManager.GetJob(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, job)
}

func handleScraperLogsSSE(c *gin.Context) {
	jobID := c.Param("id")

	logChan, err := proxyScraperManager.StreamLogs(jobID)
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

func handleScraperStop(c *gin.Context) {
	jobID := c.Param("id")

	if err := proxyScraperManager.StopJob(jobID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Stop signal sent to scraper job",
	})
}

func handleScraperStaging(c *gin.Context) {
	filter := models.TransferRequest{}
	filter.Type = c.Query("type")
	filter.Country = c.Query("country")
	if ml := c.Query("max_latency"); ml != "" {
		if v, err := strconv.Atoi(ml); err == nil {
			filter.MaxLatency = v
		}
	}
	if ml := c.Query("min_latency"); ml != "" {
		if v, err := strconv.Atoi(ml); err == nil {
			filter.MinLatency = v
		}
	}

	proxies := proxyScraperManager.GetStagingProxies(filter)
	c.JSON(http.StatusOK, gin.H{
		"proxies": proxies,
		"total":   len(proxies),
	})
}

func handleScraperTransfer(c *gin.Context) {
	var req models.TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	imported, err := proxyScraperManager.TransferToProxyPool(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"imported": imported,
		"message":  fmt.Sprintf("Transferred %d proxies to pool", imported),
	})
}

func handleScraperSources(c *gin.Context) {
	sources := proxyScraperManager.GetSources()
	c.JSON(http.StatusOK, gin.H{
		"sources": sources,
		"total":   len(sources),
	})
}

func handleScraperUpdateSources(c *gin.Context) {
	var req struct {
		Sources []*models.ProxySource `json:"sources"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Sources) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no sources provided"})
		return
	}

	proxyScraperManager.UpdateSources(req.Sources)
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": fmt.Sprintf("Updated %d sources", len(req.Sources)),
	})
}
