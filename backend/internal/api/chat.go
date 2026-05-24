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

var providerManager *core.ProviderManager
var budgetManager *core.BudgetManager

func InitChatHandler(pm *core.ProviderManager) {
	providerManager = pm
	if budgetManager == nil {
		budgetManager = core.NewBudgetManager()
	}
}

func RegisterChatRoutes(r *gin.Engine, pm *core.ProviderManager) {
	providerManager = pm
	chat := r.Group("/api")
	chat.Use(APIKeyMiddleware())

	chat.POST("/v1/chat/completions", handleChatCompletions)
	chat.POST("/chat/completions", handleChatCompletions)
	chat.POST("/completions", handleChatCompletions)
}

// chatResult bubbles up from sub-handlers so the parent can write one
// consolidated request_log entry.
type chatResult struct {
	PromptTokens     int
	CompletionTokens int
	Success          bool
	ErrorMsg         string
	HTTPStatus       int
}

func readRawBody(c *gin.Context) []byte {
	if c.Request.Body == nil {
		return nil
	}
	b, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(b))
	return b
}

func recordRequestLog(feature, model, providerName string, body []byte, status int, errMsg string, promptToks, completionToks int, saverStats core.RequestSaverStats, start time.Time) {
	if core.GlobalRequestLogTracker == nil {
		return
	}
	statusStr := "ok"
	if status >= 400 || errMsg != "" {
		statusStr = "error"
	}
	rl := core.RequestLog{
		Timestamp:        start.Unix(),
		Model:            model,
		Provider:         providerName,
		Feature:          feature,
		LatencyMs:        int(time.Since(start).Milliseconds()),
		Status:           statusStr,
		HTTPStatus:       status,
		ErrorMessage:     errMsg,
		RequestBody:      string(body),
		PromptTokens:     promptToks,
		CompletionTokens: completionToks,
	}
	if mode := saverStats.CompactionMode(); mode != "" {
		rl.CompactionMode = mode
		rl.SaverReductionPct = saverStats.RTKReductionPct()
		rl.RtkFilters = saverStats.RTKFiltersCSV()
		if saverStats.RTK != nil {
			rl.RtkBytesBefore = saverStats.RTK.BytesBefore
			rl.RtkBytesAfter = saverStats.RTK.BytesAfter
		}
		if saverStats.Caveman.Enabled {
			rl.CavemanLevel = saverStats.Caveman.Level
		}
	}
	core.GlobalRequestLogTracker.RecordRequest(rl)
}

func handleChatCompletions(c *gin.Context) {
	start := time.Now()
	rawBody := readRawBody(c)

	// 9router parity: RTK + Caveman run on the raw body before any other
	// parsing, so they see the original content arrays.
	mutated, saverStats := applySaver(rawBody, formatOpenAIChat)
	rawBody = mutated

	var req models.ChatRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		recordRequestLog("chat", "", "", rawBody, http.StatusBadRequest, err.Error(), 0, 0, saverStats, start)
		return
	}

	provider, resolvedModel, err := providerManager.Route(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "invalid_request_error",
			},
		})
		recordRequestLog("chat", req.Model, "", rawBody, http.StatusBadRequest, err.Error(), 0, 0, saverStats, start)
		return
	}

	req.Model = resolvedModel
	providerName := provider.Name()

	var result chatResult
	if req.Stream {
		result = handleStreamResponse(c, provider, &req)
	} else {
		result = handleNonStreamResponse(c, provider, &req)
	}

	recordRequestLog("chat", req.Model, providerName, rawBody, result.HTTPStatus, result.ErrorMsg, result.PromptTokens, result.CompletionTokens, saverStats, start)
}

func handleNonStreamResponse(c *gin.Context, provider models.Provider, req *models.ChatRequest) chatResult {
	resp, err := provider.ChatCompletion(c.Request.Context(), req)
	if err != nil {
		RecordUsage(core.UsageRecord{
			Timestamp: time.Now().Unix(),
			Model:     req.Model,
			Feature:   "chat",
			Success:   false,
		})
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "server_error",
			},
		})
		return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: err.Error()}
	}

	promptToks := 0
	completionToks := 0
	if resp.Usage != nil {
		promptToks = resp.Usage.PromptTokens
		completionToks = resp.Usage.CompletionTokens
	}

	RecordUsage(core.UsageRecord{
		Timestamp:        time.Now().Unix(),
		Model:            req.Model,
		Feature:          "chat",
		PromptTokens:     promptToks,
		CompletionTokens: completionToks,
		Success:          true,
	})

	c.JSON(http.StatusOK, resp)
	return chatResult{
		PromptTokens:     promptToks,
		CompletionTokens: completionToks,
		Success:          true,
		HTTPStatus:       http.StatusOK,
	}
}

