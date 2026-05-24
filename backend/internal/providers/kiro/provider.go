package kiro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

const (
	providerName    = "kiro"
	defaultEndpoint = "https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse"
	maxAttempts     = 3
)

// KiroProvider talks to AWS CodeWhisperer (Kiro) using a Bearer accessToken
// minted from a long-lived refreshToken. v1 supports import-only onboarding:
// users paste an `aorAAAAAG…` refresh token, the backend calls Refresh once
// to harvest accessToken + profileArn, and that pair lives on the Account.
type KiroProvider struct {
	pool     *core.AccountPool
	registry *core.ProviderRegistry
	quota    *QuotaCache
	refresh  *refreshClient
	http     *http.Client

	// mu guards the in-flight refresh map. We deduplicate concurrent
	// refreshes for the same account so 10 simultaneous chat calls don't
	// hammer the OIDC endpoint 10 times.
	mu       sync.Mutex
	inflight map[string]*refreshGate
}

type refreshGate struct {
	done chan struct{}
	err  error
}

func NewProvider(pool *core.AccountPool, registry *core.ProviderRegistry) *KiroProvider {
	return &KiroProvider{
		pool:     pool,
		registry: registry,
		quota:    NewQuotaCache(registry),
		refresh:  newRefreshClient(registry),
		http: &http.Client{
			Timeout: 180 * time.Second,
		},
		inflight: make(map[string]*refreshGate),
	}
}

func (p *KiroProvider) Name() string              { return providerName }
func (p *KiroProvider) Prefix() string            { return "kiro/" }
func (p *KiroProvider) Type() models.ProviderType { return models.ProviderTypeAccount }
func (p *KiroProvider) ResolveModel(model string) string {
	return strings.TrimPrefix(model, "kiro/")
}

func (p *KiroProvider) Metadata() models.ProviderMetadata {
	return models.ProviderMetadata{
		Capabilities: models.ProviderCapabilities{SupportsChat: true, SupportsStream: true},
		Limits:       models.ProviderLimits{DefaultContextWindow: 200000, MaxRetries: maxAttempts},
	}
}

func (p *KiroProvider) Health(ctx context.Context) error {
	if p.pool.CountByStatus(providerName, models.StatusValid) == 0 {
		return errors.New("no valid kiro accounts")
	}
	return nil
}

// QuotaCache exposes the cache so the dashboard handler can fetch on demand
// without re-importing the kiro package's internals.
func (p *KiroProvider) QuotaCache() *QuotaCache { return p.quota }

// BuildAuthHeaders adds the Bearer + AWS SDK headers. Per-request invocation
// ID is generated here because each call needs its own UUID.
func (p *KiroProvider) BuildAuthHeaders(acc *models.Account) map[string]string {
	return map[string]string{
		"Authorization":          "Bearer " + acc.Token,
		"Content-Type":           "application/x-amz-json-1.0",
		"x-amz-target":           "AmazonCodeWhispererService.GenerateAssistantResponse",
		"Amz-Sdk-Request":        "attempt=1; max=3",
		"Amz-Sdk-Invocation-Id":  uuidv4(),
		"User-Agent":             "aws-sdk-js/1.0.0 ua/2.1 os/linux lang/js md/nodejs/22.0.0 KiroIDE",
	}
}

// EnsureFresh refreshes accessToken when it's close to expiry. Skew comes
// from the registry (default 60s before expiry triggers proactive refresh).
// Concurrent callers for the same account piggyback on the first refresh.
func (p *KiroProvider) EnsureFresh(acc *models.Account) error {
	if !p.needsRefresh(acc) {
		return nil
	}

	gate := p.beginRefresh(acc.Email)
	if gate.done == nil {
		// We are the leader — do the work.
		err := p.doRefresh(acc)
		p.finishRefresh(acc.Email, err)
		return err
	}
	// Wait for the leader.
	<-gate.done
	return gate.err
}

