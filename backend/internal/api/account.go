package api

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

var accountPool *core.AccountPool
var accountDB *core.JSONDatabase

func InitAccountHandler(pool *core.AccountPool, db *core.JSONDatabase) {
	accountPool = pool
	accountDB = db
}

func RegisterAccountRoutes(r *gin.Engine, pool *core.AccountPool, db *core.JSONDatabase) {
	accountPool = pool
	accountDB = db

	accounts := r.Group("/api/admin/accounts")
	accounts.Use(AdminMiddleware())

	accounts.POST("", handleAccountAdd)
	accounts.POST("/batch", handleAccountBatch)
	accounts.POST("/batch-delete", handleAccountBatchDelete)
	accounts.POST("/batch-verify", handleAccountBatchVerify)
	accounts.GET("/search", handleAccountSearch)
	accounts.DELETE("/:email", handleAccountDelete)
	accounts.POST("/:email/verify", handleAccountVerify)
	accounts.POST("/:email/refresh", handleAccountRefresh)
}

// handleAccountRefresh exposes the provider's AuthProvider.RefreshAuth hook
// over the admin API. Today this is the path the dashboard's "Refresh
// Cookies" button uses for gemini-web — the geminiweb provider rotates
// __Secure-1PSIDTS via accounts.google.com and re-scrapes SNlM0e, then we
// persist the new cookie back to the account row.
func handleAccountRefresh(c *gin.Context) {
	email := c.Param("email")
	var acc *models.Account
	for _, a := range accountPool.ListAccounts() {
		if a.Email == email {
			acc = a
			break
		}
	}
	if acc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}
	if providerManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provider manager not initialized"})
		return
	}
	provider := providerManager.GetByName(acc.Provider)
	if provider == nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": fmt.Sprintf("provider %q not registered", acc.Provider)})
		return
	}
	authProvider, ok := provider.(models.AuthProvider)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": fmt.Sprintf("provider %q does not support refresh", acc.Provider)})
		return
	}
	if err := authProvider.RefreshAuth(acc); err != nil {
		accountPool.MarkError(acc.Email, "auth", err.Error())
		persistAccountChanges()
		c.JSON(http.StatusOK, gin.H{"ok": false, "email": email, "error": err.Error()})
		return
	}
	accountPool.MarkSuccess(acc)
	persistAccountChanges()
	c.JSON(http.StatusOK, gin.H{"ok": true, "email": email, "provider": acc.Provider})
}

func handleAccountDelete(c *gin.Context) {
	email := c.Param("email")

	if err := accountPool.SoftDeleteAccount(email); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	persistAccountChanges()
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "Account deleted successfully",
	})
}

func handleAccountAdd(c *gin.Context) {
	var body struct {
		Provider     string `json:"provider"`
		Email        string `json:"email"`
		Password     string `json:"password"`
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}
	provider := strings.ToLower(strings.TrimSpace(body.Provider))
	if provider == "" {
		provider = "qwen"
	}
	// opencode-zen sessions are ephemeral UUIDs — mint one if the caller
	// didn't supply credentials so the dashboard "Add Account" button can
	// be a single click.
	if provider == "opencode-zen" && body.Token == "" && body.Password == "" {
		body.Token = "ses_" + randomHex(32)
		if body.Email == "" {
			body.Email = body.Token
		}
	}
	if body.Token == "" && body.Password == "" {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "Token or password is required"})
		return
	}
	email := body.Email
	if email == "" {
		email = fmt.Sprintf("manual_%d@%s", time.Now().Unix(), provider)
	}
	acc := newAccount(provider, email, body.Password, body.Token, body.RefreshToken)
	accountPool.AddAccount(acc)
	persistAccountChanges()
	c.JSON(http.StatusOK, gin.H{"ok": true, "email": acc.Email, "provider": acc.Provider})
}