func handleStreamResponse(c *gin.Context, provider models.Provider, req *models.ChatRequest) chatResult {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ctx := c.Request.Context()
	chunks, err := provider.ChatStream(ctx, req)
	if err != nil {
		RecordUsage(core.UsageRecord{
			Timestamp: time.Now().Unix(),
			Model:     req.Model,
			Feature:   "chat",
			Success:   false,
		})
		fmt.Fprintf(c.Writer, "data: %s\n\n", mustMarshal(gin.H{"error": err.Error()}))
		return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: err.Error()}
	}

	completionID := fmt.Sprintf("chatcmpl-%s", randomString(12))
	created := time.Now().Unix()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		RecordUsage(core.UsageRecord{
			Timestamp: time.Now().Unix(),
			Model:     req.Model,
			Feature:   "chat",
			Success:   false,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: "streaming not supported"}
	}

	var fullContent strings.Builder
	for {
		select {
		case <-ctx.Done():
			return chatResult{HTTPStatus: 0, ErrorMsg: "client disconnected"}
		case chunk, ok := <-chunks:
			if !ok {
				promptToks := len(req.Messages) * 10
				completionToks := fullContent.Len() / 4
				RecordUsage(core.UsageRecord{
					Timestamp:        time.Now().Unix(),
					Model:            req.Model,
					Feature:          "chat",
					PromptTokens:     promptToks,
					CompletionTokens: completionToks,
					Success:          true,
				})
				return chatResult{
					PromptTokens:     promptToks,
					CompletionTokens: completionToks,
					Success:          true,
					HTTPStatus:       http.StatusOK,
				}
			}
			if chunk.Error != "" {
				RecordUsage(core.UsageRecord{
					Timestamp: time.Now().Unix(),
					Model:     req.Model,
					Feature:   "chat",
					Success:   false,
				})
				fmt.Fprintf(c.Writer, "data: %s\n\n", mustMarshal(gin.H{"error": chunk.Error}))
				flusher.Flush()
				return chatResult{HTTPStatus: http.StatusInternalServerError, ErrorMsg: chunk.Error}
			}

			fullContent.WriteString(chunk.Content)

			data := map[string]interface{}{
				"id":      completionID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   req.Model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]string{
							"content": chunk.Content,
						},
						"finish_reason": nil,
					},
				},
			}

			if chunk.Done {
				data["choices"].([]map[string]interface{})[0]["finish_reason"] = "stop"
			}

			fmt.Fprintf(c.Writer, "data: %s\n\n", mustMarshal(data))
			flusher.Flush()

			if chunk.Done {
				fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
				flusher.Flush()

				promptToks := len(req.Messages) * 10
				completionToks := fullContent.Len() / 4
				RecordUsage(core.UsageRecord{
					Timestamp:        time.Now().Unix(),
					Model:            req.Model,
					Feature:          "chat",
					PromptTokens:     promptToks,
					CompletionTokens: completionToks,
					Success:          true,
				})
				return chatResult{
					PromptTokens:     promptToks,
					CompletionTokens: completionToks,
					Success:          true,
					HTTPStatus:       http.StatusOK,
				}
			}
		}
	}
}

func RegisterImageRoutes(r *gin.Engine, pm *core.ProviderManager) {
	providerManager = pm
	img := r.Group("/api/v1")
	img.Use(APIKeyMiddleware())

	img.POST("/images/generations", handleImageGeneration)
	img.POST("/videos/generations", handleVideoGeneration)
}

func handleImageGeneration(c *gin.Context) {
	start := time.Now()
	rawBody := readRawBody(c)

	var req models.ImageRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		recordRequestLog("t2i", "", "", rawBody, http.StatusBadRequest, err.Error(), 0, 0, core.RequestSaverStats{}, start)
		return
	}

	if req.Model == "" {
		req.Model = "qw/qwen-vl-plus"
	}

	provider, _, err := providerManager.Route(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		recordRequestLog("t2i", req.Model, "", rawBody, http.StatusBadRequest, err.Error(), 0, 0, core.RequestSaverStats{}, start)
		return
	}
	providerName := provider.Name()

	resp, err := provider.ImageGeneration(c.Request.Context(), &req)
	if err != nil {
		RecordUsage(core.UsageRecord{
			Timestamp: time.Now().Unix(),
			Model:     req.Model,
			Feature:   "t2i",
			Success:   false,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		recordRequestLog("t2i", req.Model, providerName, rawBody, http.StatusInternalServerError, err.Error(), 0, 0, core.RequestSaverStats{}, start)
		return
	}

	RecordUsage(core.UsageRecord{
		Timestamp: time.Now().Unix(),
		Model:     req.Model,
		Feature:   "t2i",
		Success:   true,
	})

	c.JSON(http.StatusOK, resp)
	recordRequestLog("t2i", req.Model, providerName, rawBody, http.StatusOK, "", 0, 0, core.RequestSaverStats{}, start)
}

func handleVideoGeneration(c *gin.Context) {
	start := time.Now()
	rawBody := readRawBody(c)

	var req models.VideoRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		recordRequestLog("t2v", "", "", rawBody, http.StatusBadRequest, err.Error(), 0, 0, core.RequestSaverStats{}, start)
		return
	}

	if req.Prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "prompt is required"})
		recordRequestLog("t2v", req.Model, "", rawBody, http.StatusBadRequest, "prompt is required", 0, 0, core.RequestSaverStats{}, start)
		return
	}
	if req.Model == "" {
		req.Model = "qw/qwen-video"
	}
	if req.N <= 0 {
		req.N = 1
	}
	if req.N > 2 {
		req.N = 2
	}

	provider, resolvedModel, err := providerManager.Route(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		recordRequestLog("t2v", req.Model, "", rawBody, http.StatusBadRequest, err.Error(), 0, 0, core.RequestSaverStats{}, start)
		return
	}
	req.Model = resolvedModel
	providerName := provider.Name()

	resp, err := provider.VideoGeneration(c.Request.Context(), &req)
	if err != nil {
		RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "t2v", Success: false})
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		recordRequestLog("t2v", req.Model, providerName, rawBody, http.StatusInternalServerError, err.Error(), 0, 0, core.RequestSaverStats{}, start)
		return
	}

	RecordUsage(core.UsageRecord{Timestamp: time.Now().Unix(), Model: req.Model, Feature: "t2v", Success: true})
	c.JSON(http.StatusOK, resp)
	recordRequestLog("t2v", req.Model, providerName, rawBody, http.StatusOK, "", 0, 0, core.RequestSaverStats{}, start)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().Nanosecond()%len(letters)]
	}
	return string(b)
}

func mustMarshal(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
