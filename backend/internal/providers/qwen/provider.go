package qwen

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

type QwenProvider struct {
	client      *QwenClient
	accountPool *core.AccountPool
	modelMap    map[string]string
}

const qwenMaxRetries = 2

func NewQwenProvider(pool *core.AccountPool) *QwenProvider {
	return &QwenProvider{
		client:      NewQwenClient(),
		accountPool: pool,
		modelMap:    initQwenModelMap(),
	}
}

// SetProxyResolver forwards a per-request proxy decision function to the
// underlying QwenClient. Pass nil to clear and route directly. Honoured by
// both doRequest (chat creation/deletion) and StreamChat (SSE streams).
func (p *QwenProvider) SetProxyResolver(r ProxyResolver) {
	p.client.SetProxyResolver(r)
}

func initQwenModelMap() map[string]string {
	return map[string]string{
		"qwen-max":            "qwen3.6-max-preview",
		"qwen-plus":           "qwen3.6-plus",
		"qwen-27b":            "qwen3.6-27b",
		"gpt-4o":              "qwen3.6-plus",
		"gpt-4o-mini":         "qwen3.6-27b",
		"gpt-4":               "qwen3.6-plus",
		"gpt-4-turbo":         "qwen3.6-plus",
		"gpt-3.5-turbo":       "qwen3.6-27b",
		"gpt-3.5":             "qwen3.6-27b",
		"claude-3-5-sonnet":   "qwen3.6-max-preview",
		"gemini-2.5-pro":      "qwen3.6-max-preview",
	}
}

func (p *QwenProvider) Name() string {
	return "qwen"
}

func (p *QwenProvider) Prefix() string {
	return "qw/"
}

func (p *QwenProvider) Type() models.ProviderType {
	return models.ProviderTypeAccount
}

func (p *QwenProvider) ResolveModel(model string) string {
	if resolved, ok := p.modelMap[model]; ok {
		return resolved
	}
	return model
}

func (p *QwenProvider) Metadata() models.ProviderMetadata {
	return models.ProviderMetadata{
		Capabilities: models.ProviderCapabilities{
			SupportsChat:   true,
			SupportsStream: true,
			SupportsImage:  true,
		},
		Limits: models.ProviderLimits{
			DefaultContextWindow: 32768,
			MaxRetries:           qwenMaxRetries,
		},
	}
}

func (p *QwenProvider) buildPayload(chatID, model string, messages []models.ChatMessage) map[string]interface{} {
	ts := int(time.Now().Unix())
	featureConfig := map[string]interface{}{
		"thinking_enabled": false,
		"output_schema":    "phase",
		"research_mode":    "normal",
		"auto_thinking":    false,
		"thinking_mode":    "off",
		"thinking_format":  "summary",
		"auto_search":      true,
		"code_interpreter": true,
		"function_calling": false,
		"plugins_enabled":  true,
	}

	msgs := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		msgs[i] = map[string]interface{}{
			"fid":            fmt.Sprintf("%d", ts+i),
			"parentId":       nil,
			"childrenIds":    []string{fmt.Sprintf("%d", ts+i+1000)},
			"role":           m.Role,
			"content":        m.Content,
			"user_action":    "chat",
			"files":          []interface{}{},
			"timestamp":      ts,
			"models":         []string{model},
			"chat_type":      "t2t",
			"feature_config": featureConfig,
			"extra": map[string]interface{}{
				"meta": map[string]string{"subChatType": "t2t"},
			},
			"sub_chat_type": "t2t",
			"parent_id":     nil,
		}
	}

	return map[string]interface{}{
		"stream":             true,
		"version":            "2.1",
		"incremental_output": true,
		"chat_id":            chatID,
		"chat_mode":          "normal",
		"model":              model,
		"parent_id":          nil,
		"messages":           msgs,
		"timestamp":          ts,
	}
}