// RefreshAuth is the explicit "force refresh" path (called when an upstream
// 401 surfaces, separate from time-based EnsureFresh).
func (p *KiroProvider) RefreshAuth(acc *models.Account) error {
	gate := p.beginRefresh(acc.Email)
	if gate.done == nil {
		err := p.doRefresh(acc)
		p.finishRefresh(acc.Email, err)
		return err
	}
	<-gate.done
	return gate.err
}

func (p *KiroProvider) needsRefresh(acc *models.Account) bool {
	if acc.Token == "" {
		return true
	}
	if acc.ExpiresAt == 0 {
		// Unknown expiry — refresh once to learn it.
		return true
	}
	skew := int64(60)
	if cfg := p.registry.Get(providerName); cfg != nil && cfg.RefreshSkewSec > 0 {
		skew = int64(cfg.RefreshSkewSec)
	}
	return time.Now().Unix()+skew >= acc.ExpiresAt
}

// beginRefresh returns a gate to wait on. If we're the first caller, the
// returned gate has done==nil and the caller is expected to perform the
// refresh and call finishRefresh.
func (p *KiroProvider) beginRefresh(email string) *refreshGate {
	p.mu.Lock()
	defer p.mu.Unlock()
	if g, ok := p.inflight[email]; ok {
		return g
	}
	g := &refreshGate{done: make(chan struct{})}
	p.inflight[email] = g
	// Hand the leader an empty-channel gate that signals "you're the leader".
	return &refreshGate{}
}

func (p *KiroProvider) finishRefresh(email string, err error) {
	p.mu.Lock()
	g, ok := p.inflight[email]
	delete(p.inflight, email)
	p.mu.Unlock()
	if ok {
		g.err = err
		close(g.done)
	}
}

// doRefresh hits the refresh endpoint and writes the new token onto the
// account. Touch() persists the rotation so it survives a restart.
func (p *KiroProvider) doRefresh(acc *models.Account) error {
	res, err := p.refresh.Refresh(acc)
	if err != nil {
		return err
	}
	acc.Token = res.AccessToken
	if res.RefreshToken != "" {
		acc.RefreshToken = res.RefreshToken
	}
	if res.ExpiresIn > 0 {
		acc.ExpiresAt = time.Now().Unix() + res.ExpiresIn
	}
	if res.ProfileArn != "" {
		if acc.Metadata == nil {
			acc.Metadata = make(map[string]string)
		}
		acc.Metadata["profile_arn"] = res.ProfileArn
	}
	p.pool.Touch(acc)
	return nil
}

func (p *KiroProvider) endpoint() string {
	if cfg := p.registry.Get(providerName); cfg != nil && cfg.BaseEndpoint != "" {
		return cfg.BaseEndpoint
	}
	return defaultEndpoint
}

