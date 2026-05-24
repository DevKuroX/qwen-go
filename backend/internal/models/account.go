package models

import "time"

type AccountStatus string

const (
	StatusValid        AccountStatus = "VALID"
	StatusRateLimited  AccountStatus = "RATE_LIMITED"
	StatusSoftError    AccountStatus = "SOFT_ERROR"
	StatusCircuitOpen  AccountStatus = "CIRCUIT_OPEN"
	StatusHalfOpen     AccountStatus = "HALF_OPEN"
	StatusBanned       AccountStatus = "BANNED"
)

type Account struct {
	Email               string        `json:"email"`
	Password            string        `json:"password"`
	Token               string        `json:"token"`
	Cookies             string        `json:"cookies,omitempty"`
	Username            string        `json:"username"`
	Provider            string        `json:"provider"`
	Status              AccountStatus `json:"status"`
	StatusCode          string        `json:"status_code,omitempty"`
	Inflight            int           `json:"inflight"`
	RateLimitedUntil    float64       `json:"rate_limited_until"`
	RateLimitCount      int           `json:"rate_limit_count"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	CircuitOpenCount    int           `json:"circuit_open_count"`
	LastRequestTime     time.Time     `json:"last_request_time"`
	LastError           string        `json:"last_error"`
	ActivationPending   bool          `json:"activation_pending"`
	Score               float64       `json:"score"`
	Valid               bool          `json:"valid,omitempty"`
	CreatedAt           time.Time     `json:"created_at"`
	DeletedAt           *time.Time    `json:"deleted_at,omitempty"`

	// Multi-provider auth material. Provider populates whichever applies.
	RefreshToken string            `json:"refresh_token,omitempty"` // Kiro, Gemini-Web (__Secure-1PSIDTS)
	ExpiresAt    int64             `json:"expires_at,omitempty"`    // Unix seconds (Kiro)
	RequestCount int64             `json:"request_count,omitempty"` // OpenCode-Zen rotation counter
	SpawnedAt    int64             `json:"spawned_at,omitempty"`    // OpenCode-Zen session age
	Metadata     map[string]string `json:"metadata,omitempty"`      // Kiro: profile_arn, auth_method, region, client_id, client_secret
}

func (a *Account) EffectiveMaxRPM() int {
	baseRPM := 50
	if a.RateLimitCount > 0 {
		baseRPM = 30
	}
	if a.ConsecutiveFailures > 0 {
		baseRPM = 20
	}
	return baseRPM
}

func (a *Account) ComputeScore() float64 {
	score := 0.0
	
	status := a.Status
	if status == "" && a.StatusCode != "" {
		status = AccountStatus(a.StatusCode)
	}
	
	if status == StatusValid || a.Valid {
		score += 100
	} else if status == StatusRateLimited {
		if a.RateLimitedUntil > float64(time.Now().Unix()) {
			return -1000
		}
		score += 50
	} else if status == StatusCircuitOpen {
		return -500
	} else if status == StatusBanned {
		return -10000
	}
	
	score -= float64(a.Inflight * 10)
	score -= float64(a.ConsecutiveFailures * 20)
	score -= float64(a.RateLimitCount * 5)
	
	return score
}
