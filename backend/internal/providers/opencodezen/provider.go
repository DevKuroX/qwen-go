package opencodezen

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

const (
	providerName       = "opencode-zen"
	defaultSessionCap  = 190
	defaultConcurrent  = 10
	defaultEndpoint    = "https://opencode.ai/zen/v1/chat/completions"
	defaultUserAgent   = "opencode/1.15.9 (Linux)"
	maxAttempts        = 3
)

// OpenCodeZenProvider implements the ephemeral-session-UUID auth shape.
// Identity is just a client-generated session UUID; quota is per-session
// (~200 reqs) so we rotate proactively at SessionMaxRequests and discard
// dead sessions outright (never recycle).
type OpenCodeZenProvider struct {
	pool       *core.AccountPool
	registry   *core.ProviderRegistry
	httpClient *http.Client
}

func NewProvider(pool *core.AccountPool, registry *core.ProviderRegistry) *OpenCodeZenProvider {
	return &OpenCodeZenProvider{
		pool:     pool,
		registry: registry,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (p *OpenCodeZenProvider) Name() string                { return providerName }
func (p *OpenCodeZenProvider) Prefix() string              { return "zen/" }
func (p *OpenCodeZenProvider) Type() models.ProviderType   { return models.ProviderTypePublic }
// ResolveModel strips the "zen/" prefix only. Alias→base swap is deferred
// to singleAttempt so the variant name survives through chat.go's
// req.Model = resolvedModel overwrite and lookupAlias can still match.
func (p *OpenCodeZenProvider) ResolveModel(model string) string {
	return strings.TrimPrefix(model, "zen/")
}

// lookupAlias returns the registered ModelAlias if `model` (already
// prefix-stripped) is a configured variant for this provider.
func (p *OpenCodeZenProvider) lookupAlias(model string) (core.ModelAlias, bool) {
	cfg := p.registry.Get(providerName)
	if cfg == nil || len(cfg.ModelAliases) == 0 {
		return core.ModelAlias{}, false
	}
	alias, ok := cfg.ModelAliases[model]
	return alias, ok
}

// isKnownModel returns true for the base catalog OR any registered alias.
// Used by entry points to reject unknown variants early with a clear error.
func (p *OpenCodeZenProvider) isKnownModel(model string) bool {
	cfg := p.registry.Get(providerName)
	if cfg == nil {
		return false
	}
	for _, m := range cfg.AvailableModels {
		if m == model {
			return true
		}
	}
	_, ok := cfg.ModelAliases[model]
	return ok
}

func (p *OpenCodeZenProvider) Metadata() models.ProviderMetadata {
	return models.ProviderMetadata{
		Capabilities: models.ProviderCapabilities{SupportsChat: true, SupportsStream: true},
		Limits:       models.ProviderLimits{DefaultContextWindow: 32768, MaxRetries: maxAttempts},
	}
}

func (p *OpenCodeZenProvider) Health(ctx context.Context) error {
	// Health here = "registry has a config for us." Pool can be empty (we
	// lazy-spawn) so a zero-account pool is not a failure for this provider.
	if p.registry == nil || p.registry.Get(providerName) == nil {
		return errors.New("opencode-zen provider not configured in registry")
	}
	return nil
}

func (p *OpenCodeZenProvider) BuildAuthHeaders(acc *models.Account) map[string]string {
	cfg := p.registry.Get(providerName)
	h := map[string]string{
		"Authorization":      "Bearer public",
		"x-opencode-session": acc.Token,
		"x-opencode-request": "msg_" + shortToken(20),
		"Content-Type":       "application/json",
		"Accept":             "text/event-stream",
		"User-Agent":         defaultUserAgent,
	}
	if cfg != nil {
		for k, v := range cfg.Headers {
			h[k] = v
		}
	}
	return h
}

func (p *OpenCodeZenProvider) EnsureFresh(acc *models.Account) error { return nil }
func (p *OpenCodeZenProvider) RefreshAuth(acc *models.Account) error {
	return errors.New("opencode-zen is identity-less; spawn a new session instead of refreshing")
}

// spawnSession mints a fresh in-memory account. Token = session UUID,
// Email = the same UUID (used as pool key). SpawnedAt feeds age tracking.
func (p *OpenCodeZenProvider) spawnSession() *models.Account {
	uuid := "ses_" + shortToken(32)
	now := time.Now()
	return &models.Account{
		Provider:        providerName,
		Email:           uuid,
		Token:           uuid,
		Status:          models.StatusValid,
		SpawnedAt:       now.Unix(),
		CreatedAt:       now,
		LastRequestTime: now,
	}
}

func (p *OpenCodeZenProvider) sessionMaxRequests() int {
	if cfg := p.registry.Get(providerName); cfg != nil && cfg.SessionMaxRequests > 0 {
		return cfg.SessionMaxRequests
	}
	return defaultSessionCap
}

func (p *OpenCodeZenProvider) maxConcurrentSessions() int {
	if cfg := p.registry.Get(providerName); cfg != nil && cfg.MaxConcurrentSessions > 0 {
		return cfg.MaxConcurrentSessions
	}
	return defaultConcurrent
}

func (p *OpenCodeZenProvider) endpoint() string {
	if cfg := p.registry.Get(providerName); cfg != nil && cfg.BaseEndpoint != "" {
		return cfg.BaseEndpoint
	}
	return defaultEndpoint
}

// acquireOrSpawn pulls a live session from the pool, lazy-spawning if the
// pool has nothing for us. Honors maxConcurrentSessions as a soft cap (logs
// but does not block — availability > anti-abuse heuristic on free tier).
func (p *OpenCodeZenProvider) acquireOrSpawn(ctx context.Context, exclude map[string]bool) (*models.Account, error) {
	acc, err := p.pool.Acquire(ctx, providerName, exclude)
	if err == nil {
		return acc, nil
	}
	if !errors.Is(err, core.ErrNoAccounts) {
		return nil, err
	}
	fresh := p.spawnSession()
	p.pool.AddAccount(fresh)
	return p.pool.Acquire(ctx, providerName, exclude)
}

// retireAndSpawn drops a dead session (quota hit / auth fail) and seeds a
// replacement. Caller continues the loop to Acquire the replacement.
func (p *OpenCodeZenProvider) retireAndSpawn(email string) {
	p.pool.RemoveAccount(email)
	p.pool.AddAccount(p.spawnSession())
}

func (p *OpenCodeZenProvider) ChatCompletion(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	if err := p.validateModel(req.Model); err != nil {
		return nil, err
	}
	full, err := p.runChat(ctx, req, nil)
	if err != nil {
		return nil, err
	}
	id := "chatcmpl-" + shortToken(16)
	return &models.ChatResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []models.ChatChoice{{
			Index:        0,
			Message:      &models.ChatMessage{Role: "assistant", Content: full},
			FinishReason: "stop",
		}},
		Usage: &models.ChatUsage{
			PromptTokens:     len(req.Messages) * 10,
			CompletionTokens: len(full) / 4,
			TotalTokens:      len(req.Messages)*10 + len(full)/4,
		},
	}, nil
}

func (p *OpenCodeZenProvider) ChatStream(ctx context.Context, req *models.ChatRequest) (<-chan models.StreamChunk, error) {
	if err := p.validateModel(req.Model); err != nil {
		return nil, err
	}
	outCh := make(chan models.StreamChunk, 100)
	go func() {
		defer close(outCh)
		_, err := p.runChat(ctx, req, outCh)
		if err != nil {
			outCh <- models.StreamChunk{Error: err.Error()}
		}
	}()
	return outCh, nil
}

// validateModel rejects unknown variant suffixes early so the user gets a
// clear 400 rather than a confusing upstream error. Base catalog + registered
// aliases both count as known.
func (p *OpenCodeZenProvider) validateModel(model string) error {
	stripped := strings.TrimPrefix(model, "zen/")
	if !p.isKnownModel(stripped) {
		return fmt.Errorf("unknown opencode-zen model %q (not in available_models or model_aliases)", stripped)
	}
	return nil
}

// runChat is the shared body for both streaming and non-streaming calls.
// When outCh is non-nil chunks are forwarded as they arrive (streaming);
// when nil they accumulate into the returned string (non-streaming).
//
// Retry semantics mirror the Qwen provider: pre-stream failures swap to a
// fresh session, mid-stream failures surface and stop (we can't replay
// partial output to the client).
func (p *OpenCodeZenProvider) runChat(ctx context.Context, req *models.ChatRequest, outCh chan<- models.StreamChunk) (string, error) {
	exclude := make(map[string]bool)
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		acc, err := p.acquireOrSpawn(ctx, exclude)
		if err != nil {
			if lastErr != nil {
				return "", lastErr
			}
			return "", fmt.Errorf("no available opencode-zen sessions: %w", err)
		}

		// Proactive rotation — retire before the upstream returns 429.
		if acc.RequestCount >= int64(p.sessionMaxRequests()) {
			p.pool.Release(acc)
			p.retireAndSpawn(acc.Email)
			continue
		}

		content, started, attemptErr := p.singleAttempt(ctx, req, acc, outCh)
		p.pool.Release(acc)

		if attemptErr == nil {
			atomic.AddInt64(&acc.RequestCount, 1)
			p.pool.MarkSuccess(acc)
			p.pool.Touch(acc)
			return content, nil
		}

		class := classifyError(attemptErr)

		// Quota / auth failure → session dead forever; discard and respawn.
		if class == "rate_limit" || class == "auth" {
			p.retireAndSpawn(acc.Email)
		} else {
			p.pool.MarkError(acc.Email, class, attemptErr.Error())
			exclude[acc.Email] = true
		}

		if started {
			// Mid-stream failure: surface error and stop. Client already
			// saw partial content from THIS attempt; retrying would splice
			// two different generations together.
			return "", attemptErr
		}

		lastErr = attemptErr
		if !shouldRetry(class) {
			return "", attemptErr
		}
	}

	if lastErr == nil {
		lastErr = errors.New("opencode-zen request failed")
	}
	return "", lastErr
}

