package api

import (
	"bytes"
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

// Anthropic Messages API facade. Translates {role, content, system,
// max_tokens, stream} → internal ChatRequest, then re-shapes the response
// into Anthropic's {id, type:"message", content:[{type:"text",text}],
// stop_reason, usage} envelope. Streaming emits the event types Anthropic's
// SDK expects: message_start → content_block_start → content_block_delta* →
// content_block_stop → message_delta → message_stop.

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	System    json.RawMessage    `json:"system"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream"`
}

func RegisterClaudeRoutes(r *gin.Engine) {
	g := r.Group("/")
	g.Use(APIKeyMiddleware())
	g.POST("/v1/messages", handleClaudeMessages)
	g.POST("/anthropic/v1/messages", handleClaudeMessages)
}

func handleClaudeMessages(c *gin.Context) {
	start := time.Now()
	rawBody := readRawBody(c)
	mutated, saverStats := applySaver(rawBody, formatAnthropic)
	rawBody = mutated
	c.Request.Body = io.NopCloser(bytes.NewReader(mutated))

	result := chatResult{HTTPStatus: http.StatusOK}
	model := ""
	provName := ""
	defer func() {
		recordRequestLog("claude", model, provName, rawBody, result.HTTPStatus, result.ErrorMsg, result.PromptTokens, result.CompletionTokens, saverStats, start)
	}()

	var req anthropicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": err.Error()}})
		result = chatResult{HTTPStatus: http.StatusBadRequest, ErrorMsg: err.Error()}
		return
	}
	model = req.Model

	chatReq := &models.ChatRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Stream:    req.Stream,
		Messages:  make([]models.ChatMessage, 0, len(req.Messages)+1),
	}
	if sys := flattenContent(req.System); sys != "" {
		chatReq.Messages = append(chatReq.Messages, models.ChatMessage{Role: "system", Content: sys})
	}
	for _, m := range req.Messages {
		chatReq.Messages = append(chatReq.Messages, models.ChatMessage{
			Role:    m.Role,
			Content: flattenContent(m.Content),
		})
	}

	provider, resolved, err := routeForFacade(chatReq.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": err.Error()}})
		result = chatResult{HTTPStatus: http.StatusBadRequest, ErrorMsg: err.Error()}
		return
	}
	chatReq.Model = resolved
	model = resolved
	provName = provider.Name()

	msgID := "msg_" + randomString(16)

	if req.Stream {
		result = streamClaude(c, provider, chatReq, msgID, req.Model)
		return
	}

	resp, err := provider.ChatCompletion(c.Request.Context(), chatReq)
	if err != nil {
		RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: chatReq.Model, Feature: "claude", Success: false})
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "api_error", "message": err.Error()}})
		result = chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: err.Error()}
		return
	}

	content := ""
	if len(resp.Choices) > 0 && resp.Choices[0].Message != nil {
		content = resp.Choices[0].Message.Content
	}
	inputTokens, outputTokens := 0, 0
	if resp.Usage != nil {
		inputTokens = resp.Usage.PromptTokens
		outputTokens = resp.Usage.CompletionTokens
	}
	RecordUsage(core.UsageRecord{
		Timestamp: time.Now().Unix(), Model: chatReq.Model, Feature: "claude",
		PromptTokens: inputTokens, CompletionTokens: outputTokens, Success: true,
	})

	c.JSON(http.StatusOK, gin.H{
		"id":            msgID,
		"type":          "message",
		"role":          "assistant",
		"content":       []gin.H{{"type": "text", "text": content}},
		"model":         req.Model,
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": gin.H{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	})
	result = chatResult{PromptTokens: inputTokens, CompletionTokens: outputTokens, Success: true, HTTPStatus: http.StatusOK}
}

func streamClaude(c *gin.Context, provider models.Provider, req *models.ChatRequest, msgID, displayModel string) chatResult {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ctx := c.Request.Context()
	chunks, err := provider.ChatStream(ctx, req)
	if err != nil {
		RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "claude", Success: false})
		fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", mustMarshal(gin.H{"type": "error", "error": gin.H{"type": "api_error", "message": err.Error()}}))
		return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: err.Error()}
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: "streaming not supported"}
	}

	emit := func(event string, payload interface{}) {
		fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, mustMarshal(payload))
		flusher.Flush()
	}

	emit("message_start", gin.H{
		"type": "message_start",
		"message": gin.H{
			"id": msgID, "type": "message", "role": "assistant",
			"content": []interface{}{}, "model": displayModel,
			"stop_reason": nil, "stop_sequence": nil,
			"usage": gin.H{"input_tokens": 0, "output_tokens": 0},
		},
	})
	emit("content_block_start", gin.H{
		"type": "content_block_start", "index": 0,
		"content_block": gin.H{"type": "text", "text": ""},
	})

	var full strings.Builder
	for {
		select {
		case <-ctx.Done():
			return chatResult{HTTPStatus: 0, ErrorMsg: "client disconnected"}
		case chunk, ok := <-chunks:
			if !ok {
				emit("content_block_stop", gin.H{"type": "content_block_stop", "index": 0})
				emit("message_delta", gin.H{
					"type":  "message_delta",
					"delta": gin.H{"stop_reason": "end_turn", "stop_sequence": nil},
					"usage": gin.H{"output_tokens": full.Len() / 4},
				})
				emit("message_stop", gin.H{"type": "message_stop"})
				completion := full.Len() / 4
				RecordUsage(core.UsageRecord{
					Timestamp: time.Now().Unix(), Model: req.Model, Feature: "claude",
					CompletionTokens: completion, Success: true,
				})
				return chatResult{CompletionTokens: completion, Success: true, HTTPStatus: http.StatusOK}
			}
			if chunk.Error != "" {
				emit("error", gin.H{"type": "error", "error": gin.H{"type": "api_error", "message": chunk.Error}})
				RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "claude", Success: false})
				return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: chunk.Error}
			}
			if chunk.Content != "" {
				full.WriteString(chunk.Content)
				emit("content_block_delta", gin.H{
					"type": "content_block_delta", "index": 0,
					"delta": gin.H{"type": "text_delta", "text": chunk.Content},
				})
			}
			if chunk.Done {
				emit("content_block_stop", gin.H{"type": "content_block_stop", "index": 0})
				emit("message_delta", gin.H{
					"type":  "message_delta",
					"delta": gin.H{"stop_reason": "end_turn", "stop_sequence": nil},
					"usage": gin.H{"output_tokens": full.Len() / 4},
				})
				emit("message_stop", gin.H{"type": "message_stop"})
				completion := full.Len() / 4
				RecordUsage(core.UsageRecord{
					Timestamp: time.Now().Unix(), Model: req.Model, Feature: "claude",
					CompletionTokens: completion, Success: true,
				})
				return chatResult{CompletionTokens: completion, Success: true, HTTPStatus: http.StatusOK}
			}
		}
	}
}
