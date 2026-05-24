package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

type chatTestProvider struct {
	streamChunks []models.StreamChunk
	chatResp     *models.ChatResponse
	chatErr      error
}

func (p *chatTestProvider) Name() string { return "test" }
func (p *chatTestProvider) Prefix() string { return "tt/" }
func (p *chatTestProvider) Type() models.ProviderType { return models.ProviderTypeAPIKey }
func (p *chatTestProvider) Health(ctx context.Context) error { return nil }
func (p *chatTestProvider) ResolveModel(model string) string { return model }
func (p *chatTestProvider) Metadata() models.ProviderMetadata { return models.ProviderMetadata{} }
func (p *chatTestProvider) ImageGeneration(ctx context.Context, req *models.ImageRequest) (*models.ImageResponse, error) {
	return nil, nil
}
func (p *chatTestProvider) VideoGeneration(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
	return nil, nil
}
func (p *chatTestProvider) ChatCompletion(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	return p.chatResp, p.chatErr
}
func (p *chatTestProvider) ChatStream(ctx context.Context, req *models.ChatRequest) (<-chan models.StreamChunk, error) {
	ch := make(chan models.StreamChunk, len(p.streamChunks))
	for _, chunk := range p.streamChunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func setupChatRouter(t *testing.T, provider models.Provider) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	core.GlobalConfig = &core.Config{AdminKey: "secret"}
	km := core.NewKeyManager(filepath.Join(t.TempDir(), "api_keys.json"))
	apiKey, err := km.Generate()
	if err != nil {
		t.Fatalf("seed api key: %v", err)
	}
	keyManager = km
	usageTracker = nil
	pm := core.NewProviderManager()
	pm.Register(provider)
	InitChatHandler(pm)
	r := gin.New()
	RegisterChatRoutes(r, pm)
	return r, apiKey
}

func TestHandleStreamResponseEmitsSSEAndDoneOnce(t *testing.T) {
	provider := &chatTestProvider{streamChunks: []models.StreamChunk{{Content: "hello"}, {Done: true}}}
	r, apiKey := setupChatRouter(t, provider)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions?key="+apiKey, strings.NewReader(`{"model":"tt/demo","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	body := w.Body.String()
	if strings.Count(body, "data: [DONE]") != 1 {
		t.Fatalf("body = %q, want [DONE] once", body)
	}
	if !strings.Contains(body, `"object":"chat.completion.chunk"`) {
		t.Fatalf("body = %q, want SSE chunk payload", body)
	}
}

func TestHandleStreamResponseEscapesErrorPayload(t *testing.T) {
	provider := &chatTestProvider{streamChunks: []models.StreamChunk{{Error: `bad "error"`}}}
	r, apiKey := setupChatRouter(t, provider)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions?key="+apiKey, strings.NewReader(`{"model":"tt/demo","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `bad \"error\"`) {
		t.Fatalf("body = %q, want escaped error json", w.Body.String())
	}
}

func TestHandleNonStreamResponseRegression(t *testing.T) {
	provider := &chatTestProvider{chatResp: &models.ChatResponse{
		ID: "id",
		Object: "chat.completion",
		Model: "tt/demo",
		Choices: []models.ChatChoice{{Index: 0, Message: &models.ChatMessage{Role: "assistant", Content: "ok"}, FinishReason: "stop"}},
		Usage: &models.ChatUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}}
	r, apiKey := setupChatRouter(t, provider)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions?key="+apiKey, strings.NewReader(`{"model":"tt/demo","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp models.ChatResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response error = %v", err)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Fatalf("content = %q, want ok", resp.Choices[0].Message.Content)
	}
}
