package core

import (
	"context"
	"testing"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

type stubProvider struct {
	name   string
	prefix string
	meta   models.ProviderMetadata
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Prefix() string { return s.prefix }
func (s *stubProvider) Type() models.ProviderType { return models.ProviderTypeAPIKey }
func (s *stubProvider) ChatCompletion(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) { return nil, nil }
func (s *stubProvider) ChatStream(ctx context.Context, req *models.ChatRequest) (<-chan models.StreamChunk, error) { return nil, nil }
func (s *stubProvider) ImageGeneration(ctx context.Context, req *models.ImageRequest) (*models.ImageResponse, error) { return nil, nil }
func (s *stubProvider) VideoGeneration(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) { return nil, nil }
func (s *stubProvider) Health(ctx context.Context) error { return nil }
func (s *stubProvider) ResolveModel(model string) string { return "resolved-" + model }
func (s *stubProvider) Metadata() models.ProviderMetadata { return s.meta }

func TestProviderManagerRouteByPrefix(t *testing.T) {
	pm := NewProviderManager()
	pm.Register(&stubProvider{name: "stub", prefix: "st/"})

	provider, model, err := pm.Route("st/demo")
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if provider.Name() != "stub" || model != "resolved-demo" {
		t.Fatalf("Route() got provider=%v model=%q", provider.Name(), model)
	}
}

func TestProviderManagerCapabilityMetadata(t *testing.T) {
	pm := NewProviderManager()
	pm.Register(&stubProvider{
		name: "stub",
		prefix: "st/",
		meta: models.ProviderMetadata{
			Capabilities: models.ProviderCapabilities{SupportsChat: true, SupportsStream: true},
			Limits:       models.ProviderLimits{DefaultContextWindow: 4096, MaxRetries: 1},
		},
	})

	meta, ok := pm.Metadata("stub")
	if !ok {
		t.Fatal("Metadata() ok = false, want true")
	}
	if !meta.Capabilities.SupportsChat || meta.Limits.DefaultContextWindow != 4096 {
		t.Fatalf("Metadata() got %+v", meta)
	}
}

func TestProviderManagerRouteUsesRuntimeAliasOverride(t *testing.T) {
	GlobalSettingsManager = &SettingsManager{settings: DynamicSettings{ModelAliases: map[string]string{"demo": "alias-model"}}}
	pm := NewProviderManager()
	pm.Register(&stubProvider{name: "stub", prefix: "st/"})

	_, model, err := pm.Route("st/demo")
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if model != "resolved-alias-model" {
		t.Fatalf("Route() model = %q, want resolved-alias-model", model)
	}
}
