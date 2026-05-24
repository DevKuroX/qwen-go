package geminiweb

import (
	"context"
	"encoding/base64"
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
	providerName = "gemini-web"
	maxAttempts  = 2
)

// GeminiWebProvider implements the cookie-pair + scraped-HTML-token auth
// shape. Accounts are real Google logins; we never auto-spawn. PSID and
// PSIDTS cookies live on the Account; the per-session SNlM0e access token
// is in-memory only and refreshed on EnsureFresh.
type GeminiWebProvider struct {
	pool     *core.AccountPool
	registry *core.ProviderRegistry

	mu       sync.Mutex
	sessions map[string]*session
}

func NewProvider(pool *core.AccountPool, registry *core.ProviderRegistry) *GeminiWebProvider {
	return &GeminiWebProvider{
		pool:     pool,
		registry: registry,
		sessions: make(map[string]*session),
	}
}

func (p *GeminiWebProvider) Name() string              { return providerName }
func (p *GeminiWebProvider) Prefix() string            { return "gw/" }
func (p *GeminiWebProvider) Type() models.ProviderType { return models.ProviderTypeAccount }
func (p *GeminiWebProvider) ResolveModel(model string) string {
	return strings.TrimPrefix(model, "gw/")
}

func (p *GeminiWebProvider) Metadata() models.ProviderMetadata {
	return models.ProviderMetadata{
		Capabilities: models.ProviderCapabilities{SupportsChat: true, SupportsStream: true},
		Limits:       models.ProviderLimits{DefaultContextWindow: 32768, MaxRetries: maxAttempts},
	}
}

func (p *GeminiWebProvider) Health(ctx context.Context) error {
	if p.pool.CountByStatus(providerName, models.StatusValid) == 0 {
		return errors.New("no valid gemini-web accounts")
	}
	return nil
}

// BuildAuthHeaders returns the cookie-domain headers. Per-request bits
// (UUID-embedded x-goog-ext-525005358-jspb) are added in singleAttempt
// because they vary per call.
func (p *GeminiWebProvider) BuildAuthHeaders(acc *models.Account) map[string]string {
	return map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Origin":     "https://gemini.google.com",
		"Referer":    "https://gemini.google.com/",
	}
}

// EnsureFresh bootstraps the in-memory session on first use and refreshes
// the SNlM0e access token + PSIDTS cookie when the TTL has elapsed.
func (p *GeminiWebProvider) EnsureFresh(acc *models.Account) error {
	sess := p.getOrCreateSession(acc)

	if !sess.IsAuthenticated() {
		if err := sess.Init(); err != nil {
			return err
		}
	}

	ttl := p.tokenTTL()
	if sess.IsTokenExpired(ttl) {
		if err := sess.RefreshAccessToken(); err != nil {
			return err
		}
		// PSIDTS may have rotated — persist to the account so the snapshot
		// loop (BACKLOG #22) catches the new value.
		if sess.Secure1PSIDTS != "" && sess.Secure1PSIDTS != acc.RefreshToken {
			acc.RefreshToken = sess.Secure1PSIDTS
			p.pool.Touch(acc)
		}
	}
	return nil
}

// RefreshAuth — same path as EnsureFresh. PSID recapture (browser-driven)
// is intentionally out of scope here; admin runs CLI/dashboard recapture.
func (p *GeminiWebProvider) RefreshAuth(acc *models.Account) error {
	return p.EnsureFresh(acc)
}

func (p *GeminiWebProvider) getOrCreateSession(acc *models.Account) *session {
	p.mu.Lock()
	defer p.mu.Unlock()

	if existing, ok := p.sessions[acc.Email]; ok {
		return existing
	}
	sess := newSession(acc.Token, acc.RefreshToken, "", p.modelLookup)
	p.sessions[acc.Email] = sess
	return sess
}

// modelLookup is injected into the session so model IDs come from the
// dashboard-editable registry instead of a hard-coded map.
func (p *GeminiWebProvider) modelLookup(name string) modelMeta {
	if p.registry == nil {
		return modelMeta{}
	}
	cfg := p.registry.Get(providerName)
	if cfg == nil {
		return modelMeta{}
	}
	if m, ok := cfg.Models[name]; ok {
		return modelMeta{ModelName: name, ModelID: m.ID, Capacity: m.Capacity}
	}
	return modelMeta{}
}

func (p *GeminiWebProvider) tokenTTL() time.Duration {
	if cfg := p.registry.Get(providerName); cfg != nil && cfg.AccessTokenTTLMinutes > 0 {
		return time.Duration(cfg.AccessTokenTTLMinutes) * time.Minute
	}
	return 50 * time.Minute
}

