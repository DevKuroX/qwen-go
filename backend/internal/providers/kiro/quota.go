package kiro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
)

// QuotaInfo is the dashboard-facing representation of a single quota bucket
// (e.g. "agentic_request", "agentic_request_freetrial"). All numeric fields
// match the upstream "WithPrecision" doubles directly.
type QuotaInfo struct {
	Used      float64    `json:"used"`
	Total     float64    `json:"total"`
	Remaining float64    `json:"remaining"`
	ResetAt   *time.Time `json:"reset_at,omitempty"`
}

// QuotaSnapshot is what the cache hands out and what handlers render.
type QuotaSnapshot struct {
	Plan      string               `json:"plan"`
	Quotas    map[string]QuotaInfo `json:"quotas"`
	FetchedAt time.Time            `json:"fetched_at"`
}

// QuotaCache holds the last-known QuotaSnapshot per account email with
// TTL-only eviction. Per user feedback ("TTL"): no LRU, no max-size cap —
// stale entries just expire. In-memory only, never persisted.
type QuotaCache struct {
	mu       sync.RWMutex
	registry *core.ProviderRegistry
	http     *http.Client
	entries  map[string]quotaEntry
}

type quotaEntry struct {
	snap   *QuotaSnapshot
	expiry time.Time
}

func NewQuotaCache(registry *core.ProviderRegistry) *QuotaCache {
	return &QuotaCache{
		registry: registry,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
		entries: make(map[string]quotaEntry),
	}
}

// Get returns the cached snapshot if still within TTL. Caller decides
// whether to fetch on miss — keeps the hot chat path off the quota API
// unless the dashboard explicitly asks for it.
func (c *QuotaCache) Get(email string) *QuotaSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[email]
	if !ok {
		return nil
	}
	if time.Now().After(e.expiry) {
		return nil
	}
	return e.snap
}

// Fetch hits the upstream quota API. Tries each configured endpoint in
// order; the first 200 wins. Stores the result in the cache and returns it.
// Returns (nil, err) when every endpoint fails — caller can render "quota
// unavailable" without erroring out chat.
func (c *QuotaCache) Fetch(acc *models.Account) (*QuotaSnapshot, error) {
	endpoints := c.endpoints()
	profileArn := acc.Metadata["profile_arn"]
	if profileArn == "" {
		profileArn = defaultProfileArn
	}

	var lastErr error
	for _, ep := range endpoints {
		snap, err := c.tryEndpoint(ep, acc.Token, profileArn)
		if err != nil {
			lastErr = err
			continue
		}
		c.store(acc.Email, snap)
		return snap, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("kiro: no quota endpoints configured")
	}
	return nil, lastErr
}

// tryEndpoint dispatches to the right verb/body for each known endpoint
// shape. Mirrors the JS reference's three "attempt" entries.
func (c *QuotaCache) tryEndpoint(endpoint, accessToken, profileArn string) (*QuotaSnapshot, error) {
	// The codewhisperer host has two shapes: GET ?resourceType=... and POST
	// AmazonCodeWhispererService.GetUsageLimits. Switch on the path.
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	switch {
	case u.Path == "/getUsageLimits" && u.Host != "":
		return c.fetchGET(endpoint, accessToken, profileArn)
	case u.Host == "codewhisperer.us-east-1.amazonaws.com" && (u.Path == "" || u.Path == "/"):
		return c.fetchPOST(endpoint, accessToken, profileArn)
	default:
		// Best-effort: try GET, then fall back to POST.
		if snap, err := c.fetchGET(endpoint, accessToken, profileArn); err == nil {
			return snap, nil
		}
		return c.fetchPOST(endpoint, accessToken, profileArn)
	}
}

