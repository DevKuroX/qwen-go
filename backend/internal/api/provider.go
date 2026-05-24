package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

var providerPool *core.AccountPool

func InitProviderHandler(pool *core.AccountPool) {
	providerPool = pool
}

func RegisterProviderRoutes(r *gin.Engine, pool *core.AccountPool) {
	providerPool = pool

	providers := r.Group("/api/admin/providers")
	providers.Use(AdminMiddleware())

	providers.GET("", handleProviderList)
	providers.GET("/:name/accounts", handleProviderAccounts)

	providers.GET("/configs", handleProviderConfigList)
	providers.GET("/configs/:name", handleProviderConfigGet)
	providers.PUT("/configs/:name", handleProviderConfigUpdate)
	providers.DELETE("/configs/:name", handleProviderConfigDelete)
}

func handleProviderList(c *gin.Context) {
	providers := []models.ProviderStats{
		{
			Name:   "qwen",
			Type:   "temp_mail",
			Total:  providerPool.CountByProvider("qwen"),
			Live:   providerPool.CountByStatus("qwen", models.StatusValid),
			Error:  providerPool.CountByStatus("qwen", models.StatusSoftError) + providerPool.CountByStatus("qwen", models.StatusCircuitOpen),
			Banned: providerPool.CountByStatus("qwen", models.StatusBanned),
		},
		{
			Name:   "kiro",
			Type:   "oauth",
			Total:  providerPool.CountByProvider("kiro"),
			Live:   providerPool.CountByStatus("kiro", models.StatusValid),
			Error:  providerPool.CountByStatus("kiro", models.StatusSoftError) + providerPool.CountByStatus("kiro", models.StatusCircuitOpen),
			Banned: providerPool.CountByStatus("kiro", models.StatusBanned),
		},
		{
			Name:   "opencode-zen",
			Type:   "session",
			Total:  providerPool.CountByProvider("opencode-zen"),
			Live:   providerPool.CountByStatus("opencode-zen", models.StatusValid),
			Error:  providerPool.CountByStatus("opencode-zen", models.StatusSoftError) + providerPool.CountByStatus("opencode-zen", models.StatusCircuitOpen),
			Banned: providerPool.CountByStatus("opencode-zen", models.StatusBanned),
		},
		{
			Name:   "gemini-web",
			Type:   "cookie",
			Total:  providerPool.CountByProvider("gemini-web"),
			Live:   providerPool.CountByStatus("gemini-web", models.StatusValid),
			Error:  providerPool.CountByStatus("gemini-web", models.StatusSoftError) + providerPool.CountByStatus("gemini-web", models.StatusCircuitOpen),
			Banned: providerPool.CountByStatus("gemini-web", models.StatusBanned),
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"providers": providers,
	})
}

func handleProviderAccounts(c *gin.Context) {
	name := c.Param("name")

	accounts := providerPool.GetAccountsByProvider(name)

	c.JSON(http.StatusOK, gin.H{
		"provider": name,
		"total":    len(accounts),
		"accounts": accounts,
	})
}

func handleProviderConfigList(c *gin.Context) {
	if core.GlobalProviderRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider registry not initialized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"configs": core.GlobalProviderRegistry.List()})
}

func handleProviderConfigGet(c *gin.Context) {
	if core.GlobalProviderRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider registry not initialized"})
		return
	}
	name := c.Param("name")
	cfg := core.GlobalProviderRegistry.Get(name)
	if cfg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider config not found"})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func handleProviderConfigUpdate(c *gin.Context) {
	if core.GlobalProviderRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider registry not initialized"})
		return
	}
	name := c.Param("name")
	var cfg core.ProviderConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := core.GlobalProviderRegistry.Update(name, &cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "config": core.GlobalProviderRegistry.Get(name)})
}

func handleProviderConfigDelete(c *gin.Context) {
	if core.GlobalProviderRegistry == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider registry not initialized"})
		return
	}
	name := c.Param("name")
	if err := core.GlobalProviderRegistry.Delete(name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