func (p *GeminiWebProvider) ChatCompletion(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	content, err := p.runChat(ctx, req, nil)
	if err != nil {
		return nil, err
	}
	return &models.ChatResponse{
		ID:      "chatcmpl-gw-" + newUUID()[:16],
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

func (p *GeminiWebProvider) ChatStream(ctx context.Context, req *models.ChatRequest) (<-chan models.StreamChunk, error) {
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

func (p *GeminiWebProvider) runChat(ctx context.Context, req *models.ChatRequest, outCh chan<- models.StreamChunk) (string, error) {
	exclude := make(map[string]bool)
	prompt := flattenMessages(req.Messages)
	model := p.ResolveModel(req.Model)
	var lastErr error

	for attempt := 0; attempt <= maxAttempts; attempt++ {
		acc, err := p.pool.Acquire(ctx, providerName, exclude)
		if err != nil {
			if lastErr != nil {
				return "", lastErr
			}
			return "", fmt.Errorf("no available gemini-web accounts: %w", err)
		}

		if err := p.EnsureFresh(acc); err != nil {
			p.pool.Release(acc)
			p.pool.MarkError(acc.Email, "auth", err.Error())
			exclude[acc.Email] = true
			lastErr = err
			continue
		}

		content, started, attemptErr := p.singleAttempt(ctx, acc, model, prompt, outCh)
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
		lastErr = errors.New("gemini-web request failed")
	}
	return "", lastErr
}

func (p *GeminiWebProvider) singleAttempt(ctx context.Context, acc *models.Account, model, prompt string, outCh chan<- models.StreamChunk) (string, bool, error) {
	sess := p.getOrCreateSession(acc)

	url, body, err := sess.BuildChatPayload(prompt, model)
	if err != nil {
		return "", false, err
	}

	uid := strings.ToUpper(newUUID())
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	if err != nil {
		return "", false, err
	}
	for k, v := range p.BuildAuthHeaders(acc) {
		httpReq.Header.Set(k, v)
	}
	for k, v := range sess.buildRequestHeaders(model, uid) {
		httpReq.Header.Set(k, v)
	}

	// Stream timeout — Gemini's frame protocol has no natural EOF; the
	// session client already wraps with a longer timeout for streaming.
	sess.client.client.Timeout = 180 * time.Second

	resp, err := sess.client.do(httpReq)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	chunkCh := make(chan geminiResponse, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ParseStream(resp.Body, chunkCh)
	}()

	var collected strings.Builder
	prevText := ""
	started := false

	for chunk := range chunkCh {
		// Gemini streams cumulative text — diff against the previous frame to
		// expose only the new characters as a delta to OpenAI-style clients.
		if chunk.Text != "" && chunk.Text != prevText {
			delta := chunk.Text
			if strings.HasPrefix(chunk.Text, prevText) {
				delta = chunk.Text[len(prevText):]
			}
			prevText = chunk.Text
			collected.WriteString(delta)
			if outCh != nil {
				outCh <- models.StreamChunk{Content: delta}
				started = true
			}
		}
		if chunk.Done {
			if outCh != nil {
				outCh <- models.StreamChunk{Done: true}
			}
		}
	}

	if perr := <-errCh; perr != nil {
		return "", started, perr
	}
	return collected.String(), started, nil
}

// ImageGeneration handles both text-to-image and image-edit. When req.Image
// is set, the bytes are first POSTed to content-push.googleapis.com/upload
// and the returned URL is threaded into message_content[3] as file_data —
// Gemini then sees the prompt as "edit this image" instead of "generate from
// scratch". Output URLs are harvested from candidate_data[12][7][0] (plain)
// or [12][0]["8"][0] (edit), per HanaokaYuzu/Gemini-API client.py:1443-1467.
func (p *GeminiWebProvider) ImageGeneration(ctx context.Context, req *models.ImageRequest) (*models.ImageResponse, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}

	model := p.ResolveModel(req.Model)
	exclude := make(map[string]bool)
	var lastErr error

	for attempt := 0; attempt <= maxAttempts; attempt++ {
		acc, err := p.pool.Acquire(ctx, providerName, exclude)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("no available gemini-web accounts: %w", err)
		}

		if err := p.EnsureFresh(acc); err != nil {
			p.pool.Release(acc)
			p.pool.MarkError(acc.Email, "auth", err.Error())
			exclude[acc.Email] = true
			lastErr = err
			continue
		}

		images, attemptErr := p.singleImageAttempt(ctx, acc, model, req)
		p.pool.Release(acc)

		if attemptErr == nil {
			p.pool.MarkSuccess(acc)
			data := make([]models.ImageData, 0, len(images))
			for _, img := range images {
				data = append(data, models.ImageData{URL: img.URL, RevisedPrompt: img.Alt})
			}
			return &models.ImageResponse{
				Created: time.Now().Unix(),
				Data:    data,
			}, nil
		}

		class := classifyError(attemptErr)
		p.pool.MarkError(acc.Email, class, attemptErr.Error())
		exclude[acc.Email] = true
		lastErr = attemptErr
		if !shouldRetry(class) {
			return nil, attemptErr
		}
	}

	if lastErr == nil {
		lastErr = errors.New("gemini-web image generation failed")
	}
	return nil, lastErr
}

// singleImageAttempt is the per-account image gen/edit transaction. Mirrors
// singleAttempt for chat but consumes the full stream (no client-side
// streaming for image requests) and returns the harvested URL list.
func (p *GeminiWebProvider) singleImageAttempt(ctx context.Context, acc *models.Account, model string, req *models.ImageRequest) ([]generatedImage, error) {
	sess := p.getOrCreateSession(acc)

	var fileData []interface{}
	if req.Image != "" {
		raw, filename, err := decodeImageInput(req.Image)
		if err != nil {
			return nil, fmt.Errorf("decode input image: %w", err)
		}
		uploadedURL, err := sess.UploadImage(raw, filename)
		if err != nil {
			return nil, fmt.Errorf("upload input image: %w", err)
		}
		// file_data shape per HanaokaYuzu lib: [[ [url], filename ], …]
		fileData = []interface{}{
			[]interface{}{[]interface{}{uploadedURL}, filename},
		}
	}

	url, body, err := sess.BuildChatPayloadWithFiles(req.Prompt, model, fileData)
	if err != nil {
		return nil, err
	}

	uid := strings.ToUpper(newUUID())
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range p.BuildAuthHeaders(acc) {
		httpReq.Header.Set(k, v)
	}
	for k, v := range sess.buildRequestHeaders(model, uid) {
		httpReq.Header.Set(k, v)
	}

	sess.client.client.Timeout = 180 * time.Second
	resp, err := sess.client.do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	chunkCh := make(chan geminiResponse, 64)
	parseErrCh := make(chan error, 1)
	go func() { parseErrCh <- ParseStream(resp.Body, chunkCh) }()

	var images []generatedImage
	seen := make(map[string]bool)
	for chunk := range chunkCh {
		for _, img := range chunk.Images {
			if img.URL == "" || seen[img.URL] {
				continue
			}
			seen[img.URL] = true
			images = append(images, img)
		}
	}
	if perr := <-parseErrCh; perr != nil {
		return nil, perr
	}
	if len(images) == 0 {
		return nil, errors.New("gemini-web returned no images — prompt may be unsupported or rate-limited")
	}
	return images, nil
}

// decodeImageInput accepts either a data URL (data:image/png;base64,…) or a
// raw base64 string and returns the binary payload plus a best-guess filename
// extension based on the declared mime type.
func decodeImageInput(s string) ([]byte, string, error) {
	filename := "input.png"
	payload := s
	if strings.HasPrefix(s, "data:") {
		comma := strings.IndexByte(s, ',')
		if comma < 0 {
			return nil, "", errors.New("malformed data URL")
		}
		header := s[:comma]
		payload = s[comma+1:]
		// header looks like "data:image/jpeg;base64"
		if strings.Contains(header, "image/jpeg") {
			filename = "input.jpg"
		} else if strings.Contains(header, "image/webp") {
			filename = "input.webp"
		} else if strings.Contains(header, "image/gif") {
			filename = "input.gif"
		}
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		// Tolerate URL-safe / unpadded variants.
		raw, err = base64.RawStdEncoding.DecodeString(payload)
		if err != nil {
			return nil, "", fmt.Errorf("base64 decode: %w", err)
		}
	}
	return raw, filename, nil
}

func (p *GeminiWebProvider) VideoGeneration(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
	return nil, errors.New("gemini-web video generation not yet implemented")
}

func flattenMessages(messages []models.ChatMessage) string {
	var sb strings.Builder
	for _, m := range messages {
		if m.Role == "system" {
			sb.WriteString("[system]\n")
			sb.WriteString(m.Content)
			sb.WriteString("\n\n")
			continue
		}
		if m.Role == "assistant" {
			sb.WriteString("[assistant]\n")
		} else {
			sb.WriteString("[user]\n")
		}
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

func classifyError(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "429"), strings.Contains(msg, "too many"), strings.Contains(msg, "rate limit"):
		return "rate_limit"
	case strings.Contains(msg, "401"), strings.Contains(msg, "403"), strings.Contains(msg, "unauthorized"), strings.Contains(msg, "forbidden"),
		strings.Contains(msg, "snlm0e"), strings.Contains(msg, "cookies may be invalid"):
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