func (c *QuotaCache) fetchGET(endpoint, accessToken, profileArn string) (*QuotaSnapshot, error) {
	q := url.Values{
		"isEmailRequired": []string{"true"},
		"origin":          []string{"AI_EDITOR"},
		"resourceType":    []string{"AGENTIC_REQUEST"},
	}
	if profileArn != "" {
		q.Set("profileArn", profileArn)
	}
	sep := "?"
	if u, _ := url.Parse(endpoint); u != nil && u.RawQuery != "" {
		sep = "&"
	}
	full := endpoint + sep + q.Encode()

	req, err := http.NewRequest("GET", full, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.0 KiroIDE")
	req.Header.Set("user-agent", "aws-sdk-js/1.0.0 KiroIDE")
	return c.exec(req)
}

func (c *QuotaCache) fetchPOST(endpoint, accessToken, profileArn string) (*QuotaSnapshot, error) {
	body := map[string]interface{}{
		"origin":       "AI_EDITOR",
		"resourceType": "AGENTIC_REQUEST",
	}
	if profileArn != "" {
		body["profileArn"] = profileArn
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("x-amz-target", "AmazonCodeWhispererService.GetUsageLimits")
	req.Header.Set("Accept", "application/json")
	return c.exec(req)
}

func (c *QuotaCache) exec(req *http.Request) (*QuotaSnapshot, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return parseQuotaPayload(body)
}

// parseQuotaPayload maps the AWS getUsageLimits response onto QuotaSnapshot.
// Mirrors the JS reference's parseKiroQuotaData.
func parseQuotaPayload(body []byte) (*QuotaSnapshot, error) {
	var parsed struct {
		NextDateReset      string `json:"nextDateReset"`
		ResetDate          string `json:"resetDate"`
		SubscriptionInfo   struct {
			SubscriptionTitle string `json:"subscriptionTitle"`
		} `json:"subscriptionInfo"`
		UsageBreakdownList []struct {
			ResourceType              string  `json:"resourceType"`
			CurrentUsageWithPrecision float64 `json:"currentUsageWithPrecision"`
			UsageLimitWithPrecision   float64 `json:"usageLimitWithPrecision"`
			FreeTrialInfo             *struct {
				CurrentUsageWithPrecision float64 `json:"currentUsageWithPrecision"`
				UsageLimitWithPrecision   float64 `json:"usageLimitWithPrecision"`
				FreeTrialExpiry           string  `json:"freeTrialExpiry"`
			} `json:"freeTrialInfo"`
		} `json:"usageBreakdownList"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse quota payload: %w", err)
	}

	resetAt := parseTimeFlexible(firstNonEmpty(parsed.NextDateReset, parsed.ResetDate))
	quotas := make(map[string]QuotaInfo, len(parsed.UsageBreakdownList)*2)
	for _, b := range parsed.UsageBreakdownList {
		key := lower(b.ResourceType)
		if key == "" {
			key = "unknown"
		}
		quotas[key] = QuotaInfo{
			Used:      b.CurrentUsageWithPrecision,
			Total:     b.UsageLimitWithPrecision,
			Remaining: b.UsageLimitWithPrecision - b.CurrentUsageWithPrecision,
			ResetAt:   resetAt,
		}
		if b.FreeTrialInfo != nil {
			ft := b.FreeTrialInfo
			ftReset := parseTimeFlexible(ft.FreeTrialExpiry)
			if ftReset == nil {
				ftReset = resetAt
			}
			quotas[key+"_freetrial"] = QuotaInfo{
				Used:      ft.CurrentUsageWithPrecision,
				Total:     ft.UsageLimitWithPrecision,
				Remaining: ft.UsageLimitWithPrecision - ft.CurrentUsageWithPrecision,
				ResetAt:   ftReset,
			}
		}
	}

	return &QuotaSnapshot{
		Plan:      firstNonEmpty(parsed.SubscriptionInfo.SubscriptionTitle, "Kiro"),
		Quotas:    quotas,
		FetchedAt: time.Now(),
	}, nil
}

func (c *QuotaCache) store(email string, snap *QuotaSnapshot) {
	ttl := c.ttl()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[email] = quotaEntry{
		snap:   snap,
		expiry: time.Now().Add(ttl),
	}
}

func (c *QuotaCache) ttl() time.Duration {
	if cfg := c.registry.Get(providerName); cfg != nil && cfg.QuotaCacheTTLs > 0 {
		return time.Duration(cfg.QuotaCacheTTLs) * time.Second
	}
	return 60 * time.Second
}

func (c *QuotaCache) endpoints() []string {
	if cfg := c.registry.Get(providerName); cfg != nil && len(cfg.QuotaEndpoints) > 0 {
		return cfg.QuotaEndpoints
	}
	return []string{
		"https://codewhisperer.us-east-1.amazonaws.com/getUsageLimits",
		"https://codewhisperer.us-east-1.amazonaws.com",
		"https://q.us-east-1.amazonaws.com/getUsageLimits",
	}
}

// parseTimeFlexible accepts ISO-8601 strings, unix seconds, unix millis.
// Returns nil on unrecognized input so callers can fall back to "no reset".
func parseTimeFlexible(v string) *time.Time {
	if v == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t
	}
	// Numeric? Try as unix seconds, then millis.
	var num int64
	if _, err := fmt.Sscanf(v, "%d", &num); err == nil && num > 0 {
		var t time.Time
		if num > 1e12 {
			t = time.UnixMilli(num)
		} else {
			t = time.Unix(num, 0)
		}
		return &t
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