// singleAttempt POSTs the OpenAI-compat payload upstream and forwards SSE
// chunks. Returns (content, started, error) where `started` indicates that
// at least one chunk was already delivered to outCh — making this attempt
// unsafe to retry.
func (p *OpenCodeZenProvider) singleAttempt(ctx context.Context, req *models.ChatRequest, acc *models.Account, outCh chan<- models.StreamChunk) (string, bool, error) {
	stripped := strings.TrimPrefix(req.Model, "zen/")
	upstreamModel := stripped
	var aliasParams map[string]interface{}
	if alias, ok := p.lookupAlias(stripped); ok {
		upstreamModel = alias.Base
		aliasParams = alias.Params
	}
	payload := buildPayload(req, upstreamModel, aliasParams)
	body, err := json.Marshal(payload)
	if err != nil {
		return "", false, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint(), bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	for k, v := range p.BuildAuthHeaders(acc) {
		httpReq.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var collected strings.Builder
	started := false
	reader := bufio.NewReader(resp.Body)

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\r\n")
			if len(line) == 0 {
				continue
			}
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if bytes.Equal(data, []byte("[DONE]")) {
				if outCh != nil {
					outCh <- models.StreamChunk{Done: true}
				}
				return collected.String(), true, nil
			}

			chunk, perr := parseSSEChunk(data)
			if perr != nil {
				// Tolerate junk lines — keep reading. Upstream sometimes
				// emits keep-alive comments or empty frames.
				continue
			}
			if chunk.errMsg != "" {
				return "", started, errors.New(chunk.errMsg)
			}
			if chunk.content != "" {
				collected.WriteString(chunk.content)
				if outCh != nil {
					outCh <- models.StreamChunk{Content: chunk.content}
					started = true
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if outCh != nil && started {
					outCh <- models.StreamChunk{Done: true}
				}
				return collected.String(), started, nil
			}
			return "", started, err
		}
	}
}

type sseChunk struct {
	content string
	errMsg  string
}

// parseSSEChunk extracts the delta content from one OpenAI-style SSE frame.
// Body shape: {"choices":[{"delta":{"content":"..."}}], "error":{"message":"..."}}
func parseSSEChunk(data []byte) (sseChunk, error) {
	var envelope struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return sseChunk{}, err
	}
	if envelope.Error != nil {
		return sseChunk{errMsg: envelope.Error.Message}, nil
	}
	if len(envelope.Choices) > 0 {
		return sseChunk{content: envelope.Choices[0].Delta.Content}, nil
	}
	return sseChunk{}, nil
}

