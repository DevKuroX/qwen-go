package core

import (
	"sync"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
	"go.uber.org/zap"
)

// ProxyChecker exposes a Check / CheckBatch API that delegates to the shared
// LiveCheck (TCP precheck → cdn-cgi/trace forwarding test → offline geo).
// The legacy `testEndpoint` argument is accepted for compatibility but
// ignored — the verdict logic is now uniform across the scraper and the pool.
type ProxyChecker struct {
	logger           *zap.Logger
	timeout          time.Duration
	batchConcurrency int
	live             *LiveCheck
	// checkFunc is retained for tests that inject a stub verdict.
	checkFunc func(*models.Proxy, string) models.ProxyTestResult
}

func NewProxyChecker(timeout time.Duration, live *LiveCheck) *ProxyChecker {
	return &ProxyChecker{
		logger:           zap.L(),
		timeout:          timeout,
		batchConcurrency: 200,
		live:             live,
	}
}

func (pc *ProxyChecker) Check(p *models.Proxy, _testEndpoint string) models.ProxyTestResult {
	if pc.checkFunc != nil {
		return pc.checkFunc(p, _testEndpoint)
	}

	v := pc.live.Validate(p.Type, p.Host, p.Port, p.Username, p.Password)
	if !v.Alive {
		return models.ProxyTestResult{
			ID:        p.ID,
			Status:    models.ProxyStatusDead,
			LatencyMs: v.LatencyMs,
		}
	}
	return models.ProxyTestResult{
		ID:        p.ID,
		Status:    models.ProxyStatusLive,
		LatencyMs: v.LatencyMs,
		Region:    v.Country,
	}
}

func (pc *ProxyChecker) CheckBatch(proxies []*models.Proxy, testEndpoint string) []models.ProxyTestResult {
	results := make([]models.ProxyTestResult, len(proxies))
	limit := pc.batchConcurrency
	if limit <= 0 {
		limit = 1
	}
	sem := make(chan struct{}, limit)

	var wg sync.WaitGroup
	for i, p := range proxies {
		wg.Add(1)
		go func(idx int, proxy *models.Proxy) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = pc.Check(proxy, testEndpoint)
		}(i, p)
	}
	wg.Wait()
	return results
}
