package openai

import (
	"context"
	"errors"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

type OpenAIProvider struct{}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

func (p *OpenAIProvider) Prefix() string {
	return "openai/"
}

func (p *OpenAIProvider) Type() models.ProviderType {
	return models.ProviderTypeAPIKey
}

func (p *OpenAIProvider) ResolveModel(model string) string {
	return model
}

func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req *models.ChatRequest) (*models.ChatResponse, error) {
	return nil, errors.New("0penAI provider not implemented yet")
}

func (p *OpenAIProvider) ChatStream(ctx context.Context, req *models.ChatRequest) (<-chan models.StreamChunk, error) {
	return nil, errors.New("0penAI provider not implemented yet")
}

func (p *OpenAIProvider) ImageGeneration(ctx context.Context, req *models.ImageRequest) (*models.ImageResponse, error) {
	return nil, errors.New("0penAI provider not implemented yet")
}

func (p *OpenAIProvider) VideoGeneration(ctx context.Context, req *models.VideoRequest) (*models.VideoResponse, error) {
	return nil, errors.New("0penAI provider not implemented yet")
}

func (p *OpenAIProvider) Health(ctx context.Context) error {
	return errors.New("0penAI provider not implemented yet")
}
