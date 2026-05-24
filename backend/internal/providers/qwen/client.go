package qwen

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
	"go.uber.org/zap"
)

// ProxyResolver is consulted before every outbound Qwen request. Implementations
// return:
//   - (nil, nil)     → use the direct pooled client (no proxy this request).
//   - (proxy, nil)   → build a fresh proxied client wired to this proxy.
//   - (nil, err)     → refuse the request (e.g. proxy required but no live
//                      proxy available and FallbackDirect=false).
//
// A nil resolver means "always direct" — current behavior.
type ProxyResolver func() (*models.Proxy, error)

type QwenClient struct {
	httpClient    *http.Client
	logger        *zap.Logger
	baseURL       string
	proxyResolver ProxyResolver
}

func NewQwenClient() *QwenClient {
	return &QwenClient{
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		logger:  zap.L(),
		baseURL: "https://chat.qwen.ai",
	}
}

// SetProxyResolver swaps in (or clears, when nil) the resolver consulted
// before each outbound request. Safe to call once during wire-up; not
// goroutine-safe for live swaps.
func (c *QwenClient) SetProxyResolver(r ProxyResolver) {
	c.proxyResolver = r
}

// clientFor picks the http.Client to use for one outbound. Returns the
// pooled direct client when no resolver is set or the resolver yields
// (nil, nil). Returns a fresh proxied client built around `p` otherwise.
// Errors from the resolver are surfaced to the caller (the request fails).
func (c *QwenClient) clientFor() (*http.Client, error) {
	if c.proxyResolver == nil {
		return c.httpClient, nil
	}
	p, err := c.proxyResolver()
	if err != nil {
		return nil, err
	}
	if p == nil {
		return c.httpClient, nil
	}
	return newProxiedHTTPClient(p, c.httpClient.Timeout)
}

// newProxiedHTTPClient builds a one-off http.Client tunneled through `p`.
// Connection pooling is per-instance, so on hot paths the caller may see
// extra TLS handshake cost — accepted for now given low expected QPS.
func newProxiedHTTPClient(p *models.Proxy, timeout time.Duration) (*http.Client, error) {
	scheme := "http"
	if p.Type == "socks5" {
		scheme = "socks5"
	}
	var raw string
	if p.Username != "" {
		raw = fmt.Sprintf("%s://%s:%s@%s:%d", scheme, p.Username, p.Password, p.Host, p.Port)
	} else {
		raw = fmt.Sprintf("%s://%s:%d", scheme, p.Host, p.Port)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy url %q: %w", raw, err)
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(parsed),
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     60 * time.Second,
		},
	}, nil
}

