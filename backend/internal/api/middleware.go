package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
)

// extractAuth pulls the raw credential out of an incoming request. Order:
// Authorization header → x-api-key header → ?key= query param. Strips a
// leading "Bearer " if present. Returns "" if none provided.
func extractAuth(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		auth = c.GetHeader("x-api-key")
	}
	if auth == "" {
		auth = c.Query("key")
	}
	if strings.HasPrefix(auth, "Bearer ") {
		auth = auth[7:]
	}
	return auth
}

// APIKeyMiddleware authorises end-user API calls (/api/v1/*). Only keys
// present in the keyManager pool pass. AdminKey / JWTs are NOT accepted here
// — admin credentials are for the dashboard surface, not for invoking
// generation endpoints. The dashboard authenticates these routes by sending
// a real pool key minted at login time (stored in the qwenpi_apikey cookie).
func APIKeyMiddleware() gin.HandlerFunc {
	_ = core.GlobalConfig // keep import; admin key intentionally unused here
	return func(c *gin.Context) {
		auth := extractAuth(c)
		if auth == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "missing api key"})
			c.Abort()
			return
		}
		if keyManager == nil || !keyManager.IsValid(auth) {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "invalid api key"})
			c.Abort()
			return
		}
		c.Set("api_key", auth)
		c.Set("is_valid_key", true)
		c.Next()
	}
}

// AdminMiddleware authorises administrative calls (/api/admin/*). It accepts
// either the raw configured admin key OR a JWT signed with the admin key as
// secret. API keys from the user pool are rejected.
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := extractAuth(c)
		if auth == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "missing admin key"})
			c.Abort()
			return
		}
		if !isValidAdminCredential(auth) {
			c.JSON(http.StatusUnauthorized, gin.H{"detail": "admin access required"})
			c.Abort()
			return
		}
		c.Set("api_key", auth)
		c.Set("is_admin", true)
		c.Next()
	}
}