func (p *QwenProvider) buildImagePayload(chatID, model, prompt string) map[string]interface{} {
	ts := int(time.Now().Unix())
	featureConfig := map[string]interface{}{
		"thinking_enabled":      false,
		"output_schema":         "phase",
		"auto_thinking":         false,
		"thinking_mode":         "off",
		"auto_search":           false,
		"code_interpreter":      false,
		"function_calling":      false,
		"plugins_enabled":       true,
		"image_generation":      true,
		"default_aspect_ratio":  "1:1",
		"image_size":            "1024*1024",
		"t2i_size":              "1024*1024",
	}

	return map[string]interface{}{
		"stream":             true,
		"version":            "2.1",
		"incremental_output": true,
		"chat_id":            chatID,
		"chat_mode":          "normal",
		"model":              model,
		"parent_id":          nil,
		"messages": []map[string]interface{}{
			{
				"fid":            fmt.Sprintf("%d", ts),
				"parentId":       nil,
				"childrenIds":    []string{fmt.Sprintf("%d", ts+1000)},
				"role":           "user",
				"content":        fmt.Sprintf("\u751f\u6210\u56fe\u7247\uff1a%s", prompt),
				"user_action":    "chat",
				"files":          []interface{}{},
				"timestamp":      ts,
				"models":         []string{model},
				"chat_type":      "t2i",
				"feature_config": featureConfig,
				"extra": map[string]interface{}{
					"meta": map[string]string{"subChatType": "t2i"},
				},
				"sub_chat_type": "t2i",
				"parent_id":     nil,
			},
		},
		"timestamp": ts,
	}
}

func (p *QwenProvider) buildVideoPayload(chatID, model, prompt, aspectRatio string) map[string]interface{} {
	ts := int(time.Now().Unix())
	ratioToSize := map[string]string{
		"1:1":  "1024*1024",
		"16:9": "1280*720",
		"9:16": "720*1280",
		"4:3":  "1024*768",
		"3:4":  "768*1024",
	}
	px := ratioToSize[aspectRatio]
	if px == "" {
		px = "1280*720"
		aspectRatio = "16:9"
	}
	pxX := strings.ReplaceAll(px, "*", "x")
	parts := strings.Split(px, "*")
	width, height := 1280, 720
	if len(parts) == 2 {
		fmt.Sscanf(parts[0], "%d", &width)
		fmt.Sscanf(parts[1], "%d", &height)
	}
	featureConfig := map[string]interface{}{
		"thinking_enabled":     false,
		"output_schema":        "phase",
		"auto_thinking":        false,
		"thinking_mode":        "off",
		"auto_search":          false,
		"code_interpreter":     false,
		"function_calling":     false,
		"plugins_enabled":      true,
		"video_generation":     true,
		"default_aspect_ratio": aspectRatio,
		"video_size":           px,
		"t2v_size":             px,
	}

	return map[string]interface{}{
		"stream":             true,
		"version":            "2.1",
		"incremental_output": true,
		"chat_id":            chatID,
		"chat_mode":          "normal",
		"model":              model,
		"parent_id":          nil,
		"messages": []map[string]interface{}{
			{
				"fid":         fmt.Sprintf("%d", ts),
				"parentId":    nil,
				"childrenIds": []string{fmt.Sprintf("%d", ts+1000)},
				"role":        "user",
				"content":     fmt.Sprintf("Generate video: %s", prompt),
				"user_action": "chat",
				"files":       []interface{}{},
				"timestamp":   ts,
				"models":      []string{model},
				"chat_type":   "t2v",
				"feature_config": featureConfig,
				"extra": map[string]interface{}{
					"meta": map[string]interface{}{
						"subChatType":              "t2v",
						"mode":                     "video_generation",
						"aspectRatio":              aspectRatio,
						"videoSize":                px,
						"size":                     pxX,
						"width":                    width,
						"height":                   height,
						"video_generation_enabled": true,
					},
				},
				"sub_chat_type": "t2v",
				"parent_id":     nil,
			},
		},
		"timestamp": ts,
	}
}

func (p *QwenProvider) ChatCompletion(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	model := p.ResolveModel(req.Model)
	fullContent, err := p.completeWithRetry(ctx, req, model)
	if err != nil {
		return nil, err
	}

	completionID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

	return &models.ChatResponse{
		ID:      completionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []models.ChatChoice{
			{
				Index: 0,
				Message: &models.ChatMessage{
					Role:    "assistant",
					Content: fullContent,
				},
				FinishReason: "stop",
			},
		},
		Usage: &models.ChatUsage{
			PromptTokens:     len(req.Messages) * 10,
			CompletionTokens: len(fullContent) / 4,
			TotalTokens:      len(req.Messages)*10 + len(fullContent)/4,
		},
	}, nil
}

