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

// OpenAI Responses API facade. Translates {input, instructions, model,
// stream} → ChatRequest, re-shapes to a single output_text item in the
// Responses envelope.

type responsesRequest struct {
	Model        string          `json:"model"`
	Input        json.RawMessage `json:"input"`
	Instructions string          `json:"instructions"`
	Stream       bool            `json:"stream"`
}

type responsesInputItem struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func RegisterResponsesRoutes(r *gin.Engine) {
	g := r.Group("/")
	g.Use(APIKeyMiddleware())
	g.POST("/v1/responses", handleResponses)
}

func handleResponses(c *gin.Context) {
	start := time.Now()
	rawBody := readRawBody(c)
	mutated, saverStats := applySaver(rawBody, formatOpenAIResponses)
	rawBody = mutated
	c.Request.Body = io.NopCloser(bytes.NewReader(mutated))

	result := chatResult{HTTPStatus: http.StatusOK}
	model := ""
	provName := ""
	defer func() {
		recordRequestLog("responses", model, provName, rawBody, result.HTTPStatus, result.ErrorMsg, result.PromptTokens, result.CompletionTokens, saverStats, start)
	}()

	var req responsesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		result = chatResult{HTTPStatus: http.StatusBadRequest, ErrorMsg: err.Error()}
		return
	}
	model = req.Model

	messages := make([]models.ChatMessage, 0, 4)
	if req.Instructions != "" {
		messages = append(messages, models.ChatMessage{Role: "system", Content: req.Instructions})
	}

	// `input` can be a string, an array of strings, or an array of
	// {role,content} items. Try each shape; fall back to flatten.
	var asString string
	if err := json.Unmarshal(req.Input, &asString); err == nil {
		messages = append(messages, models.ChatMessage{Role: "user", Content: asString})
	} else {
		var items []responsesInputItem
		if err := json.Unmarshal(req.Input, &items); err == nil && len(items) > 0 {
			for _, it := range items {
				role := it.Role
				if role == "" {
					role = "user"
				}
				messages = append(messages, models.ChatMessage{Role: role, Content: flattenContent(it.Content)})
			}
		} else {
			var strList []string
			if err := json.Unmarshal(req.Input, &strList); err == nil {
				for _, s := range strList {
					messages = append(messages, models.ChatMessage{Role: "user", Content: s})
				}
			}
		}
	}
	if len(messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "empty input", "type": "invalid_request_error"}})
		result = chatResult{HTTPStatus: http.StatusBadRequest, ErrorMsg: "empty input"}
		return
	}

	chatReq := &models.ChatRequest{Model: req.Model, Messages: messages, Stream: req.Stream}
	provider, resolved, err := routeForFacade(chatReq.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		result = chatResult{HTTPStatus: http.StatusBadRequest, ErrorMsg: err.Error()}
		return
	}
	chatReq.Model = resolved
	model = resolved
	provName = provider.Name()

	respID := "resp_" + randomString(16)

	if req.Stream {
		result = streamResponses(c, provider, chatReq, respID, req.Model)
		return
	}

	resp, err := provider.ChatCompletion(c.Request.Context(), chatReq)
	if err != nil {
		RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: chatReq.Model, Feature: "responses", Success: false})
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error(), "type": "api_error"}})
		result = chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: err.Error()}
		return
	}
	text := ""
	if len(resp.Choices) > 0 && resp.Choices[0].Message != nil {
		text = resp.Choices[0].Message.Content
	}
	prompt, completion := 0, 0
	if resp.Usage != nil {
		prompt = resp.Usage.PromptTokens
		completion = resp.Usage.CompletionTokens
	}
	RecordUsage(core.UsageRecord{
		Timestamp: time.Now().Unix(), Model: chatReq.Model, Feature: "responses",
		PromptTokens: prompt, CompletionTokens: completion, Success: true,
	})

	c.JSON(http.StatusOK, gin.H{
		"id":          respID,
		"object":      "response",
		"created_at":  time.Now().Unix(),
		"model":       req.Model,
		"status":      "completed",
		"output_text": text,
		"output": []gin.H{{
			"type":    "message",
			"id":      "msg_" + randomString(12),
			"status":  "completed",
			"role":    "assistant",
			"content": []gin.H{{"type": "output_text", "text": text, "annotations": []interface{}{}}},
		}},
		"usage": gin.H{
			"input_tokens":  prompt,
			"output_tokens": completion,
			"total_tokens":  prompt + completion,
		},
	})
	result = chatResult{PromptTokens: prompt, CompletionTokens: completion, Success: true, HTTPStatus: http.StatusOK}
}

func streamResponses(c *gin.Context, provider models.Provider, req *models.ChatRequest, respID, displayModel string) chatResult {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ctx := c.Request.Context()
	chunks, err := provider.ChatStream(ctx, req)
	if err != nil {
		RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "responses", Success: false})
		fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", mustMarshal(gin.H{"type": "error", "message": err.Error()}))
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

	emit("response.created", gin.H{
		"type": "response.created",
		"response": gin.H{
			"id": respID, "object": "response", "model": displayModel,
			"status": "in_progress",
		},
	})
	itemID := "msg_" + randomString(12)
	emit("response.output_item.added", gin.H{
		"type": "response.output_item.added", "output_index": 0,
		"item": gin.H{"id": itemID, "type": "message", "role": "assistant", "status": "in_progress"},
	})

	var full strings.Builder
	for {
		select {
		case <-ctx.Done():
			return chatResult{HTTPStatus: 0, ErrorMsg: "client disconnected"}
		case chunk, ok := <-chunks:
			if !ok {
				emit("response.output_item.done", gin.H{"type": "response.output_item.done", "output_index": 0})
				emit("response.completed", gin.H{
					"type": "response.completed",
					"response": gin.H{
						"id": respID, "object": "response", "model": displayModel,
						"status":      "completed",
						"output_text": full.String(),
					},
				})
				completion := full.Len() / 4
				RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "responses", CompletionTokens: completion, Success: true})
				return chatResult{CompletionTokens: completion, Success: true, HTTPStatus: http.StatusOK}
			}
			if chunk.Error != "" {
				emit("error", gin.H{"type": "error", "message": chunk.Error})
				RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "responses", Success: false})
				return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: chunk.Error}
			}
			if chunk.Content != "" {
				full.WriteString(chunk.Content)
				emit("response.output_text.delta", gin.H{
					"type": "response.output_text.delta", "output_index": 0,
					"item_id": itemID, "delta": chunk.Content,
				})
			}
			if chunk.Done {
				emit("response.output_text.done", gin.H{
					"type": "response.output_text.done", "output_index": 0,
					"item_id": itemID, "text": full.String(),
				})
				emit("response.output_item.done", gin.H{"type": "response.output_item.done", "output_index": 0})
				emit("response.completed", gin.H{
					"type": "response.completed",
					"response": gin.H{
						"id": respID, "object": "response", "model": displayModel,
						"status":      "completed",
						"output_text": full.String(),
					},
				})
				completion := full.Len() / 4
				RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "responses", CompletionTokens: completion, Success: true})
				return chatResult{CompletionTokens: completion, Success: true, HTTPStatus: http.StatusOK}
			}
		}
	}
}
