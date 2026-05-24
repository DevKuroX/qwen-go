package models

import "time"

type ProxyStatus string

const (
	ProxyStatusLive     ProxyStatus = "live"
	ProxyStatusDead     ProxyStatus = "dead"
	ProxyStatusChecking ProxyStatus = "checking"
	ProxyStatusUnknown  ProxyStatus = "unknown"
)

type Proxy struct {
	ID                   string      `json:"id"`
	Enabled              bool        `json:"enabled"`
	Type                 string      `json:"type"`
	Host                 string      `json:"host"`
	Port                 int         `json:"port"`
	Username             string      `json:"username,omitempty"`
	Password             string      `json:"password,omitempty"`
	Status               ProxyStatus `json:"status"`
	Region               string      `json:"region"`
	LatencyMs            int         `json:"latency_ms"`
	LastCheck            time.Time   `json:"last_checked,omitempty"`
	FailCount            int         `json:"fail_count"`
	RegisterSuccessCount int         `json:"register_success_count,omitempty"`
	RegisterFailureCount int         `json:"register_failure_count,omitempty"`
	CaptchaFailureCount  int         `json:"captcha_failure_count,omitempty"`
	LastRegisterFailure  string      `json:"last_register_failure,omitempty"`
	CreatedAt            time.Time   `json:"created_at"`
}

type ProxyPoolConfig struct {
	Enabled          bool   `json:"enabled"`
	TestEndpoint     string `json:"test_endpoint"`
	RotationStrategy string `json:"rotation_strategy"`
	AutoTestInterval int    `json:"auto_test_interval"`
	AutoDeleteFailed bool   `json:"auto_delete_failed"`
	FallbackDirect   bool   `json:"fallback_direct"`
	AutoLogin        bool   `json:"auto_login"`

	// ApplyTo controls which paths route through the proxy pool. Two switches:
	//   BatchRegistration — account-creation flows spawn engines through the proxy.
	//   ProviderCall      — every provider outbound HTTP (chat/image/video) honours the proxy.
	// Defaults: BatchRegistration=true (today's behavior), ProviderCall=false.
	ApplyTo ProxyApplyToConfig `json:"apply_to"`

	// Deprecated: ApplyToProviders was the previous per-provider opt-in shape.
	// Kept for backward compat with existing proxies.json files; new code should
	// read ApplyTo instead.
	ApplyToProviders []string `json:"apply_to_providers,omitempty"`
}

type ProxyApplyToConfig struct {
	BatchRegistration bool `json:"batch_registration"`
	ProviderCall      bool `json:"provider_call"`
}

type ProxyImportRequest struct {
	RawURLs []string `json:"raw_urls"`
}

type ProxyToggleRequest struct {
	Enabled bool `json:"enabled"`
}

type ProxyBatchDeleteRequest struct {
	IDs []string `json:"ids"`
}

type ProxyTestResult struct {
	ID        string      `json:"id"`
	Status    ProxyStatus `json:"status"`
	LatencyMs int         `json:"latency_ms"`
	Region    string      `json:"region"`
	Error     string      `json:"error,omitempty"`
}