func (p *QwenProvider) ChatStream(ctx context.Context, req *models.ChatRequest) (<-chan models.StreamChunk, error) {
	outCh := make(chan models.StreamChunk, 100)

	// send guards every write to outCh with a ctx select so a dead client
	// can never deadlock this goroutine on a full buffer. Returns false when
	// the client is gone — caller exits the producer loop.
	send := func(sc models.StreamChunk) bool {
		select {
		case outCh <- sc:
			return true
		case <-ctx.Done():
			return false
		}
	}

	go func() {
		defer close(outCh)
		exclude := make(map[string]bool)
		model := p.ResolveModel(req.Model)
		var lastErr error

		for attempt := 0; attempt <= qwenMaxRetries; attempt++ {
			// Abort retry loop on client disconnect — no point burning
			// another account on a request nobody is listening to.
			if ctx.Err() != nil {
				return
			}

			acc, err := p.accountPool.Acquire(ctx, "qwen", exclude)
			if err != nil {
				if lastErr != nil {
					send(models.StreamChunk{Error: lastErr.Error()})
					return
				}
				send(models.StreamChunk{Error: err.Error()})
				return
			}

			started, attemptErr := p.streamSingleAttempt(ctx, req, model, acc, outCh, send)
			p.accountPool.Release(acc)
			if attemptErr == nil {
				p.accountPool.MarkSuccess(acc)
				return
			}

			class := classifyQwenError(attemptErr)
			p.accountPool.MarkError(acc.Email, class, attemptErr.Error())

			if started {
				// streamSingleAttempt already emitted an Error trailer to outCh
				// for mid-stream failures. Don't retry — the client has seen
				// partial content from THIS attempt and we can't replay/merge.
				return
			}
			// Bail out instead of retrying when the client is gone.
			if ctx.Err() != nil {
				return
			}

			lastErr = attemptErr
			exclude[acc.Email] = true
			if !shouldRetryQwenError(class) {
				send(models.StreamChunk{Error: attemptErr.Error()})
				return
			}
		}

		if lastErr != nil {
			send(models.StreamChunk{Error: lastErr.Error()})
		}
	}()

	return outCh, nil
}

func (p *QwenProvider) completeWithRetry(ctx context.Context, req *models.ChatRequest, model string) (string, error) {
	exclude := make(map[string]bool)
	var lastErr error

	for attempt := 0; attempt <= qwenMaxRetries; attempt++ {
		acc, err := p.accountPool.Acquire(ctx, "qwen", exclude)
		if err != nil {
			if lastErr != nil {
				return "", lastErr
			}
			return "", fmt.Errorf("no available accounts: %w", err)
		}

		content, attemptErr := p.completeSingleAttempt(ctx, req, model, acc)
		p.accountPool.Release(acc)
		if attemptErr == nil {
			p.accountPool.MarkSuccess(acc)
			return content, nil
		}

		lastErr = attemptErr
		class := classifyQwenError(attemptErr)
		exclude[acc.Email] = true
		p.accountPool.MarkError(acc.Email, class, attemptErr.Error())
		if !shouldRetryQwenError(class) {
			return "", attemptErr
		}
	}

	if lastErr == nil {
		lastErr = errors.New("qwen request failed")
	}
	return "", lastErr
}

func (p *QwenProvider) completeSingleAttempt(ctx context.Context, req *models.ChatRequest, model string, acc *models.Account) (string, error) {
	chatID, err := p.client.CreateChat(ctx, acc.Token, model, "t2t")
	if err != nil {
		return "", err
	}
	defer p.client.DeleteChat(ctx, acc.Token, chatID)

	payload := p.buildPayload(chatID, model, req.Messages)
	chunks, err := p.client.StreamChat(ctx, acc.Token, chatID, payload)
	if err != nil {
		return "", err
	}

	var fullContent strings.Builder
	for chunk := range chunks {
		if chunk.Error != "" {
			return "", errors.New(chunk.Error)
		}
		fullContent.WriteString(chunk.Content)
		if chunk.Done {
			break
		}
	}
	return fullContent.String(), nil
}