func (c *QwenClient) doRequest(ctx context.Context, method, path, token string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Origin", "https://chat.qwen.ai")
	req.Header.Set("Referer", "https://chat.qwen.ai/")
	req.Header.Set("source", "web")
	req.Header.Set("version", "0.2.46")
	req.Header.Set("sec-ch-ua", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client, err := c.clientFor()
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return respBody, resp.StatusCode, nil
}

func (c *QwenClient) CreateChat(ctx context.Context, token, model, chatType string) (string, error) {
	if chatType == "" {
		chatType = "t2t"
	}
	ts := time.Now().Unix()
	body := map[string]interface{}{
		"title":     fmt.Sprintf("api_%d", ts),
		"models":    []string{model},
		"chat_mode": "normal",
		"chat_type": chatType,
		"timestamp": ts,
	}

	respBody, status, err := c.doRequest(ctx, "POST", "/api/v2/chats/new", token, body)
	if err != nil {
		return "", err
	}

	if status != 200 {
		return "", fmt.Errorf("create chat failed: %d - %s", status, string(respBody)[:min(200, len(respBody))])
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	if !result.Success || result.Data.ID == "" {
		return "", fmt.Errorf("create chat returned no ID")
	}

	c.logger.Info("chat created", zap.String("chat_id", result.Data.ID))
	return result.Data.ID, nil
}

func (c *QwenClient) DeleteChat(ctx context.Context, token, chatID string) error {
	_, _, err := c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/v2/chats/%s", chatID), token, nil)
	return err
}

type StreamChunk struct {
	Content string
	Done    bool
	Error   string
	// ExtraURLs holds media URLs harvested from `delta.extra` / phase-event `extra`
	// (Qwen often returns video/image URLs there rather than inside `content`).
	ExtraURLs []string
}

func (c *QwenClient) StreamChat(ctx context.Context, token, chatID string, payload map[string]interface{}) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 100)

	go func() {
		defer close(ch)

		// sendChunk respects ctx cancellation when the consumer stops draining
		// the buffer (e.g. client disconnected upstream). Without this, a full
		// `ch` would deadlock this goroutine and keep resp.Body open until the
		// http.Client.Timeout (120s) fires.
		sendChunk := func(sc StreamChunk) bool {
			select {
			case ch <- sc:
				return true
			case <-ctx.Done():
				return false
			}
		}

		jsonBody, _ := json.Marshal(payload)
		url := fmt.Sprintf("%s/api/v2/chat/completions?chat_id=%s", c.baseURL, chatID)
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
		if err != nil {
			sendChunk(StreamChunk{Error: err.Error()})
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
		req.Header.Set("Origin", "https://chat.qwen.ai")
		req.Header.Set("Referer", "https://chat.qwen.ai/")
		req.Header.Set("source", "web")
		req.Header.Set("version", "0.2.46")
		req.Header.Set("sec-ch-ua", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`)
		req.Header.Set("sec-ch-ua-mobile", "?0")
		req.Header.Set("sec-ch-ua-platform", `"Windows"`)
		req.Header.Set("sec-fetch-dest", "empty")
		req.Header.Set("sec-fetch-mode", "cors")
		req.Header.Set("sec-fetch-site", "same-origin")

		client, err := c.clientFor()
		if err != nil {
			sendChunk(StreamChunk{Error: err.Error()})
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			sendChunk(StreamChunk{Error: err.Error()})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			sendChunk(StreamChunk{Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)[:min(500, len(body))])})
			return
		}

		if err := c.readSSEStream(ctx, resp.Body, ch); err != nil {
			if errors.Is(err, io.EOF) {
				c.logger.Info("stream completed")
				sendChunk(StreamChunk{Done: true})
				return
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// Client went away — no point emitting an Error chunk because
				// the consumer is also gone. Just exit and let defers run.
				return
			}
			c.logger.Error("stream read error", zap.Error(err))
			sendChunk(StreamChunk{Error: err.Error()})
		}
	}()

	return ch, nil
}

func (c *QwenClient) readSSEStream(ctx context.Context, body io.Reader, ch chan<- StreamChunk) error {
	reader := bufio.NewReader(body)
	var eventLines []string

	for {
		// Short-circuit on cancel before every read iteration so we don't sit
		// in ReadString past the client disconnect. The HTTP layer also
		// cancels the underlying body on ctx, but this lets us bail out one
		// hop earlier and avoid emitting trailing chunks the consumer no
		// longer wants.
		if err := ctx.Err(); err != nil {
			return err
		}

		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}

		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if trimmed == "" {
				stop := c.parseSSEEvent(ctx, eventLines, ch)
				eventLines = eventLines[:0]
				if stop {
					return nil
				}
			} else {
				eventLines = append(eventLines, trimmed)
			}
		}

		if errors.Is(err, io.EOF) {
			if len(eventLines) > 0 {
				_ = c.parseSSEEvent(ctx, eventLines, ch)
			}
			return io.EOF
		}
	}
}

func (c *QwenClient) parseSSEEvent(ctx context.Context, lines []string, ch chan<- StreamChunk) bool {
	if len(lines) == 0 {
		return false
	}

	dataLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
	}
	if len(dataLines) == 0 {
		return false
	}

	send := func(sc StreamChunk) bool {
		select {
		case ch <- sc:
			return true
		case <-ctx.Done():
			return false
		}
	}

	dataStr := strings.Join(dataLines, "\n")
	if dataStr == "[DONE]" {
		send(StreamChunk{Done: true})
		return true
	}

	var chunk map[string]interface{}
	if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
		c.logger.Error("failed to parse chunk", zap.Error(err), zap.String("data", dataStr[:min(200, len(dataStr))]))
		return false
	}

	content, done := extractChunkContent(chunk)
	extraURLs := extractMediaURLsFromChunk(chunk)
	if content != "" || done || len(extraURLs) > 0 {
		send(StreamChunk{Content: content, Done: done, ExtraURLs: extraURLs})
	}
	return done
}

func extractChunkContent(chunk map[string]interface{}) (string, bool) {
	content := ""
	hasChoices := false
	if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			hasChoices = true
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				if c, ok := delta["content"].(string); ok {
					content = c
				}
			}
			if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" {
				return content, true
			}
		}
	}

	// Qwen also emits "phase" events at top level with content + extra (used heavily
	// for t2v/t2i). Only consult these when there are no choices — mirrors the
	// reference's `if choices ... elif phase` (mutually exclusive) so regular chat
	// streams don't double-append.
	if !hasChoices {
		if _, ok := chunk["phase"].(string); ok {
			if c, ok := chunk["content"].(string); ok {
				content += c
			} else if c, ok := chunk["text"].(string); ok {
				content += c
			}
		}
	}

	done := false
	if d, ok := chunk["done"].(bool); ok {
		done = d
	}
	return content, done
}

// extractMediaURLsFromChunk pulls media URLs out of a chunk's `extra` payload (either
// inside `choices[0].delta.extra` or a top-level `extra` for phase events). Mirrors
// the reference Python qwen_client._extract_urls_from_extra logic, including the
// mutually-exclusive choices-vs-phase routing.
func extractMediaURLsFromChunk(chunk map[string]interface{}) []string {
	var urls []string
	hasChoices := false
	if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			hasChoices = true
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				if extra, ok := delta["extra"].(map[string]interface{}); ok {
					urls = append(urls, extractURLsFromExtra(extra)...)
				}
			}
		}
	}
	if !hasChoices {
		if extra, ok := chunk["extra"].(map[string]interface{}); ok {
			urls = append(urls, extractURLsFromExtra(extra)...)
		}
	}
	return urls
}

func extractURLsFromExtra(extra map[string]interface{}) []string {
	var urls []string
	collect := func(v interface{}) {
		if s, ok := v.(string); ok && strings.HasPrefix(s, "http") {
			urls = append(urls, s)
		}
	}

	// wanx.image_list / wanx.video_list
	if wanx, ok := extra["wanx"].(map[string]interface{}); ok {
		for _, listKey := range []string{"image_list", "video_list"} {
			if list, ok := wanx[listKey].([]interface{}); ok {
				for _, item := range list {
					if m, ok := item.(map[string]interface{}); ok {
						for _, k := range []string{"url", "image_url", "video_url"} {
							collect(m[k])
						}
					}
				}
			}
		}
	}

	// tool_result[].image / .url / .video
	if tr, ok := extra["tool_result"].([]interface{}); ok {
		for _, item := range tr {
			if m, ok := item.(map[string]interface{}); ok {
				for _, k := range []string{"image", "video", "url", "src", "imageUrl", "image_url", "videoUrl", "video_url"} {
					collect(m[k])
				}
			} else {
				collect(item)
			}
		}
	}

	// Flat scalar fields
	for _, k := range []string{
		"image_url", "imageUrl", "wanx_image_url",
		"video_url", "videoUrl", "wanx_video_url",
		"url", "image", "video",
	} {
		collect(extra[k])
	}

	// List fields with either strings or {url:...} items
	for _, k := range []string{"image_urls", "imageUrls", "images", "video_urls", "videoUrls", "videos"} {
		if list, ok := extra[k].([]interface{}); ok {
			for _, item := range list {
				if s, ok := item.(string); ok {
					collect(s)
					continue
				}
				if m, ok := item.(map[string]interface{}); ok {
					for _, sub := range []string{"url", "src", "image", "imageUrl", "image_url", "video", "videoUrl", "video_url"} {
						collect(m[sub])
					}
				}
			}
		}
	}
	return urls
}

func (c *QwenClient) GenerateImage(ctx context.Context, token, prompt string, buildPayload func(chatID, model, prompt string) map[string]interface{}, model string) (string, error) {
	chatID, err := c.CreateChat(ctx, token, model, "t2t")
	if err != nil {
		return "", err
	}
	defer c.DeleteChat(context.Background(), token, chatID)

	payload := buildPayload(chatID, model, prompt)

	chunks, err := c.StreamChat(ctx, token, chatID, payload)
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

	content := fullContent.String()

	start := strings.Index(content, "https://")
	if start == -1 {
		return "", fmt.Errorf("no image URL in response: %s", content[:min(200, len(content))])
	}

	end := start
	for end < len(content) && content[end] != '"' && content[end] != '\'' && content[end] != ' ' && content[end] != '\n' {
		end++
	}

	return content[start:end], nil
}

// VideoGenResult carries one attempt's worth of T2V output. URLs may come from
// either Qwen's markdown/content (parsed via regex) or directly from the SSE
// `extra` field. RawBody is the concatenated content stream — used by callers
// to classify upstream errors (rate-limit, "not supported", etc.).
type VideoGenResult struct {
	URLs    []string
	RawBody string
}

func (c *QwenClient) GenerateVideo(ctx context.Context, token, prompt string, buildPayload func(chatID, model, prompt, aspectRatio string) map[string]interface{}, model, aspectRatio string) (*VideoGenResult, error) {
	chatID, err := c.CreateChat(ctx, token, model, "t2v")
	if err != nil {
		return nil, err
	}
	defer c.DeleteChat(context.Background(), token, chatID)

	payload := buildPayload(chatID, model, prompt, aspectRatio)

	chunks, err := c.StreamChat(ctx, token, chatID, payload)
	if err != nil {
		return nil, err
	}

	var fullContent strings.Builder
	var extraURLs []string
	for chunk := range chunks {
		if chunk.Error != "" {
			return nil, errors.New(chunk.Error)
		}
		fullContent.WriteString(chunk.Content)
		extraURLs = append(extraURLs, chunk.ExtraURLs...)
		if chunk.Done {
			break
		}
	}

	content := fullContent.String()
	urls := dedupStrings(append(extraURLs, extractAllVideoURLs(content)...))

	result := &VideoGenResult{URLs: urls, RawBody: content}
	return result, nil
}

func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func extractAllVideoURLs(text string) []string {
	patterns := []string{
		`\[(?:video|img)\]\((https?://[^\s\)]+)\)`,
		`!\[.*?\]\((https?://[^\s\)]+\.(?:mp4|webm|mov)[^\s\)]*)\)`,
		`"(?:url|video|src|videoUrl|video_url)"\s*:\s*"(https?://[^"]+)"`,
	}
	bare := `https?://(?:cdn\.qwenlm\.ai|wanx\.alicdn\.com|[^\s"<>]+\.(?:mp4|webm|mov))[^\s"<>]*`
	var urls []string
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) > 1 {
				urls = append(urls, strings.TrimRight(m[1], `).,;"'>`))
			}
		}
	}
	re := regexp.MustCompile(bare)
	for _, m := range re.FindAllString(text, -1) {
		urls = append(urls, strings.TrimRight(m, `).,;"'>`))
	}
	return urls
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