// randomHex returns 2*n lowercase hex characters from crypto/rand.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := cryptorand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func handleAccountBatch(c *gin.Context) {
	// Accept three shapes (mirrors python ref): accounts[] / tokens (string|[]) / lines.
	body := struct {
		Accounts json.RawMessage `json:"accounts"`
		Tokens   json.RawMessage `json:"tokens"`
		Lines    string          `json:"lines"`
	}{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	imported := 0
	errs := make([]string, 0)

	switch {
	case len(body.Accounts) > 0:
		// May be a string-wrapped JSON array, a normal array, or singly-nested.
		raw := body.Accounts
		var asString string
		if err := json.Unmarshal(raw, &asString); err == nil {
			raw = json.RawMessage(asString)
		}
		var arr []map[string]interface{}
		if err := json.Unmarshal(raw, &arr); err != nil {
			// Try one level of nested array (matches python auto-flatten).
			var nested [][]map[string]interface{}
			if err2 := json.Unmarshal(raw, &nested); err2 == nil && len(nested) == 1 {
				arr = nested[0]
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "accounts must be an array"})
				return
			}
		}
		for i, item := range arr {
			token, _ := item["token"].(string)
			password, _ := item["password"].(string)
			email, _ := item["email"].(string)
			if email == "" {
				email = fmt.Sprintf("batch_%d_%d@qwen", time.Now().Unix(), i)
			}
			if token == "" && password == "" {
				errs = append(errs, fmt.Sprintf("Item %d (%s): no token or password", i, email))
				continue
			}
			accountPool.AddAccount(newQwenAccount(email, password, token))
			imported++
		}

	case len(body.Tokens) > 0:
		var list []string
		if err := json.Unmarshal(body.Tokens, &list); err != nil {
			var s string
			if err2 := json.Unmarshal(body.Tokens, &s); err2 != nil {
				c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "tokens must be array or newline string"})
				return
			}
			for _, line := range strings.Split(s, "\n") {
				if t := strings.TrimSpace(line); t != "" {
					list = append(list, t)
				}
			}
		}
		for i, token := range list {
			if len(token) < 10 {
				errs = append(errs, fmt.Sprintf("Line %d: token too short", i+1))
				continue
			}
			email := fmt.Sprintf("token_%d_%d@qwen", time.Now().Unix(), i)
			accountPool.AddAccount(newQwenAccount(email, "", token))
			imported++
		}

	case body.Lines != "":
		for i, raw := range strings.Split(body.Lines, "\n") {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 3)
			var email, password, token string
			switch len(parts) {
			case 3:
				email, password, token = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
			case 2:
				first, second := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
				if len(second) > 50 {
					email, password, token = first, "", second
				} else {
					email, password, token = first, second, ""
				}
			case 1:
				token = strings.TrimSpace(parts[0])
				email = fmt.Sprintf("line_%d_%d@qwen", time.Now().Unix(), i)
			default:
				errs = append(errs, fmt.Sprintf("Line %d: cannot parse", i+1))
				continue
			}
			if token == "" && password == "" {
				errs = append(errs, fmt.Sprintf("Line %d: no token or password", i+1))
				continue
			}
			if email == "" {
				email = fmt.Sprintf("line_%d_%d@qwen", time.Now().Unix(), i)
			}
			accountPool.AddAccount(newQwenAccount(email, password, token))
			imported++
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "Please provide one of: accounts, tokens, or lines"})
		return
	}

	if imported > 0 {
		persistAccountChanges()
	}
	if len(errs) > 20 {
		errs = errs[:20]
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":            imported > 0,
		"imported":      imported,
		"errors":        errs,
		"total_in_pool": len(accountPool.ListAccounts()),
	})
}