// streamSingleAttempt runs one upstream attempt and forwards chunks to outCh.
// It returns `started=true` once at least one chunk has been pushed to outCh
// (i.e. the client has already observed partial output for this attempt). The
// caller MUST NOT retry when started=true — doing so would concatenate output
// from two different generations into the same client response.
//
// Pre-stream failures (account acquisition, chat creation, HTTP error before
// the first chunk, or an error envelope as the first chunk) return started=false
// and are safe to retry on a different account.
//
// `send` is ctx-aware: returns false when the client is gone, at which point
// we abort the read loop and let upstream goroutines drain via ctx cancel.
func (p *QwenProvider) streamSingleAttempt(ctx context.Context, req *models.ChatRequest, model string, acc *models.Account, outCh chan<- models.StreamChunk, send func(models.StreamChunk) bool) (bool, error) {
	chatID, err := p.client.CreateChat(ctx, acc.Token, model, "t2t")
	if err != nil {
		return false, err
	}
	defer p.client.DeleteChat(context.Background(), acc.Token, chatID)

	payload := p.buildPayload(chatID, model, req.Messages)
	chunks, err := p.client.StreamChat(ctx, acc.Token, chatID, payload)
	if err != nil {
		return false, err
	}

	started := false
	for {
		select {
		case <-ctx.Done():
			// Client disconnected — drop the in-flight chunks and exit.
			// chunks goroutine observes ctx via its own select on its write
			// side and will close `chunks` itself once it unwinds.
			return started, ctx.Err()
		case chunk, ok := <-chunks:
			if !ok {
				return started, nil
			}
			if chunk.Error != "" {
				e := errors.New(chunk.Error)
				if started {
					// Mid-stream failure: client already saw partial content,
					// surface the error as a trailer instead of retrying.
					send(models.StreamChunk{Error: chunk.Error})
				}
				return started, e
			}
			if !send(models.StreamChunk{Content: chunk.Content, Done: chunk.Done}) {
				return started, ctx.Err()
			}
			started = true
			if chunk.Done {
				return true, nil
			}
		}
	}
}

func classifyQwenError(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "429"), strings.Contains(msg, "too many"), strings.Contains(msg, "rate limit"):
		return "rate_limit"
	case strings.Contains(msg, "banned"), strings.Contains(msg, "suspended"), strings.Contains(msg, "blocked"), strings.Contains(msg, "forbidden by policy"):
		return "banned"
	case strings.Contains(msg, "401"), strings.Contains(msg, "403"), strings.Contains(msg, "unauthorized"), strings.Contains(msg, "token"), strings.Contains(msg, "forbidden"):
		return "auth"
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "eof"), strings.Contains(msg, "connection reset"), strings.Contains(msg, "temporary"):
		return "transient"
	default:
		return "unknown"
	}
}

func shouldRetryQwenError(class string) bool {
	switch class {
	case "auth", "rate_limit", "transient", "unknown":
		return true
	default:
		return false
	}
}

func (p *QwenProvider) ImageGeneration(ctx context.Context, req *models.ImageRequest) (*models.ImageResponse, error) {
	acc, err := p.accountPool.Acquire(ctx, "qwen", nil)
	if err != nil {
		return nil, fmt.Errorf("no available accounts: %w", err)
	}
	defer p.accountPool.Release(acc)

	model := "qwen3.6-plus"
	imageURL, err := p.client.GenerateImage(ctx, acc.Token, req.Prompt, p.buildImagePayload, model)
	if err != nil {
		return nil, fmt.Errorf("image generation failed: %w", err)
	}

	return &models.ImageResponse{
		Created: time.Now().Unix(),
		Data: []models.ImageData{
			{
				URL:           imageURL,
				RevisedPrompt: req.Prompt,
			},
		},
	}, nil
}

