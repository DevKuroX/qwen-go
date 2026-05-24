package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

type QwenClient struct {
	httpClient *http.Client
	logger     *zap.Logger
	baseURL    string
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
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://chat.qwen.ai")
	req.Header.Set("Referer", "https://chat.qwen.ai/")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
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

func (c *QwenClient) CreateChat(ctx context.Context, token, model string) (string, error) {
	ts := time.Now().Unix()
	body := map[string]interface{}{
		"title":     fmt.Sprintf("api_%d", ts),
		"models":    []string{model},
		"chat_mode": "normal",
		"chat_type": "t2t",
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
}

func (c *QwenClient) StreamChat(ctx context.Context, token, chatID string, payload map[string]interface{}) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 100)

	go func() {
		defer close(ch)

		jsonBody, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v2/chats/"+chatID+"/completions", bytes.NewBuffer(jsonBody))
		if err != nil {
			ch <- StreamChunk{Error: err.Error()}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Origin", "https://chat.qwen.ai")
		req.Header.Set("Referer", "https://chat.qwen.ai/")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			ch <- StreamChunk{Error: err.Error()}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			ch <- StreamChunk{Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)[:min(500, len(body))])}
			return
		}

		buf := make([]byte, 8192)
		for {
			n, err := resp.Body.Read(buf)
			if err != nil {
				if err == io.EOF {
					ch <- StreamChunk{Done: true}
					break
				}
				ch <- StreamChunk{Error: err.Error()}
				return
			}

			data := string(buf[:n])
			lines := strings.Split(data, "\n")

			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || !strings.HasPrefix(line, "data:") {
					continue
				}

				dataStr := strings.TrimPrefix(line, "data:")
				dataStr = strings.TrimSpace(dataStr)

				if dataStr == "[DONE]" {
					ch <- StreamChunk{Done: true}
					return
				}

				var chunk map[string]interface{}
				if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
					continue
				}

				content := ""
				if c, ok := chunk["content"].(string); ok {
					content = c
				}

				done := false
				if d, ok := chunk["done"].(bool); ok {
					done = d
				}

				ch <- StreamChunk{
					Content: content,
					Done:    done,
				}

				if done {
					return
				}
			}
		}
	}()

	return ch, nil
}

func (c *QwenClient) GenerateImage(ctx context.Context, token, prompt string) (string, error) {
	chatID, err := c.CreateChat(ctx, token, "qwen-vl-plus")
	if err != nil {
		return "", err
	}
	defer c.DeleteChat(context.Background(), token, chatID)

	payload := map[string]interface{}{
		"chat_type": "t2i",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