func handleAccountBatchDelete(c *gin.Context) {
	var body struct {
		Emails []string `json:"emails"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Emails) == 0 {
		c.JSON(http.StatusOK, gin.H{"ok": false, "deleted": 0, "error": "emails must be a non-empty array"})
		return
	}
	deleted := 0
	for _, email := range body.Emails {
		if err := accountPool.SoftDeleteAccount(email); err == nil {
			deleted++
		}
	}
	if deleted > 0 {
		persistAccountChanges()
	}
	c.JSON(http.StatusOK, gin.H{"ok": deleted > 0, "deleted": deleted, "total": len(body.Emails)})
}

func handleAccountBatchVerify(c *gin.Context) {
	var body struct {
		Emails []string `json:"emails"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Emails) == 0 {
		c.JSON(http.StatusOK, gin.H{"ok": false, "verified": 0, "error": "emails must be a non-empty array"})
		return
	}
	verified := 0
	all := accountPool.ListAccounts()
	byEmail := make(map[string]*models.Account, len(all))
	for _, a := range all {
		byEmail[a.Email] = a
	}
	for _, email := range body.Emails {
		acc, ok := byEmail[email]
		if !ok {
			continue
		}
		valid := verifyQwenToken(c.Request.Context(), acc.Token)
		if valid {
			accountPool.MarkSuccess(acc)
		} else {
			accountPool.MarkError(acc.Email, "auth", "Batch verify failed")
		}
		verified++
	}
	persistAccountChanges()
	c.JSON(http.StatusOK, gin.H{"ok": true, "verified": verified, "total": len(body.Emails)})
}

func handleAccountVerify(c *gin.Context) {
	email := c.Param("email")
	var found *models.Account
	for _, a := range accountPool.ListAccounts() {
		if a.Email == email {
			found = a
			break
		}
	}
	if found == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}
	valid := verifyQwenToken(c.Request.Context(), found.Token)
	if valid {
		accountPool.MarkSuccess(found)
	} else {
		accountPool.MarkError(found.Email, "auth", "Token verification failed")
	}
	persistAccountChanges()
	c.JSON(http.StatusOK, gin.H{"valid": valid, "email": email, "status": found.Status})
}

func handleAccountSearch(c *gin.Context) {
	q := strings.TrimSpace(strings.ToLower(c.Query("q")))
	all := accountPool.ListAccounts()
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"accounts": all})
		return
	}
	filtered := make([]*models.Account, 0, len(all))
	for _, a := range all {
		if strings.Contains(strings.ToLower(a.Email), q) ||
			strings.Contains(strings.ToLower(string(a.Status)), q) ||
			strings.Contains(strings.ToLower(a.StatusCode), q) {
			filtered = append(filtered, a)
		}
	}
	c.JSON(http.StatusOK, gin.H{"accounts": filtered})
}

// newQwenAccount is a back-compat shim — qwen-only callers (batch import).
func newQwenAccount(email, password, token string) *models.Account {
	return newAccount("qwen", email, password, token, "")
}

// newAccount builds a fresh Account for any provider. Status=Valid; verify will
// downgrade it if creds are bad.
func newAccount(provider, email, password, token, refreshToken string) *models.Account {
	if provider == "" {
		provider = "qwen"
	}
	return &models.Account{
		Email:        email,
		Password:     password,
		Token:        token,
		RefreshToken: refreshToken,
		Provider:     provider,
		Status:       models.StatusValid,
		Valid:        true,
		CreatedAt:    time.Now(),
	}
}

// persistAccountChanges syncs the in-memory pool to disk. The snapshot loop
// already does this every 60s, but writes here ensure the dashboard sees the
// effect of admin actions immediately after a refresh.
func persistAccountChanges() {
	if accountDB == nil {
		return
	}
	accountDB.Set(accountPool.ListAccounts())
	_ = accountDB.Save()
}

// verifyQwenToken hits chat.qwen.ai /api/v1/auths/ as a fast liveness check.
// Mirrors the python `QwenClient.verify_token` logic: 200 + role=user → live.
// On WAF (HTML or "aliyun_waf") we lean optimistic — the token itself is
// usually fine; the direct path is just blocked.
func verifyQwenToken(ctx context.Context, token string) bool {
	if token == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chat.qwen.ai/api/v1/auths/", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://chat.qwen.ai/")
	req.Header.Set("Origin", "https://chat.qwen.ai")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return false
	}
	var parsed struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		return parsed.Role == "user"
	}
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "aliyun_waf") || strings.Contains(lower, "<!doctype") {
		return true
	}
	return false
}