func (p *QwenProvider) VideoGeneration(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
	model := req.Model
	if model == "" || strings.HasPrefix(model, "qw/") {
		model = "qwen3.6-plus"
	}
	aspectRatio := req.AspectRatio
	if aspectRatio == "" {
		aspectRatio = "16:9"
	}

	wanted := req.N
	if wanted <= 0 {
		wanted = 1
	}
	if wanted > 2 {
		wanted = 2 // upstream is heavy; mirror the reference cap
	}

	maxAttempts := wanted * 2
	exclude := make(map[string]bool)
	var collected []string
	var errMsgs []string

	for attempt := 0; attempt < maxAttempts && len(collected) < wanted; attempt++ {
		acc, err := p.accountPool.Acquire(ctx, "qwen", exclude)
		if err != nil {
			errMsgs = append(errMsgs, "No available accounts (all rate-limited or cooling down)")
			break
		}

		result, gerr := p.client.GenerateVideo(ctx, acc.Token, req.Prompt, p.buildVideoPayload, model, aspectRatio)
		p.accountPool.Release(acc)

		// Even on a "successful" stream, the body may contain an upstream error envelope.
		// Classify on either gerr or the raw body so callers see a sane message.
		if gerr == nil && result != nil {
			if cls := classifyVideoBody(result.RawBody); cls != "" {
				gerr = errors.New(cls)
			}
		}

		if gerr != nil {
			class := classifyQwenError(gerr)
			p.accountPool.MarkError(acc.Email, class, gerr.Error())
			exclude[acc.Email] = true
			errMsgs = append(errMsgs, friendlyVideoError(gerr.Error()))
			continue
		}

		p.accountPool.MarkSuccess(acc)
		for _, u := range result.URLs {
			if len(collected) >= wanted {
				break
			}
			collected = append(collected, u)
		}
		if len(result.URLs) == 0 {
			// Stream completed but no URLs surfaced — usually means the account does not
			// have video generation enabled. Don't retry the same account.
			exclude[acc.Email] = true
			snippet := result.RawBody
			if len(snippet) > 160 {
				snippet = snippet[:160]
			}
			errMsgs = append(errMsgs, "Video generation is not available on this account (no video URL in upstream response): "+snippet)
		}
	}

	if len(collected) == 0 {
		detail := "Video generation failed: no video URL in upstream response (the feature may not be enabled on this account)"
		if len(errMsgs) > 0 {
			detail = "Video generation failed: " + strings.Join(dedupeStrings(errMsgs), "; ")
		}
		return nil, errors.New(detail)
	}

	data := make([]models.VideoData, 0, len(collected))
	for _, u := range collected {
		data = append(data, models.VideoData{
			URL:           u,
			RevisedPrompt: req.Prompt,
			AspectRatio:   aspectRatio,
		})
	}
	return &models.VideoResponse{
		Created: time.Now().Unix(),
		Data:    data,
	}, nil
}

func classifyVideoBody(raw string) string {
	if raw == "" {
		return ""
	}
	low := strings.ToLower(raw)
	switch {
	case strings.Contains(low, "ratelimited"),
		strings.Contains(low, "rate_limited"),
		strings.Contains(low, "daily usage limit"),
		strings.Contains(low, "reached the da"):
		return "Account hit the daily usage limit"
	case strings.Contains(low, "not supported"),
		strings.Contains(low, "unsupported"),
		strings.Contains(low, "feature not available"),
		strings.Contains(low, "not available"):
		return "Video generation is not available on this account"
	case strings.Contains(raw, `"success":false`), strings.Contains(raw, `"success": false`):
		return "Upstream API error from Qwen"
	}
	return ""
}

func friendlyVideoError(msg string) string {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "ratelimit"), strings.Contains(low, "rate_limit"), strings.Contains(low, "daily"), strings.Contains(low, "usage limit"):
		return "Account hit the daily usage limit"
	case strings.Contains(low, "not available"), strings.Contains(low, "not supported"), strings.Contains(low, "unsupported"):
		return "Video generation is not available on this account"
	case strings.Contains(low, "unauthorized"), strings.Contains(low, "auth"), strings.Contains(low, "token"):
		return "Account authentication failed"
	case strings.Contains(low, "banned"):
		return "Account has been banned"
	default:
		if len(msg) > 160 {
			msg = msg[:160]
		}
		return "Generation failed: " + msg
	}
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func (p *QwenProvider) Health(ctx context.Context) error {
	status := p.accountPool.GetStatus()
	if status["valid"].(int) == 0 {
		return fmt.Errorf("no valid accounts available")
	}
	return nil
}

// BuildAuthHeaders returns headers Qwen needs for upstream calls. The cookie
// blob lives in acc.Token (legacy field name; format is the full Cookie
// header).
func (p *QwenProvider) BuildAuthHeaders(acc *models.Account) map[string]string {
	return map[string]string{
		"Cookie":       acc.Token,
		"User-Agent":   "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Accept":       "text/event-stream",
		"Content-Type": "application/json",
	}
}

// EnsureFresh is a no-op for Qwen — cookies don't carry an explicit expiry;
// they fail visibly when the server invalidates them, which triggers
// MarkError("auth") → RefreshAuth via re-login.
func (p *QwenProvider) EnsureFresh(acc *models.Account) error {
	return nil
}

// RefreshAuth performs an email+password re-login when the cookie is rejected.
// Full re-login flow lives in BACKLOG #22; today this surfaces a clear error
// rather than silently swallowing the failure.
func (p *QwenProvider) RefreshAuth(acc *models.Account) error {
	if acc.Password == "" {
		return errors.New("qwen re-auth requires stored password; none set")
	}
	return errors.New("qwen re-auth not yet wired (see BACKLOG #22 auth-resolver)")
}