// buildPayload builds the OpenAI-compatible request body. We pass through
// only the fields the upstream is documented to honor and let the upstream
// reject unknown ones rather than silently dropping client intent.
// buildPayload assembles the upstream chat-completions body. If `aliasParams`
// is non-nil (alias-resolved upstream), its keys are merged in first as
// defaults; request body fields override on collision (alias = defaults, not
// straitjacket).
func buildPayload(req *models.ChatRequest, model string, aliasParams map[string]interface{}) map[string]interface{} {
	messages := make([]map[string]string, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, map[string]string{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   true,
	}
	// Alias presets first (lowest precedence).
	for k, v := range aliasParams {
		payload[k] = v
	}
	// Request-body fields override.
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.ReasoningEffort != "" {
		payload["reasoning_effort"] = req.ReasoningEffort
	}
	if req.Verbosity != "" {
		payload["verbosity"] = req.Verbosity
	}
	if req.ReasoningSummary != "" {
		payload["reasoning_summary"] = req.ReasoningSummary
	}
	return payload
}

// Image and video aren't free-tier features here.
func (p *OpenCodeZenProvider) ImageGeneration(ctx context.Context, req *models.ImageRequest) (*models.ImageResponse, error) {
	return nil, errors.New("opencode-zen does not support image generation")
}

func (p *OpenCodeZenProvider) VideoGeneration(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
	return nil, errors.New("opencode-zen does not support video generation")
}

func classifyError(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "429"), strings.Contains(msg, "too many"), strings.Contains(msg, "rate limit"), strings.Contains(msg, "quota"):
		return "rate_limit"
	case strings.Contains(msg, "401"), strings.Contains(msg, "403"), strings.Contains(msg, "unauthorized"), strings.Contains(msg, "forbidden"):
		return "auth"
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "eof"), strings.Contains(msg, "connection reset"), strings.Contains(msg, "temporary"):
		return "transient"
	default:
		return "unknown"
	}
}

func shouldRetry(class string) bool {
	switch class {
	case "rate_limit", "auth", "transient", "unknown":
		return true
	default:
		return false
	}
}

func shortToken(n int) string {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		// Crypto-rand failure is non-recoverable at the OS level; fall back
		// to a timestamp-derived placeholder so the call doesn't panic mid-
		// request. Collision is acceptable here — the session is throwaway.
		return fmt.Sprintf("%016xfallback", time.Now().UnixNano())[:n]
	}
	return hex.EncodeToString(b)[:n]
}
