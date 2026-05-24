package core

import (
	"fmt"
	"strings"
	"sync"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

type ProviderManager struct {
	providers         map[string]models.Provider
	providersByPrefix map[string]models.Provider
	mu                sync.RWMutex
}

func NewProviderManager() *ProviderManager {
	return &ProviderManager{
		providers:         make(map[string]models.Provider),
		providersByPrefix: make(map[string]models.Provider),
	}
}

func (pm *ProviderManager) Register(provider models.Provider) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	name := provider.Name()
	prefix := provider.Prefix()

	pm.providers[name] = provider
	pm.providersByPrefix[prefix] = provider
}

func (pm *ProviderManager) GetByName(name string) models.Provider {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.providers[name]
}

func (pm *ProviderManager) GetByPrefix(prefix string) models.Provider {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.providersByPrefix[prefix]
}

func (pm *ProviderManager) Route(model string) (models.Provider, string, error) {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid model format: %s (expected: prefix/model)", model)
	}

	prefix := parts[0] + "/"
	modelName := parts[1]

	provider := pm.GetByPrefix(prefix)
	if provider == nil {
		return nil, "", fmt.Errorf("provider not found for prefix: %s", prefix)
	}

	resolvedModel := resolveRuntimeModelAlias(modelName)
	resolvedModel = provider.ResolveModel(resolvedModel)

	return provider, resolvedModel, nil
}

func (pm *ProviderManager) Metadata(name string) (models.ProviderMetadata, bool) {
	provider := pm.GetByName(name)
	if provider == nil {
		return models.ProviderMetadata{}, false
	}
	return provider.Metadata(), true
}

func (pm *ProviderManager) ListProviders() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	names := make([]string, 0, len(pm.providers))
	for name := range pm.providers {
		names = append(names, name)
	}
	return names
}

func resolveRuntimeModelAlias(model string) string {
	if GlobalSettingsManager == nil {
		return model
	}
	settings := GlobalSettingsManager.Get()
	if settings.ModelAliases == nil {
		return model
	}
	if resolved, ok := settings.ModelAliases[model]; ok && resolved != "" {
		return resolved
	}
	return model
}
