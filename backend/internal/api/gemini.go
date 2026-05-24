package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

// Google Gemini generateContent / streamGenerateContent facade.

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction"`
	GenerationConfig  *struct {
		Temperature     float64 `json:"temperature"`
		MaxOutputTokens int     `json:"maxOutputTokens"`
	} `json:"generationConfig"`
}

func RegisterGeminiRoutes(r *gin.Engine) {
	g := r.Group("/")
	g.Use(APIKeyMiddleware())
	// Gin can't route a literal colon in a path segment; use a wildcard and
	// split the action off the model in-handler. Covers both v1 and v1beta.
	g.POST("/v1beta/models/*tail", handleGemini)
	g.POST("/v1/models/*tail", handleGemini)
}

func handleGemini(c *gin.Context) {
	start := time.Now()
	rawBody := readRawBody(c)
	mutated, saverStats := applySaver(rawBody, formatGemini)
	rawBody = mutated
	c.Request.Body = io.NopCloser(bytes.NewReader(mutated))

	result := chatResult{HTTPStatus: http.StatusOK}
	model := ""
	provName := ""
	defer func() {
		recordRequestLog("gemini", model, provName, rawBody, result.HTTPStatus, result.ErrorMsg, result.PromptTokens, result.CompletionTokens, saverStats, start)
	}()

	tail := strings.TrimPrefix(c.Param("tail"), "/")
	colon := strings.LastIndex(tail, ":")
	if colon < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "expected models/<model>:<action>"}})
		result = chatResult{HTTPStatus: http.StatusBadRequest, ErrorMsg: "expected models/<model>:<action>"}
		return
	}
	modelName := tail[:colon]
	action := tail[colon+1:]
	stream := action == "streamGenerateContent"
	model = modelName

	var req geminiRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		result = chatResult{HTTPStatus: http.StatusBadRequest, ErrorMsg: err.Error()}
		return
	}

	chatReq := &models.ChatRequest{Model: modelName, Stream: stream, Messages: make([]models.ChatMessage, 0, len(req.Contents)+1)}
	if req.GenerationConfig != nil {
		chatReq.Temperature = req.GenerationConfig.Temperature
		chatReq.MaxTokens = req.GenerationConfig.MaxOutputTokens
	}
	if req.SystemInstruction != nil {
		chatReq.Messages = append(chatReq.Messages, models.ChatMessage{Role: "system", Content: joinParts(req.SystemInstruction.Parts)})
	}
	for _, content := range req.Contents {
		role := content.Role
		if role == "model" {
			role = "assistant"
		}
		if role == "" {
			role = "user"
		}
		chatReq.Messages = append(chatReq.Messages, models.ChatMessage{Role: role, Content: joinParts(content.Parts)})
	}

	provider, resolved, err := routeForFacade(modelName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		result = chatResult{HTTPStatus: http.StatusBadRequest, ErrorMsg: err.Error()}
		return
	}
	chatReq.Model = resolved
	model = resolved
	provName = provider.Name()

	if stream {
		result = streamGemini(c, provider, chatReq)
		return
	}

	resp, err := provider.ChatCompletion(c.Request.Context(), chatReq)
	if err != nil {
		RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: chatReq.Model, Feature: "gemini", Success: false})
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
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
		Timestamp: time.Now().Unix(), Model: chatReq.Model, Feature: "gemini",
		PromptTokens: prompt, CompletionTokens: completion, Success: true,
	})
	c.JSON(http.StatusOK, geminiCandidatePayload(text, "STOP", prompt, completion))
	result = chatResult{PromptTokens: prompt, CompletionTokens: completion, Success: true, HTTPStatus: http.StatusOK}
}

func streamGemini(c *gin.Context, provider models.Provider, req *models.ChatRequest) chatResult {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ctx := c.Request.Context()
	chunks, err := provider.ChatStream(ctx, req)
	if err != nil {
		RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "gemini", Success: false})
		fmt.Fprintf(c.Writer, "data: %s\n\n", mustMarshal(gin.H{"error": gin.H{"message": err.Error()}}))
		return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: err.Error()}
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: "streaming not supported"}
	}

	var full strings.Builder
	for {
		select {
		case <-ctx.Done():
			return chatResult{HTTPStatus: 0, ErrorMsg: "client disconnected"}
		case chunk, ok := <-chunks:
			if !ok {
				fmt.Fprintf(c.Writer, "data: %s\n\n", mustMarshal(geminiCandidatePayload("", "STOP", 0, full.Len()/4)))
				flusher.Flush()
				completion := full.Len() / 4
				RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "gemini", CompletionTokens: completion, Success: true})
				return chatResult{CompletionTokens: completion, Success: true, HTTPStatus: http.StatusOK}
			}
			if chunk.Error != "" {
				fmt.Fprintf(c.Writer, "data: %s\n\n", mustMarshal(gin.H{"error": gin.H{"message": chunk.Error}}))
				flusher.Flush()
				RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "gemini", Success: false})
				return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: chunk.Error}
			}
			if chunk.Content != "" {
				full.WriteString(chunk.Content)
				fmt.Fprintf(c.Writer, "data: %s\n\n", mustMarshal(gin.H{
					"candidates": []gin.H{{
						"content": gin.H{"role": "model", "parts": []gin.H{{"text": chunk.Content}}},
						"index":   0,
					}},
				}))
				flusher.Flush()
			}
			if chunk.Done {
				fmt.Fprintf(c.Writer, "data: %s\n\n", mustMarshal(geminiCandidatePayload("", "STOP", 0, full.Len()/4)))
				flusher.Flush()
				completion := full.Len() / 4
				RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "gemini", CompletionTokens: completion, Success: true})
				return chatResult{CompletionTokens: completion, Success: true, HTTPStatus: http.StatusOK}
			}
		}
	}
}

func joinParts(parts []geminiPart) string {
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	return sb.String()
}

func geminiCandidatePayload(text, finish string, promptTokens, completionTokens int) interface{} {
	payload := map[string]interface{}{
		"candidates": []map[string]interface{}{{
			"content": map[string]interface{}{
				"role":  "model",
				"parts": []map[string]string{{"text": text}},
			},
			"finishReason": finish,
			"index":        0,
		}},
		"usageMetadata": map[string]int{
			"promptTokenCount":     promptTokens,
			"candidatesTokenCount": completionTokens,
			"totalTokenCount":      promptTokens + completionTokens,
		},
	}
	return payload
}
