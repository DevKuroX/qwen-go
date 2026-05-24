package core

import (
	"sync"
	"testing"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

func TestProxyPoolGetBestProxyPrefersQualityOverLatency(t *testing.T) {
	pool := NewProxyPool(models.ProxyPoolConfig{FallbackDirect: true}, NewProxyChecker(time.Second, nil))
	pool.Load([]*models.Proxy{
		{ID: "fast-bad", Enabled: true, Status: models.ProxyStatusLive, LatencyMs: 10, RegisterFailureCount: 4, CaptchaFailureCount: 2},
		{ID: "steady-good", Enabled: true, Status: models.ProxyStatusLive, LatencyMs: 80, RegisterSuccessCount: 6},
	})

	best := pool.GetBestProxy()
	if best == nil || best.ID != "steady-good" {
		t.Fatalf("GetBestProxy() got %+v, want steady-good", best)
	}
}

func TestProxyCheckerCheckBatchRespectsConcurrencyLimit(t *testing.T) {
	checker := NewProxyChecker(time.Second, nil)
	checker.batchConcurrency = 2

	var mu sync.Mutex
	active := 0
	maxActive := 0
	origCheck := checker.Check
	_ = origCheck

	proxies := []*models.Proxy{
		{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"},
	}

	checker.checkFunc = func(p *models.Proxy, testEndpoint string) models.ProxyTestResult {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		active--
		mu.Unlock()
		return models.ProxyTestResult{ID: p.ID, Status: models.ProxyStatusLive}
	}

	checker.CheckBatch(proxies, "https://example.com")
	if maxActive > 2 {
		t.Fatalf("max concurrent checks = %d, want <= 2", maxActive)
	}
}

func TestProxyPoolGetBestProxyReturnsNilWhenNoLiveProxy(t *testing.T) {
	pool := NewProxyPool(models.ProxyPoolConfig{FallbackDirect: true}, NewProxyChecker(time.Second, nil))
	pool.Load([]*models.Proxy{{ID: "dead", Enabled: true, Status: models.ProxyStatusDead}})

	best := pool.GetBestProxy()
	if best != nil {
		t.Fatalf("GetBestProxy() = %+v, want nil", best)
	}
}

func TestScoreProxyForRegistrationPenalizesDeadOrDisabled(t *testing.T) {
	dead := scoreProxyForRegistration(&models.Proxy{ID: "dead", Status: models.ProxyStatusDead, Enabled: true})
	disabled := scoreProxyForRegistration(&models.Proxy{ID: "disabled", Status: models.ProxyStatusLive, Enabled: false})
	live := scoreProxyForRegistration(&models.Proxy{ID: "live", Status: models.ProxyStatusLive, Enabled: true})

	if !(dead < live && disabled < live) {
		t.Fatalf("scores dead=%d disabled=%d live=%d", dead, disabled, live)
	}
}
