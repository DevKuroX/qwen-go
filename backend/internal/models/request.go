package models

import (
	"encoding/json"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`

	// Reasoning passthrough — OpenAI-spec fields forwarded as-is to providers
	// that support them. Currently only opencode-zen consumes these.
	ReasoningEffort  string `json:"reasoning_effort,omitempty"`
	Verbosity        string `json:"verbosity,omitempty"`
	ReasoningSummary string `json:"reasoning_summary,omitempty"`
}

type ChatChoice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message,omitempty"`
	Delta        *ChatMessage `json:"delta,omitempty"`
	FinishReason string       `json:"finish_reason"`
}

type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatResponse struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChatChoice  `json:"choices"`
	Usage   *ChatUsage    `json:"usage,omitempty"`
}

type ImageRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	N      int    `json:"n,omitempty"`
	Size   string `json:"size,omitempty"`
	// Image — optional base64-encoded input image. When present, the
	// provider treats the call as an image edit instead of fresh generation.
	// Accepts either a raw base64 string or a data URL (data:image/png;base64,…).
	Image string `json:"image,omitempty"`
}

type ImageData struct {
	URL           string `json:"url"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type ImageResponse struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
}

type VideoRequest struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt"`
	N           int    `json:"n,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
}

type VideoData struct {
	URL           string `json:"url"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	AspectRatio   string `json:"aspect_ratio,omitempty"`
}

type VideoResponse struct {
	Created int64       `json:"created"`
	Data    []VideoData `json:"data"`
}

type RegistrationRequest struct {
	Count         int    `json:"count"`
	Threads       int    `json:"threads"`
	Provider      string `json:"provider"`
	MoeMailDomain string `json:"moemail_domain,omitempty"`
	MoeMailKey    string `json:"moemail_key,omitempty"`
	TempMailDomain string `json:"tempmail_domain,omitempty"`
	TempMailKey   string `json:"tempmail_key,omitempty"`
}

type RegistrationResponse struct {
	Success  bool              `json:"success"`
	Accounts []json.RawMessage `json:"accounts,omitempty"`
	Error    string            `json:"error,omitempty"`
}
