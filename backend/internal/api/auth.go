package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
)

// jwtTTL is the session lifetime emitted by handleLogin.
const jwtTTL = 30 * 24 * time.Hour

// authStore persists the most recent issued session to data/auth.json so the
// admin can audit logins. Verification itself is signature-only (env-secret
// driven), so this file is informational — not load-bearing for auth.
var authStore = &AuthStore{}

type AuthRecord struct {
	Token     string `json:"token"`
	APIKey    string `json:"apikey"`
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
}

type AuthStore struct {
	mu sync.Mutex
}

func (s *AuthStore) path() string {
	if core.GlobalConfig == nil {
		return ""
	}
	return core.GlobalConfig.GetAuthFile()
}

func (s *AuthStore) Save(rec AuthRecord) error {
	p := s.path()
	if p == "" {
		return errors.New("auth store: no config")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (s *AuthStore) Clear() error {
	p := s.path()
	if p == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// signJWT returns a HS256 JWT with sub + iat + exp claims.
func signJWT(secret []byte, sub string, ttl time.Duration) (string, error) {
	headerJSON := []byte(`{"alg":"HS256","typ":"JWT"}`)
	now := time.Now().Unix()
	claimsJSON, err := json.Marshal(map[string]any{
		"sub": sub,
		"iat": now,
		"exp": now + int64(ttl.Seconds()),
	})
	if err != nil {
		return "", err
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signing := header + "." + payload
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signing + "." + sig, nil
}

// verifyJWT validates HS256 signature and exp claim. Returns claims on success.
func verifyJWT(token string, secret []byte) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed jwt")
	}
	signing := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return nil, errors.New("bad signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, err
	}
	if exp, ok := claims["exp"].(float64); ok && int64(exp) < time.Now().Unix() {
		return nil, errors.New("token expired")
	}
	return claims, nil
}

// loadAuthRecord reads data/auth.json. Returns zero value if absent or
// unreadable — used by handleLogin to reuse an existing dashboard pool key
// across sessions instead of generating a fresh one every time.
func (s *AuthStore) Load() AuthRecord {
	p := s.path()
	if p == "" {
		return AuthRecord{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(p)
	if err != nil {
		return AuthRecord{}
	}
	var rec AuthRecord
	_ = json.Unmarshal(data, &rec)
	return rec
}

// ensureDashboardAPIKey returns a pool-backed API key for the dashboard to
// authenticate `/v1/*` calls. If `data/auth.json` already has a key that is
// still valid in the keyManager pool, reuse it (so re-login doesn't churn
// the pool). Otherwise mint a fresh one.
func ensureDashboardAPIKey() (string, error) {
	if keyManager == nil {
		return "", errors.New("key manager not initialized")
	}
	prev := authStore.Load()
	if prev.APIKey != "" && keyManager.IsValid(prev.APIKey) {
		return prev.APIKey, nil
	}
	return keyManager.Generate()
}

func RegisterAuthRoutes(r *gin.Engine) {
	r.POST("/api/auth/login", handleLogin)
	r.POST("/api/auth/logout", handleLogout)
	r.GET("/api/auth/login/check", handleLoginCheck)
}

func handleLogout(c *gin.Context) {
	c.SetCookie("qwenpi_key", "", -1, "/", "", false, true)
	c.SetCookie("qwenpi_apikey", "", -1, "/", "", false, true)
	_ = authStore.Clear()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func handleLogin(c *gin.Context) {
	var req struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Key is required"})
		return
	}
	if req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Key is required"})
		return
	}
	if core.GlobalConfig == nil || req.Key != core.GlobalConfig.AdminKey {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid admin key"})
		return
	}

	token, err := signJWT([]byte(core.GlobalConfig.AdminKey), "admin", jwtTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "token signing failed"})
		return
	}
	// Dashboard provider calls (/v1/*) MUST use a real pool key — the admin
	// key is for the dashboard surface only. Reuse the prior key if it's
	// still in the pool to avoid churn across re-logins.
	apikey, err := ensureDashboardAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to provision api key"})
		return
	}
	now := time.Now().Unix()
	_ = authStore.Save(AuthRecord{
		Token:     token,
		APIKey:    apikey,
		IssuedAt:  now,
		ExpiresAt: now + int64(jwtTTL.Seconds()),
	})

	// Set cookies for direct-browser flows; also return body so the Next.js
	// frontend route (which proxies and re-sets on its own domain) can read
	// the values.
	c.SetCookie("qwenpi_key", token, int(jwtTTL.Seconds()), "/", "", false, true)
	c.SetCookie("qwenpi_apikey", apikey, int(jwtTTL.Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"token":   token,
		"apikey":  apikey,
	})
}

func handleLoginCheck(c *gin.Context) {
	key, err := c.Cookie("qwenpi_key")
	if err != nil || key == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"authenticated": false})
		return
	}
	if !isValidAdminCredential(key) {
		c.JSON(http.StatusUnauthorized, gin.H{"authenticated": false})
		return
	}
	// Self-heal: if the browser's qwenpi_apikey cookie was wiped or points to
	// a key no longer in the pool (e.g. pool reset across deploys), mint a
	// fresh one while we know the JWT is valid.
	apikeyCookie, _ := c.Cookie("qwenpi_apikey")
	if keyManager == nil || !keyManager.IsValid(apikeyCookie) {
		if apikey, err := ensureDashboardAPIKey(); err == nil {
			c.SetCookie("qwenpi_apikey", apikey, int(jwtTTL.Seconds()), "/", "", false, true)
		}
	}
	c.JSON(http.StatusOK, gin.H{"authenticated": true})
}

// isValidAdminCredential accepts either the raw AdminKey (back-compat for
// CLI/curl) or a JWT signed with the AdminKey as secret. Used by both
// AdminMiddleware and handleLoginCheck.
func isValidAdminCredential(cred string) bool {
	if core.GlobalConfig == nil || core.GlobalConfig.AdminKey == "" {
		return false
	}
	if cred == core.GlobalConfig.AdminKey {
		return true
	}
	if _, err := verifyJWT(cred, []byte(core.GlobalConfig.AdminKey)); err == nil {
		return true
	}
	return false
}
