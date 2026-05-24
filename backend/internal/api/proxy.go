package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

var proxyPool *core.ProxyPool

func InitProxyHandler(pp *core.ProxyPool) {
	proxyPool = pp
}

func RegisterProxyRoutes(r *gin.Engine, pp *core.ProxyPool) {
	proxyPool = pp

	proxy := r.Group("/api/admin/proxy")
	proxy.Use(AdminMiddleware())

	proxy.POST("/import", handleProxyImport)
	proxy.GET("", handleProxyList)
	proxy.GET("/config", handleProxyGetConfig)
	proxy.PUT("/config", handleProxyUpdateConfig)
	proxy.PUT("/:id/toggle", handleProxyToggle)
	proxy.POST("/:id/test", handleProxyTest)
	proxy.DELETE("/:id", handleProxyDelete)
	proxy.POST("/test-all", handleProxyTestAll)
	proxy.DELETE("/batch", handleProxyBatchDelete)
	proxy.DELETE("/dead", handleProxyDeleteDead)
	proxy.GET("/counts", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"counts": proxyPool.GetProxyCounts()})
	})
}

func handleProxyList(c *gin.Context) {
	proxies := proxyPool.List()
	c.JSON(http.StatusOK, gin.H{
		"proxies": proxies,
		"total":   len(proxies),
	})
}

func handleProxyImport(c *gin.Context) {
	var req models.ProxyImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if len(req.RawURLs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no proxy urls provided"})
		return
	}

	imported, failed, errors := proxyPool.Import(req.RawURLs)
	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"imported": imported,
		"failed":   failed,
		"errors":   errors,
	})
}

func handleProxyToggle(c *gin.Context) {
	id := c.Param("id")
	var req models.ProxyToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := proxyPool.Toggle(id, req.Enabled); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleProxyTest(c *gin.Context) {
	id := c.Param("id")

	result, err := proxyPool.TestSingle(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":     true,
		"result": result,
	})
}

func handleProxyDelete(c *gin.Context) {
	id := c.Param("id")

	if err := proxyPool.Delete(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func handleProxyTestAll(c *gin.Context) {
	results := proxyPool.TestAll()
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"results": results,
	})
}

func handleProxyDeleteDead(c *gin.Context) {
	deleted := proxyPool.DeleteDead()
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"deleted": deleted,
	})
}

func handleProxyBatchDelete(c *gin.Context) {
	var req models.ProxyBatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	deleted := proxyPool.BatchDelete(req.IDs)
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"deleted": deleted,
	})
}

func handleProxyGetConfig(c *gin.Context) {
	cfg := proxyPool.GetConfig()
	c.JSON(http.StatusOK, gin.H{"config": cfg})
}

func handleProxyUpdateConfig(c *gin.Context) {
	var cfg models.ProxyPoolConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if cfg.AutoTestInterval <= 0 {
		cfg.AutoTestInterval = 5
	}
	if cfg.TestEndpoint == "" {
		cfg.TestEndpoint = "https://google.com"
	}
	if cfg.RotationStrategy == "" {
		cfg.RotationStrategy = "fastest"
	}
	// Legacy default — kept until ApplyToProviders is fully removed.
	if len(cfg.ApplyToProviders) == 0 {
		cfg.ApplyToProviders = []string{"qwen"}
	}
	// If neither ApplyTo toggle was set in the payload, fall back to the
	// historical behavior: proxy was wired only for batch registration.
	if !cfg.ApplyTo.BatchRegistration && !cfg.ApplyTo.ProviderCall {
		cfg.ApplyTo.BatchRegistration = true
	}

	proxyPool.UpdateConfig(cfg)
	c.JSON(http.StatusOK, gin.H{"ok": true, "config": cfg})
}

func GetProxyEnv() (enabled bool, proxyURL, username, password string) {
	cfg := proxyPool.GetConfig()
	if !cfg.Enabled {
		return false, "", "", ""
	}
	// Account-registration engines (rod/python) only get a proxy when the
	// batch_registration switch is on. Lets the operator keep the proxy pool
	// enabled for provider calls without touching the registration path.
	if !cfg.ApplyTo.BatchRegistration {
		return false, "", "", ""
	}

	p := proxyPool.GetBestProxy()
	if p == nil {
		return false, "", "", ""
	}

	proxyURL = formatProxyURL(p)
	return true, proxyURL, p.Username, p.Password
}

func GetProxyContext() context.Context {
	cfg := proxyPool.GetConfig()
	if !cfg.Enabled {
		return context.Background()
	}

	p := proxyPool.GetBestProxy()
	if p == nil && cfg.FallbackDirect {
		return context.Background()
	}

	return context.Background()
}

func formatProxyURL(p *models.Proxy) string {
	var buf strings.Builder
	switch p.Type {
	case "socks5":
		buf.WriteString("socks5://")
	default:
		buf.WriteString("http://")
	}

	if p.Username != "" {
		buf.WriteString(p.Username)
		if p.Password != "" {
			buf.WriteString(":")
			buf.WriteString(p.Password)
		}
		buf.WriteString("@")
	}

	buf.WriteString(p.Host)
	buf.WriteString(":")
	buf.WriteString(fmt.Sprintf("%d", p.Port))

	return buf.String()
}
