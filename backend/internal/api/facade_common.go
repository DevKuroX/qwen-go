package api

import (
	"encoding/json"
	"strings"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

// routeForFacade resolves a foreign-model request. Models containing "/" go
// through the normal provider router; bare names (e.g. "claude-3.5-sonnet",
// "gemini-2.0-flash") fall back to the qwen default so foreign SDK clients
// transparently target the gateway's primary backend.
func routeForFacade(model string) (models.Provider, string, error) {
	if model != "" && strings.Contains(model, "/") {
		return providerManager.Route(model)
	}
	return providerManager.Route("qw/qwen-max")
}

// flattenContent collapses a JSON value that may be either a plain string or
// a list of {type,text} blocks (Anthropic/OpenAI multimodal shape) into a
// flat string. Non-text parts are dropped — facade is scoped to text-only
// passthrough per BACKLOG guardrails.
func flattenContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(raw, &arr); err == nil {
		var sb strings.Builder
		for _, b := range arr {
			if t, ok := b["text"].(string); ok {
				sb.WriteString(t)
			}
		}
		return sb.String()
	}
	return ""
}
