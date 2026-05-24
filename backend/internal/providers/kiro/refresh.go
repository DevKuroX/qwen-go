package kiro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

// refreshClient owns the HTTP transport used for credential refresh.
// Separate from the chat client so refresh requests don't share a timeout
// budget with potentially slow streaming chat calls.
type refreshClient struct {
	http     *http.Client
	registry *core.ProviderRegistry
}

func newRefreshClient(registry *core.ProviderRegistry) *refreshClient {
	return &refreshClient{
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		registry: registry,
	}
}

// refreshResult is the normalized output of either refresh path. Empty
// fields signal "upstream did not rotate this" — caller keeps the previous
// value rather than blanking the account.
type refreshResult struct {
	AccessToken  string
	RefreshToken string
	ProfileArn   string // Social refresh only — SSO-OIDC doesn't return this.
	ExpiresIn    int64
}

// Refresh branches on Metadata["auth_method"]. Values used:
//   - "builder-id"  → SSO-OIDC (needs client_id + client_secret in Metadata)
//   - "idc"         → SSO-OIDC (same shape)
//   - "google", "github", "imported" → Social Auth host
//
// The imported branch is special: refresh_token starts with "aorAAAAAG..."
// and only the Social Auth host accepts it. The first refresh after import
// is what harvests the profileArn we'll need for chat requests.
func (r *refreshClient) Refresh(acc *models.Account) (*refreshResult, error) {
	if acc.RefreshToken == "" {
		return nil, errors.New("kiro: refresh_token missing on account")
	}
	method := acc.Metadata["auth_method"]
	switch method {
	case "builder-id", "idc":
		return r.refreshSSOOIDC(acc)
	default:
		// Social, imported, or unset — all flow through the Social host.
		// Unset defaults here so legacy imports without auth_method still
		// work; the import endpoint should set "imported".
		return r.refreshSocial(acc)
	}
}

func (r *refreshClient) refreshSSOOIDC(acc *models.Account) (*refreshResult, error) {
	clientID := acc.Metadata["client_id"]
	clientSecret := acc.Metadata["client_secret"]
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("kiro: SSO-OIDC refresh requires client_id + client_secret in metadata")
	}
	endpoint := r.ssoOIDCEndpoint(acc.Metadata["region"])

	body := map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
		"refreshToken": acc.RefreshToken,
		"grantType":    "refresh_token",
	}
	return r.post(endpoint, body, false)
}

func (r *refreshClient) refreshSocial(acc *models.Account) (*refreshResult, error) {
	endpoint := r.socialAuthHost() + "/refreshToken"
	body := map[string]string{"refreshToken": acc.RefreshToken}
	return r.post(endpoint, body, true)
}

// post issues the JSON refresh call. wantProfileArn=true for Social refresh
// (the response carries a profileArn we have to persist for chat requests);
// false for SSO-OIDC where profileArn is harvested separately.
func (r *refreshClient) post(endpoint string, body interface{}, wantProfileArn bool) (*refreshResult, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiro refresh: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ProfileArn   string `json:"profileArn"`
		ExpiresIn    int64  `json:"expiresIn"`
		// Some responses use snake_case; tolerate it.
		AccessTokenAlt  string `json:"access_token"`
		RefreshTokenAlt string `json:"refresh_token"`
		ExpiresInAlt    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("kiro refresh: parse response: %w", err)
	}

	out := &refreshResult{
		AccessToken:  firstNonEmpty(parsed.AccessToken, parsed.AccessTokenAlt),
		RefreshToken: firstNonEmpty(parsed.RefreshToken, parsed.RefreshTokenAlt),
		ExpiresIn:    firstNonZero(parsed.ExpiresIn, parsed.ExpiresInAlt),
	}
	if wantProfileArn {
		out.ProfileArn = parsed.ProfileArn
	}
	if out.AccessToken == "" {
		return nil, errors.New("kiro refresh: response missing accessToken")
	}
	return out, nil
}

func (r *refreshClient) ssoOIDCEndpoint(region string) string {
	if cfg := r.registry.Get(providerName); cfg != nil && cfg.SSOOIDCEndpoint != "" {
		return cfg.SSOOIDCEndpoint
	}
	if region == "" {
		region = "us-east-1"
	}
	return "https://oidc." + region + ".amazonaws.com/token"
}

func (r *refreshClient) socialAuthHost() string {
	if cfg := r.registry.Get(providerName); cfg != nil && cfg.SocialAuthHost != "" {
		return cfg.SocialAuthHost
	}
	return "https://prod.us-east-1.auth.desktop.kiro.dev"
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstNonZero(a, b int64) int64 {
	if a != 0 {
		return a
	}
	return b
}
