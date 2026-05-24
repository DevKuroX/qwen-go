package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

// ModelMeta holds upstream-side metadata that the dashboard may need to edit
// when an upstream rotates IDs (e.g. Gemini Web's per-model UUID + capacity).
type ModelMeta struct {
	ID       string `json:"id"`
	Capacity int    `json:"capacity,omitempty"`
}

// ModelAlias maps a virtual model name (e.g. "deepseek-v4-flash-free-thinking")
// to a real upstream model + preset request params. Per-provider; only
// opencode-zen wires this today. Request body params override Params on
// collision (alias = defaults, not straitjacket).
type ModelAlias struct {
	Base   string                 `json:"base"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// ProviderConfig is the dashboard-editable payload backing each provider's
// runtime behavior (base endpoint, headers, payload templates, auth mode,
// model lists). It is the single source of truth for HTTP-level provider
// behavior — the in-memory registry is consulted on every request, never
// the JSON file.
type ProviderConfig struct {
	Name            string                      `json:"name"`
	BaseEndpoint    string                      `json:"base_endpoint"`
	Headers         map[string]string           `json:"headers,omitempty"`
	PayloadTemplate string                      `json:"payload_template,omitempty"`
	AuthMode        string                      `json:"auth_mode"` // "bearer" | "cookie-blob" | "refresh-access" | "ephemeral-session" | "cookie-pair-scraped"
	AvailableModels []string                    `json:"available_models,omitempty"`
	Models          map[string]ModelMeta        `json:"models,omitempty"`        // Gemini Web: name → {ID, Capacity}
	ModelAliases    map[string]ModelAlias       `json:"model_aliases,omitempty"` // variant-name → {base, preset params}; see opencode-zen
	Capabilities    models.ProviderCapabilities `json:"capabilities"`

	// ephemeral-session providers (OpenCode Zen)
	SessionMaxRequests    int `json:"session_max_requests,omitempty"`
	MaxConcurrentSessions int `json:"max_concurrent_sessions,omitempty"`

	// gemini-web
	AccessTokenTTLMinutes int    `json:"access_token_ttl_minutes,omitempty"`
	CaptureBackend        string `json:"capture_backend,omitempty"`
	PythonScriptPath      string `json:"python_script_path,omitempty"`

	// kiro
	QuotaEndpoints  []string `json:"quota_endpoints,omitempty"`
	QuotaCacheTTLs  int      `json:"quota_cache_ttls,omitempty"`
	RefreshSkewSec  int      `json:"refresh_skew_sec,omitempty"`
	SSOOIDCEndpoint string   `json:"sso_oidc_endpoint,omitempty"`
	SocialAuthHost  string   `json:"social_auth_host,omitempty"`
}

// ProviderRegistry is an in-memory cache of ProviderConfig keyed by provider
// name. Reads are lock-free fast; writes go through Update() which persists
// to JSON. Mirrors the KeyManager / SettingsManager pattern.
type ProviderRegistry struct {
	mu      sync.RWMutex
	path    string
	configs map[string]*ProviderConfig
}

// GlobalProviderRegistry is wired in server.Run so handlers can consult it
// without dependency injection plumbing (matches GlobalSettingsManager).
var GlobalProviderRegistry *ProviderRegistry

func NewProviderRegistry(path string) *ProviderRegistry {
	r := &ProviderRegistry{path: path, configs: make(map[string]*ProviderConfig)}
	r.Load()
	return r
}

// Load reads provider_configs.json. If the file is missing, the registry
// seeds itself with built-in defaults and writes them back so the dashboard
// has something to render on first run.
func (r *ProviderRegistry) Load() {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			r.configs = defaultProviderConfigs()
			// Best-effort initial seed; if write fails the registry still
			// works in-memory and the next Update() will retry persistence.
			_ = r.saveLocked()
			return
		}
		r.configs = defaultProviderConfigs()
		return
	}

	var list []*ProviderConfig
	if err := json.Unmarshal(data, &list); err != nil {
		r.configs = defaultProviderConfigs()
		return
	}

	r.configs = make(map[string]*ProviderConfig, len(list))
	for _, cfg := range list {
		if cfg == nil || cfg.Name == "" {
			continue
		}
		r.configs[cfg.Name] = cfg
	}

	// Merge in any built-in defaults that don't have a stored entry yet so
	// adding a new provider in code doesn't require manually editing the file.
	for name, def := range defaultProviderConfigs() {
		if _, ok := r.configs[name]; !ok {
			r.configs[name] = def
		}
	}
}

// Get returns the config for a provider, or nil if not registered.
// Fast path — designed to be called once per request.
func (r *ProviderRegistry) Get(name string) *ProviderConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.configs[name]
}

// List returns a snapshot copy of every registered config.
func (r *ProviderRegistry) List() []*ProviderConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ProviderConfig, 0, len(r.configs))
	for _, cfg := range r.configs {
		out = append(out, cfg)
	}
	return out
}

// Update replaces a provider's config in-place and persists the full set
// to JSON. Returns an error if the name is empty or the write fails.
func (r *ProviderRegistry) Update(name string, cfg *ProviderConfig) error {
	if name == "" {
		return errors.New("provider name required")
	}
	if cfg == nil {
		return errors.New("config required")
	}
	cfg.Name = name

	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[name] = cfg
	return r.saveLocked()
}

// Delete removes a provider config. Built-in defaults can be deleted —
// they'll just be re-seeded on next startup if the entry is missing.
func (r *ProviderRegistry) Delete(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.configs[name]; !ok {
		return fmt.Errorf("provider not found: %s", name)
	}
	delete(r.configs, name)
	return r.saveLocked()
}

func (r *ProviderRegistry) saveLocked() error {
	list := make([]*ProviderConfig, 0, len(r.configs))
	for _, cfg := range r.configs {
		list = append(list, cfg)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

func defaultProviderConfigs() map[string]*ProviderConfig {
	return map[string]*ProviderConfig{
		"qwen": {
			Name:         "qwen",
			BaseEndpoint: "https://chat.qwen.ai/api/v2/chat/completions",
			Headers: map[string]string{
				"User-Agent":   "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
				"Accept":       "text/event-stream",
				"Content-Type": "application/json",
			},
			AuthMode: "cookie-blob",
			AvailableModels: []string{
				"qwen3.6-max-preview", "qwen3.6-plus", "qwen3.6-27b",
			},
			Capabilities: models.ProviderCapabilities{
				SupportsChat: true, SupportsStream: true, SupportsImage: true,
			},
		},
		"opencode-zen": {
			Name:         "opencode-zen",
			BaseEndpoint: "https://opencode.ai/zen/v1/chat/completions",
			Headers: map[string]string{
				"User-Agent":   "opencode/1.15.9 (Linux)",
				"Content-Type": "application/json",
			},
			AuthMode:        "ephemeral-session",
			AvailableModels: []string{"deepseek-v4-flash-free"},
			ModelAliases: map[string]ModelAlias{
				"deepseek-v4-flash-free-thinking": {
					Base: "deepseek-v4-flash-free",
					Params: map[string]interface{}{
						"reasoning_effort":  "high",
						"verbosity":         "low",
						"reasoning_summary": "auto",
					},
				},
			},
			SessionMaxRequests:    190,
			MaxConcurrentSessions: 10,
			Capabilities: models.ProviderCapabilities{
				SupportsChat: true, SupportsStream: true,
			},
		},
		"gemini-web": {
			Name:         "gemini-web",
			BaseEndpoint: "https://gemini.google.com",
			Headers: map[string]string{
				"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			},
			AuthMode: "cookie-pair-scraped",
			Models: map[string]ModelMeta{
				"gemini-2.5-pro":   {ID: "fbb127bbb056c959", Capacity: 1},
				"gemini-2.5-flash": {ID: "f299729f40d7f0d2", Capacity: 1},
			},
			AccessTokenTTLMinutes: 50,
			CaptureBackend:        "auto",
			PythonScriptPath:      "/home/ubuntu/workai/qwen-go/gemini_auto_cookie.py",
			Capabilities: models.ProviderCapabilities{
				SupportsChat: true, SupportsStream: true,
			},
		},
		"kiro": {
			Name:         "kiro",
			BaseEndpoint: "https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse",
			Headers: map[string]string{
				"Content-Type": "application/x-amz-json-1.0",
			},
			AuthMode:        "refresh-access",
			AvailableModels: []string{"claude-3-5-sonnet-20241022", "claude-3-7-sonnet-20250219"},
			QuotaEndpoints: []string{
				"https://codewhisperer.us-east-1.amazonaws.com/getUsageLimits",
				"https://codewhisperer.us-east-1.amazonaws.com/listUsageLimits",
				"https://q.us-east-1.amazonaws.com/getUsageLimits",
			},
			QuotaCacheTTLs:  60,
			RefreshSkewSec:  60,
			SSOOIDCEndpoint: "https://oidc.us-east-1.amazonaws.com/token",
			SocialAuthHost:  "https://prod.us-east-1.auth.desktop.kiro.dev",
			Capabilities: models.ProviderCapabilities{
				SupportsChat: true, SupportsStream: true,
			},
		},
	}
}
