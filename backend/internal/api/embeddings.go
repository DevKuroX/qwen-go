package api

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
)

// OpenAI-style embeddings facade. Qwen-go itself doesn't expose an embedding
// endpoint; per BACKLOG #20 we ship a deterministic emulation so client code
// that asks for embeddings doesn't 404. Vectors are SHA-256-derived, normalised
// to unit length, dimension default 1536 — enough for cosine-similarity sanity
// checks against itself but NOT semantically meaningful. Document this clearly
// to operators if/when real embeddings get wired in.

type embeddingsRequest struct {
	Model      string          `json:"model"`
	Input      json.RawMessage `json:"input"`
	Dimensions int             `json:"dimensions"`
}

func RegisterEmbeddingsRoutes(r *gin.Engine) {
	g := r.Group("/")
	g.Use(APIKeyMiddleware())
	g.POST("/v1/embeddings", handleEmbeddings)
	g.POST("/embeddings", handleEmbeddings)
}

func handleEmbeddings(c *gin.Context) {
	var req embeddingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error(), "type": "invalid_request_error"}})
		return
	}

	// Input shape: string | []string. Detect either.
	var inputs []string
	var single string
	if err := json.Unmarshal(req.Input, &single); err == nil {
		inputs = []string{single}
	} else if err := json.Unmarshal(req.Input, &inputs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "input must be string or array of strings"}})
		return
	}

	dims := req.Dimensions
	if dims <= 0 || dims > 4096 {
		dims = 1536
	}

	data := make([]gin.H, 0, len(inputs))
	totalTokens := 0
	for i, text := range inputs {
		data = append(data, gin.H{
			"object":    "embedding",
			"embedding": deterministicEmbedding(text, dims),
			"index":     i,
		})
		totalTokens += approxTokenCount(text)
	}

	RecordUsage(core.UsageRecord{
		Timestamp:    time.Now().Unix(),
		Model:        req.Model,
		Feature:      "embeddings",
		PromptTokens: totalTokens,
		Success:      true,
	})

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
		"model":  req.Model,
		"usage": gin.H{
			"prompt_tokens": totalTokens,
			"total_tokens":  totalTokens,
		},
	})
}

// deterministicEmbedding generates a stable unit-length vector for `text`.
// It hashes the text repeatedly with a counter suffix to produce enough bytes
// for `dims` float32 values, maps each 4-byte block to a value in [-1, 1],
// then L2-normalises.
func deterministicEmbedding(text string, dims int) []float32 {
	out := make([]float32, dims)
	bytesNeeded := dims * 4
	buf := make([]byte, 0, bytesNeeded+32)
	for counter := 0; len(buf) < bytesNeeded; counter++ {
		var ctr [4]byte
		binary.BigEndian.PutUint32(ctr[:], uint32(counter))
		h := sha256.New()
		h.Write([]byte(text))
		h.Write(ctr[:])
		buf = append(buf, h.Sum(nil)...)
	}
	var sumSq float64
	for i := 0; i < dims; i++ {
		raw := binary.BigEndian.Uint32(buf[i*4 : i*4+4])
		// Map uint32 → [-1, 1].
		v := float64(raw)/float64(math.MaxUint32)*2.0 - 1.0
		out[i] = float32(v)
		sumSq += v * v
	}
	if sumSq > 0 {
		inv := 1.0 / math.Sqrt(sumSq)
		for i := range out {
			out[i] = float32(float64(out[i]) * inv)
		}
	}
	return out
}

func approxTokenCount(s string) int {
	if len(s) == 0 {
		return 0
	}
	return (len(s) + 3) / 4
}
