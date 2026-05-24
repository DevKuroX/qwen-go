package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/qwenpi/qwenpi-go/internal/core"
)

// OpenAI-spec /v1/models. Aggregates every provider's AvailableModels +
// Models map from the registry, plus a live-fetched, cached snapshot of
// opencode-zen's free catalog. Returned IDs include the provider prefix
// (e.g. "zen/deepseek-v4-flash-free") so clients know what to send to
// /v1/chat/completions.

type modelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

var (
	openCodeFreeCache       []string
	openCodeFreeCacheExpiry time.Time
	openCodeFreeCacheMu     sync.Mutex
	openCodeFreeFetcher     = &http.Client{Timeout: 5 * time.Second}
)

const openCodeFreeCacheTTL = 5 * time.Minute

func RegisterModelsRoutes(r *gin.Engine) {
	g := r.Group("/")
	g.Use(APIKeyMiddleware())
	g.GET("/api/v1/models", handleListModels)
	g.GET("/v1/models", handleListModels)
}

func handleListModels(c *gin.Context) {
	now := time.Now().Unix()
	seen := make(map[string]struct{}, 32)
	out := make([]modelEntry, 0, 32)

	if core.GlobalProviderRegistry == nil {
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": out})
		return
	}

	for _, cfg := range core.GlobalProviderRegistry.List() {
		prefix := providerPrefix(cfg.Name)

		add := func(id string) {
			if id == "" {
				return
			}
			full := prefix + id
			if _, dup := seen[full]; dup {
				return
			}
			seen[full] = struct{}{}
			out = append(out, modelEntry{ID: full, Object: "model", Created: now, OwnedBy: cfg.Name})
		}

		for _, id := range cfg.AvailableModels {
			add(id)
		}
		for id := range cfg.Models {
			add(id)
		}
		for id := range cfg.ModelAliases {
			add(id)
		}

		if cfg.Name == "opencode-zen" {
			for _, id := range fetchOpenCodeFreeModels(c.Request.Context()) {
				add(id)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": out})
}

func providerPrefix(name string) string {
	switch name {
	case "qwen":
		return "qw/"
	case "opencode-zen":
		return "zen/"
	case "gemini-web":
		return "gw/"
	case "kiro":
		return "kiro/"
	case "openai":
		return "openai/"
	case "anthropic":
		return "cl/"
	}
	return name + "/"
}

func fetchOpenCodeFreeModels(ctx context.Context) []string {
	openCodeFreeCacheMu.Lock()
	defer openCodeFreeCacheMu.Unlock()

	if time.Now().Before(openCodeFreeCacheExpiry) && len(openCodeFreeCache) > 0 {
		return openCodeFreeCache
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://opencode.ai/zen/v1/models", nil)
	if err != nil {
		return openCodeFreeCache
	}
	resp, err := openCodeFreeFetcher.Do(req)
	if err != nil {
		return openCodeFreeCache
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return openCodeFreeCache
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return openCodeFreeCache
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return openCodeFreeCache
	}

	free := make([]string, 0, 4)
	for _, m := range parsed.Data {
		if strings.HasSuffix(m.ID, "-free") {
			free = append(free, m.ID)
		}
	}
	if len(free) > 0 {
		openCodeFreeCache = free
		openCodeFreeCacheExpiry = time.Now().Add(openCodeFreeCacheTTL)
	}
	return openCodeFreeCache
}
