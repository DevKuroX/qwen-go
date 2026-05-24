package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
)

var keyManager *core.KeyManager
var settingsManager *core.SettingsManager

func InitKeyManager(km *core.KeyManager) {
	keyManager = km
}

func InitSettingsManager(sm *core.SettingsManager) {
	settingsManager = sm
}

func RegisterAdminRoutes(r *gin.Engine, pool *core.AccountPool) {
	admin := r.Group("/api/admin")
	admin.Use(AdminMiddleware())
	
	admin.GET("/system-info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"cpu_cores":           1,
			"ram_total_gb":        7.6,
			"ram_available_gb":    4.5,
			"recommended_threads": 3,
		})
	})

	admin.GET("/status", func(c *gin.Context) {
		engineMode := ""
		if core.GlobalConfig != nil {
			engineMode = core.GlobalConfig.EngineMode
		}
		c.JSON(http.StatusOK, gin.H{
			"accounts":       pool.GetStatus(),
			"browser_engine": gin.H{"pool_size": 0, "queue": 0},
			"engine_mode":    engineMode,
			"version":        "2.0.0-go",
		})
	})
	
	admin.GET("/accounts", func(c *gin.Context) {
		accounts := pool.ListAccounts()
		c.JSON(http.StatusOK, gin.H{
			"total":    len(accounts),
			"accounts": accounts,
		})
	})
	
	admin.GET("/pool-stats", func(c *gin.Context) {
		status := pool.GetStatus()
		c.JSON(http.StatusOK, gin.H{
			"summary": status,
			"accounts": pool.ListAccounts(),
		})
	})
	
	admin.GET("/settings", func(c *gin.Context) {
		if core.GlobalConfig == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "config not loaded"})
			return
		}
		resp := gin.H{
			"admin_key":             core.GlobalConfig.AdminKey,
			"port":                  core.GlobalConfig.Port,
			"engine_mode":           core.GlobalConfig.EngineMode,
			"max_inflight":          core.GlobalConfig.MaxInflight,
			"max_retries":           core.GlobalConfig.MaxRetries,
			"auto_replenish":        core.GlobalConfig.AutoReplenish,
			"replenish_target":      core.GlobalConfig.ReplenishTarget,
			"registration_engine":   core.GlobalConfig.RegistrationEngine,
		}
		if settingsManager != nil {
			s := settingsManager.Get()
			resp["max_inflight_per_account"] = s.MaxInflightPerAcc
			resp["model_aliases"] = s.ModelAliases
			rtkOn, cavOn, level := settingsManager.SaverFlags()
			resp["rtk_enabled"] = rtkOn
			resp["caveman_enabled"] = cavOn
			resp["caveman_level"] = level
			resp["moemail_domain"] = s.MoeMailDomain
			resp["moemail_key"] = s.MoeMailKey
			resp["tempmail_domain"] = s.TempMailDomain
			resp["tempmail_key"] = s.TempMailKey
			resp["proxy_enabled"] = s.ProxyEnabled
			resp["proxy_url"] = s.ProxyURL
			resp["proxy_username"] = s.ProxyUsername
			resp["proxy_password"] = s.ProxyPassword
		}
		c.JSON(http.StatusOK, resp)
	})
	
	admin.PUT("/settings", func(c *gin.Context) {
		if settingsManager == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "settings manager not initialized"})
			return
		}
		var updates map[string]interface{}
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
			return
		}
		if err := settingsManager.Update(updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	
	admin.POST("/proxy-test", func(c *gin.Context) {
		var req struct {
			ProxyURL      string `json:"proxy_url"`
			ProxyUsername string `json:"proxy_username"`
			ProxyPassword string `json:"proxy_password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		result := testProxy(req.ProxyURL, req.ProxyUsername, req.ProxyPassword)
		c.JSON(http.StatusOK, result)
	})
	
	admin.GET("/keys", func(c *gin.Context) {
		keys := []string{}
		if keyManager != nil {
			keys = keyManager.List()
		}
		c.JSON(http.StatusOK, gin.H{"keys": keys})
	})
	
	admin.POST("/keys", func(c *gin.Context) {
		if keyManager == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "key manager not initialized"})
			return
		}
		key, err := keyManager.Generate()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"key": key})
	})
	
	admin.DELETE("/keys/:key", func(c *gin.Context) {
		if keyManager == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "key manager not initialized"})
			return
		}
		key := c.Param("key")
		if keyManager.Revoke(key) {
			c.JSON(http.StatusOK, gin.H{"status": "revoked"})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "key not found"})
		}
	})
	
	admin.GET("/health-metrics", func(c *gin.Context) {
		status := pool.GetStatus()
		c.JSON(http.StatusOK, gin.H{
			"total":          status["total"],
			"valid":          status["valid"],
			"health_pct":     float64(status["valid"].(int)) / float64(max(status["total"].(int), 1)) * 100,
			"overall_status": "operational",
		})
	})
	
	admin.GET("/accounts/stats", func(c *gin.Context) {
		status := pool.GetStatus()
		c.JSON(http.StatusOK, gin.H{
			"valid":         status["valid"],
			"pending":       0,
			"rate_limited":  status["rate_limited"],
			"banned":        status["banned"],
			"invalid":       status["soft_error"].(int) + status["circuit_open"].(int),
		})
	})
}

func testProxy(proxyURL, username, password string) gin.H {
	type testResult struct {
		Ok       bool   `json:"ok"`
		DirectIP string `json:"direct_ip"`
		ProxyIP  string `json:"proxy_ip"`
		Error    string `json:"error,omitempty"`
	}

	directIP := ""
	proxyIP := ""

	directClient := &http.Client{Timeout: 10 * time.Second}
	directResp, err := directClient.Get("https://httpbin.org/ip")
	if err == nil {
		body, _ := io.ReadAll(directResp.Body)
		directResp.Body.Close()
		var ipResp struct {
			Origin string `json:"origin"`
		}
		if json.Unmarshal(body, &ipResp) == nil {
			directIP = ipResp.Origin
		}
	}
	if directIP == "" {
		directIP = "unknown"
	}

	if proxyURL == "" {
		return gin.H{
			"ok":       false,
			"direct_ip": directIP,
			"proxy_ip":  "",
			"error":    "no proxy URL provided",
		}
	}

	proxyParsed, err := url.Parse(proxyURL)
	if err != nil {
		return gin.H{
			"ok":        false,
			"direct_ip": directIP,
			"proxy_ip":  "",
			"error":     fmt.Sprintf("invalid proxy URL: %s", err.Error()),
		}
	}

	if username != "" {
		proxyParsed.User = url.UserPassword(username, password)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyParsed),
	}
	proxyClient := &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}

	proxyResp, err := proxyClient.Get("https://httpbin.org/ip")
	if err != nil {
		return gin.H{
			"ok":        false,
			"direct_ip": directIP,
			"proxy_ip":  "",
			"error":     fmt.Sprintf("proxy connection failed: %s", err.Error()),
		}
	}
	defer proxyResp.Body.Close()

	body, _ := io.ReadAll(proxyResp.Body)
	var ipResp struct {
		Origin string `json:"origin"`
	}
	if json.Unmarshal(body, &ipResp) == nil {
		proxyIP = ipResp.Origin
	}

	if proxyIP == "" {
		return gin.H{
			"ok":        false,
			"direct_ip": directIP,
			"proxy_ip":  "",
			"error":     "could not determine proxy IP",
		}
	}

	ok := proxyIP != directIP
	if !ok {
		return gin.H{
			"ok":        false,
			"direct_ip": directIP,
			"proxy_ip":  proxyIP,
			"error":     "proxy IP matches direct IP — proxy not working",
		}
	}

	return gin.H{
		"ok":        true,
		"direct_ip": directIP,
		"proxy_ip":  proxyIP,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
