package models

import (
	"testing"
	"time"
)

func TestAccountComputeScoreByStatus(t *testing.T) {
	valid := (&Account{Status: StatusValid, Valid: true}).ComputeScore()
	rateLimited := (&Account{Status: StatusRateLimited, RateLimitedUntil: float64(time.Now().Add(time.Hour).Unix())}).ComputeScore()
	banned := (&Account{Status: StatusBanned}).ComputeScore()
	if !(valid > rateLimited && rateLimited > banned) {
		t.Fatalf("scores valid=%f rate=%f banned=%f", valid, rateLimited, banned)
	}
}
