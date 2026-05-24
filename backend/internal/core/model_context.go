package core

import (
	"strings"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

const (
	globalContextFallback   = 8192
	providerContextFallback = 32768
)

// ModelContextRegistry holds context-window sizes per model/provider. The
// registry survives the RTK/Caveman rework because we still want a single
// source of truth for "how many tokens does this model accept?" for future
// budget logic — even though the new pipeline doesn't gate on context %.
type ModelContextRegistry struct {
	overrides        map[string]int
	knownModels      map[string]int
	providerDefaults map[string]int
	globalFallback   int
}

// BudgetManager wraps the registry with a token estimator. Kept as a
// dependency of the API layer so future budget gates can be reintroduced
// without rewiring callers.
type BudgetManager struct {
	registry *ModelContextRegistry
}

func NewModelContextRegistry() *ModelContextRegistry {
	return &ModelContextRegistry{
		overrides: map[string]int{},
		knownModels: map[string]int{
			"qwen3.6-plus":        65536,
			"qwen3.6-max-preview": 131072,
			"qwen3.6-27b":         32768,
		},
		providerDefaults: map[string]int{
			"qwen": providerContextFallback,
		},
		globalFallback: globalContextFallback,
	}
}

func NewBudgetManager() *BudgetManager {
	return &BudgetManager{registry: NewModelContextRegistry()}
}

func (r *ModelContextRegistry) Resolve(model string) int {
	if v, ok := r.overrides[model]; ok {
		return v
	}
	if v, ok := r.knownModels[model]; ok {
		return v
	}
	provider := providerFromModel(model)
	if v, ok := r.providerDefaults[provider]; ok {
		return v
	}
	return r.globalFallback
}

func (r *ModelContextRegistry) SetOverride(model string, context int) {
	r.overrides[model] = context
}

func (m *BudgetManager) Registry() *ModelContextRegistry {
	return m.registry
}

func (m *BudgetManager) EstimateRequest(req *models.ChatRequest) int {
	total := 0
	for _, msg := range req.Messages {
		total += estimateTextTokens(msg.Role) + estimateTextTokens(msg.Content)
	}
	if req.MaxTokens > 0 {
		total += req.MaxTokens
	}
	return total
}

func providerFromModel(model string) string {
	if strings.HasPrefix(model, "qwen") || strings.HasPrefix(model, "qw/") {
		return "qwen"
	}
	return ""
}

func estimateTextTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(text)/4 + 1
}
