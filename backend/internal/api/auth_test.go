package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
)

func TestHandleLoginAndLoginCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	core.GlobalConfig = &core.Config{AdminKey: "secret", DataDir: dir}
	keyManager = core.NewKeyManager(core.GlobalConfig.GetAPIKeysFile())
	t.Cleanup(func() { keyManager = nil })

	r := gin.New()
	RegisterAuthRoutes(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"key":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", w.Code)
	}

	var body struct {
		APIKey string `json:"apikey"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if body.APIKey == "" || !keyManager.IsValid(body.APIKey) {
		t.Fatalf("login apikey not provisioned in pool: %q", body.APIKey)
	}

	checkReq := httptest.NewRequest(http.MethodGet, "/api/auth/login/check", nil)
	for _, c := range w.Result().Cookies() {
		checkReq.AddCookie(c)
	}
	checkW := httptest.NewRecorder()
	r.ServeHTTP(checkW, checkReq)
	if checkW.Code != http.StatusOK {
		t.Fatalf("login check status = %d, want 200", checkW.Code)
	}
}

func TestAdminMiddlewareAcceptsBearerAndQueryKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core.GlobalConfig = &core.Config{AdminKey: "secret"}
	keyManager = nil
	r := gin.New()
	r.GET("/bearer", AdminMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/query", AdminMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	bearerReq := httptest.NewRequest(http.MethodGet, "/bearer", nil)
	bearerReq.Header.Set("Authorization", "Bearer secret")
	bearerW := httptest.NewRecorder()
	r.ServeHTTP(bearerW, bearerReq)
	if bearerW.Code != http.StatusNoContent {
		t.Fatalf("bearer auth status = %d, want 204", bearerW.Code)
	}

	queryW := httptest.NewRecorder()
	queryReq := httptest.NewRequest(http.MethodGet, "/query?key=secret", nil)
	r.ServeHTTP(queryW, queryReq)
	if queryW.Code != http.StatusNoContent {
		t.Fatalf("query auth status = %d, want 204", queryW.Code)
	}

	// Non-admin key must be rejected by AdminMiddleware (split-auth invariant).
	rejectW := httptest.NewRecorder()
	rejectReq := httptest.NewRequest(http.MethodGet, "/bearer", nil)
	rejectReq.Header.Set("Authorization", "Bearer not-admin")
	r.ServeHTTP(rejectW, rejectReq)
	if rejectW.Code != http.StatusUnauthorized {
		t.Fatalf("non-admin status = %d, want 401", rejectW.Code)
	}
}

func TestEmptyStatsShapes(t *testing.T) {
	stats := emptyStats()
	if stats == nil || stats.ByFeature == nil || stats.ModelStats == nil || stats.Timeline == nil || stats.Models == nil {
		b, _ := json.Marshal(stats)
		t.Fatalf("emptyStats() malformed: %s", string(b))
	}
}