func (p *KiroProvider) ChatCompletion(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	content, err := p.runChat(ctx, req, nil)
	if err != nil {
		return nil, err
	}
	return &models.ChatResponse{
		ID:      "chatcmpl-kiro-" + uuidv4()[:16],
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []models.ChatChoice{{
			Index:        0,
			Message:      &models.ChatMessage{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: &models.ChatUsage{
			PromptTokens:     len(req.Messages) * 10,
			CompletionTokens: len(content) / 4,
			TotalTokens:      len(req.Messages)*10 + len(content)/4,
		},
	}, nil
}

func (p *KiroProvider) ChatStream(ctx context.Context, req *models.ChatRequest) (<-chan models.StreamChunk, error) {
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

// runChat is the shared body for both streaming and non-streaming requests.
// Mid-stream failures bubble out without retry — we can't replay partial
// output. Pre-stream failures swap to a different account.
func (p *KiroProvider) runChat(ctx context.Context, req *models.ChatRequest, outCh chan<- models.StreamChunk) (string, error) {
	exclude := make(map[string]bool)
	model := p.ResolveModel(req.Model)
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		acc, err := p.pool.Acquire(ctx, providerName, exclude)
		if err != nil {
			if lastErr != nil {
				return "", lastErr
			}
			return "", fmt.Errorf("no available kiro accounts: %w", err)
		}

		if err := p.EnsureFresh(acc); err != nil {
			p.pool.Release(acc)
			p.pool.MarkError(acc.Email, "auth", err.Error())
			exclude[acc.Email] = true
			lastErr = err
			continue
		}

		content, started, attemptErr := p.singleAttempt(ctx, acc, model, req, outCh)
		p.pool.Release(acc)

		if attemptErr == nil {
			p.pool.MarkSuccess(acc)
			return content, nil
		}

		class := classifyError(attemptErr)
		p.pool.MarkError(acc.Email, class, attemptErr.Error())
		exclude[acc.Email] = true

		if started {
			return "", attemptErr
		}
		lastErr = attemptErr
		if !shouldRetry(class) {
			return "", attemptErr
		}
	}

	if lastErr == nil {
		lastErr = errors.New("kiro request failed")
	}
	return "", lastErr
}

// singleAttempt POSTs the payload, parses EventStream frames, and emits
// OpenAI-style deltas. `started` indicates that at least one delta was
// already sent to the client — so a mid-stream error cannot be retried.
func (p *KiroProvider) singleAttempt(ctx context.Context, acc *models.Account, model string, req *models.ChatRequest, outCh chan<- models.StreamChunk) (string, bool, error) {
	payload := buildPayload(req, model, acc)
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

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	parser := NewStreamParser()
	var collected strings.Builder
	started := false
	buf := make([]byte, 32*1024)

	// Tracks tool-use frames to merge multi-chunk tool inputs (Kiro splits
	// large JSON inputs across frames). v1 doesn't expose tool_calls back
	// to the client — we only consume the events for state tracking so the
	// stream terminates cleanly.
	_ = uuidv4 // referenced to silence unused-import linting if added

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			frames, _ := parser.Feed(buf[:n])
			for _, frame := range frames {
				switch frame.EventType() {
				case "assistantResponseEvent", "codeEvent":
					if content := stringFromPayload(frame.Payload, "content"); content != "" {
						collected.WriteString(content)
						if outCh != nil {
							outCh <- models.StreamChunk{Content: content}
							started = true
						}
					}
				case "messageStopEvent", "":
					// Some frames have no event-type header — treat them as
					// keep-alive and ignore. messageStopEvent terminates.
					if frame.EventType() == "messageStopEvent" {
						if outCh != nil {
							outCh <- models.StreamChunk{Done: true}
						}
						return collected.String(), started, nil
					}
				}
			}
		}
		if readErr == io.EOF {
			if outCh != nil && started {
				outCh <- models.StreamChunk{Done: true}
			}
			return collected.String(), started, nil
		}
		if readErr != nil {
			return "", started, readErr
		}
	}
}

func stringFromPayload(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

func (p *KiroProvider) ImageGeneration(ctx context.Context, req *models.ImageRequest) (*models.ImageResponse, error) {
	return nil, errors.New("kiro does not support image generation")
}

func (p *KiroProvider) VideoGeneration(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
	return nil, errors.New("kiro does not support video generation")
}

func classifyError(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "429"), strings.Contains(msg, "too many"), strings.Contains(msg, "rate limit"), strings.Contains(msg, "quota"), strings.Contains(msg, "throttl"):
		return "rate_limit"
	case strings.Contains(msg, "401"), strings.Contains(msg, "403"), strings.Contains(msg, "unauthorized"), strings.Contains(msg, "forbidden"), strings.Contains(msg, "expired"):
		return "auth"
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "eof"), strings.Contains(msg, "connection reset"), strings.Contains(msg, "temporary"):
		return "transient"
	default:
		return "unknown"
	}
}

func shouldRetry(class string) bool {
	switch class {
	case "auth", "rate_limit", "transient", "unknown":
		return true
	default:
		return false
	}
}
