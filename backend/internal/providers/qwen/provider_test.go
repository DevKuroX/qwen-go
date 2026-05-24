package qwen

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newHTTPResponse(status int, body string, req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}
}

func newRetryTestProvider(responses []struct{ status int; body string }) (*QwenProvider, *core.AccountPool) {
	pool := core.NewAccountPool()
	pool.Load([]*models.Account{
		{Email: "a@example.com", Token: "token-a", Provider: "qwen", Status: models.StatusValid, Valid: true},
		{Email: "b@example.com", Token: "token-b", Provider: "qwen", Status: models.StatusValid, Valid: true},
	})
	provider := NewQwenProvider(pool)
	idx := 0
	provider.client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := responses[idx]
		if idx < len(responses)-1 {
			idx++
		}
		return newHTTPResponse(resp.status, resp.body, req), nil
	})}
	return provider, pool
}

func TestQwenProviderRetriesWithDifferentAccounts(t *testing.T) {
	provider, pool := newRetryTestProvider([]struct{ status int; body string }{
		{status: 401, body: "unauthorized"},
		{status: 200, body: `{"success":true,"data":{"id":"chat-2"}}`},
		{status: 200, body: "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"},
		{status: 200, body: ""},
	})

	resp, err := provider.ChatCompletion(context.Background(), &models.ChatRequest{Model: "qwen-plus", Messages: []models.ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Fatalf("content = %q, want ok", resp.Choices[0].Message.Content)
	}
	statuses := map[string]models.AccountStatus{}
	for _, acc := range pool.ListAccounts() {
		statuses[acc.Email] = acc.Status
	}
	if got := statuses["a@example.com"]; got != models.StatusSoftError {
		t.Fatalf("first account status = %s, want %s", got, models.StatusSoftError)
	}
}

func TestQwenProviderMarksRateLimitedAccount(t *testing.T) {
	provider, pool := newRetryTestProvider([]struct{ status int; body string }{{status: 429, body: "too many requests"}, {status: 429, body: "too many requests"}})
	_, _ = provider.ChatCompletion(context.Background(), &models.ChatRequest{Model: "qwen-plus", Messages: []models.ChatMessage{{Role: "user", Content: "hi"}}})
	statuses := map[string]models.AccountStatus{}
	for _, acc := range pool.ListAccounts() {
		statuses[acc.Email] = acc.Status
	}
	if got := statuses["a@example.com"]; got != models.StatusRateLimited {
		t.Fatalf("status = %s, want %s", got, models.StatusRateLimited)
	}
}

func TestQwenProviderMarksAuthFailure(t *testing.T) {
	provider, pool := newRetryTestProvider([]struct{ status int; body string }{{status: 401, body: "unauthorized"}, {status: 401, body: "unauthorized"}})
	_, _ = provider.ChatCompletion(context.Background(), &models.ChatRequest{Model: "qwen-plus", Messages: []models.ChatMessage{{Role: "user", Content: "hi"}}})
	statuses := map[string]models.AccountStatus{}
	for _, acc := range pool.ListAccounts() {
		statuses[acc.Email] = acc.Status
	}
	if got := statuses["a@example.com"]; got != models.StatusSoftError {
		t.Fatalf("status = %s, want %s", got, models.StatusSoftError)
	}
}

func TestQwenProviderStopsRetryOnNoAccounts(t *testing.T) {
	provider := NewQwenProvider(core.NewAccountPool())
	_, err := provider.ChatCompletion(context.Background(), &models.ChatRequest{Model: "qwen-plus", Messages: []models.ChatMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("ChatCompletion() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "no available accounts") {
		t.Fatalf("error = %v, want no available accounts", err)
	}
}

func TestQwenProviderMetadataCompatibility(t *testing.T) {
	provider := NewQwenProvider(core.NewAccountPool())
	meta := provider.Metadata()
	if !meta.Capabilities.SupportsChat || !meta.Capabilities.SupportsStream || !meta.Capabilities.SupportsImage {
		t.Fatalf("Metadata() capabilities = %+v", meta.Capabilities)
	}
	if meta.Limits.MaxRetries != qwenMaxRetries {
		t.Fatalf("Metadata() max retries = %d, want %d", meta.Limits.MaxRetries, qwenMaxRetries)
	}
}

func TestQwenProviderTransientClassificationNoOverPenalty(t *testing.T) {
	provider, pool := newRetryTestProvider([]struct{ status int; body string }{{status: 500, body: "temporary timeout"}, {status: 500, body: "temporary timeout"}})
	_, _ = provider.ChatCompletion(context.Background(), &models.ChatRequest{Model: "qwen-plus", Messages: []models.ChatMessage{{Role: "user", Content: "hi"}}})
	if got := pool.ListAccounts()[0].ConsecutiveFailures; got < 1 {
		t.Fatalf("consecutive failures = %d, want >= 1", got)
	}
	if got := pool.ListAccounts()[0].Status; got == models.StatusBanned {
		t.Fatalf("status = %s, do not want banned", got)
	}
}

func TestQwenProviderSuccessAfterSoftFailureClearsState(t *testing.T) {
	pool := core.NewAccountPool()
	pool.Load([]*models.Account{{Email: "a@example.com", Token: "token-a", Provider: "qwen", Status: models.StatusSoftError, Valid: true, ConsecutiveFailures: 2}})
	provider := NewQwenProvider(pool)
	provider.client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/chats/new") {
			return newHTTPResponse(200, `{"success":true,"data":{"id":"chat-1"}}`, req), nil
		}
		if strings.Contains(req.URL.Path, "/chat/completions") {
			return newHTTPResponse(200, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n", req), nil
		}
		return newHTTPResponse(200, "", req), nil
	})}

	_, err := provider.ChatCompletion(context.Background(), &models.ChatRequest{Model: "qwen-plus", Messages: []models.ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	acc := pool.ListAccounts()[0]
	if acc.Status != models.StatusValid || acc.ConsecutiveFailures != 0 {
		t.Fatalf("account after success = %+v", acc)
	}
}
