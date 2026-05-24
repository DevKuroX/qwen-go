package models

import "context"

type ProviderType string

const (
	ProviderTypeAccount ProviderType = "account"
	ProviderTypeAPIKey  ProviderType = "api_key"
	ProviderTypePublic  ProviderType = "public"
)

type StreamChunk struct {
	Content string
	Done    bool
	Error   string
}

type ProviderErrorKind string

const (
	ProviderErrorUnknown    ProviderErrorKind = "unknown"
	ProviderErrorAuth       ProviderErrorKind = "auth"
	ProviderErrorRateLimit  ProviderErrorKind = "rate_limit"
	ProviderErrorBanned     ProviderErrorKind = "banned"
	ProviderErrorTransient  ProviderErrorKind = "transient"
)

type ProviderCapabilities struct {
	SupportsChat   bool `json:"supports_chat"`
	SupportsStream bool `json:"supports_stream"`
	SupportsImage  bool `json:"supports_image"`
}

type ProviderLimits struct {
	DefaultContextWindow int `json:"default_context_window"`
	MaxRetries           int `json:"max_retries"`
}

type ProviderMetadata struct {
	Capabilities ProviderCapabilities `json:"capabilities"`
	Limits       ProviderLimits       `json:"limits"`
}

type ProviderError struct {
	Kind    ProviderErrorKind `json:"kind"`
	Message string            `json:"message"`
}

type Provider interface {
	Name() string
	Prefix() string
	Type() ProviderType

	ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
	ImageGeneration(ctx context.Context, req *ImageRequest) (*ImageResponse, error)
	VideoGeneration(ctx context.Context, req *VideoRequest) (*VideoResponse, error)

	Health(ctx context.Context) error

	ResolveModel(model string) string
	Metadata() ProviderMetadata
}

// AuthProvider is an optional interface providers implement when they have
// non-trivial auth material (refresh-token rotation, cookie scraping,
// ephemeral session UUIDs, etc.). The generic chat call path may call these
// before dispatch; providers that only need a static Bearer don't need to
// implement it.
type AuthProvider interface {
	BuildAuthHeaders(acc *Account) map[string]string
	EnsureFresh(acc *Account) error
	RefreshAuth(acc *Account) error
}
