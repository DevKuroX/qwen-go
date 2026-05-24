package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
	"go.uber.org/zap"
)

type ProxyPool struct {
	mu         sync.RWMutex
	proxies    map[string]*models.Proxy
	config     models.ProxyPoolConfig
	checker    *ProxyChecker
	logger     *zap.Logger
	stopTick   chan struct{}
	ticker     *time.Ticker
	db         *JSONDatabase
}

func NewProxyPool(config models.ProxyPoolConfig, checker *ProxyChecker) *ProxyPool {
	return &ProxyPool{
		proxies:  make(map[string]*models.Proxy),
		config:   config,
		checker:  checker,
		logger:   zap.L(),
		stopTick: make(chan struct{}),
	}
}

func (pp *ProxyPool) SetDatabase(db *JSONDatabase) {
	pp.db = db
}

func (pp *ProxyPool) Load(proxies []*models.Proxy) {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	pp.proxies = make(map[string]*models.Proxy)
	for _, p := range proxies {
		pp.proxies[p.ID] = p
	}
	pp.logger.Info("loaded proxies", zap.Int("count", len(proxies)))
}

func (pp *ProxyPool) GetConfig() models.ProxyPoolConfig {
	pp.mu.RLock()
	defer pp.mu.RUnlock()
	return pp.config
}

func (pp *ProxyPool) UpdateConfig(cfg models.ProxyPoolConfig) {
	pp.mu.Lock()
	pp.config = cfg
	pp.mu.Unlock()

	pp.restartTicker()
	pp.persist()
	pp.logger.Info("proxy config updated", zap.Any("config", cfg))
}

func (pp *ProxyPool) restartTicker() {
	pp.mu.RLock()
	interval := pp.config.AutoTestInterval
	enabled := pp.config.Enabled
	pp.mu.RUnlock()

	if pp.ticker != nil {
		pp.ticker.Stop()
		close(pp.stopTick)
		pp.ticker = nil
	}

	if !enabled || interval <= 0 {
		return
	}

	pp.stopTick = make(chan struct{})
	pp.ticker = time.NewTicker(time.Duration(interval) * time.Minute)
	ticker := pp.ticker
	stopCh := pp.stopTick

	go func() {
		for {
			select {
			case <-ticker.C:
				pp.TestAll()
			case <-stopCh:
				return
			}
		}
	}()
}

func (pp *ProxyPool) StopTicker() {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	if pp.ticker != nil {
		pp.ticker.Stop()
		pp.ticker = nil
		close(pp.stopTick)
		pp.stopTick = make(chan struct{})
	}
}

func (pp *ProxyPool) Import(rawURLs []string) (int, int, []string) {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	var errors []string
	imported := 0

	for _, raw := range rawURLs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		p, err := pp.parseProxyURL(raw)
		if err != nil {
			errors = append(errors, fmt.Sprintf("invalid proxy %q: %s", raw, err.Error()))
			continue
		}

		if pp.findByHostPort(p.Host, p.Port) != nil {
			errors = append(errors, fmt.Sprintf("duplicate proxy %s:%d", p.Host, p.Port))
			continue
		}

		pp.proxies[p.ID] = p
		imported++
	}

	pp.persist()
	pp.logger.Info("proxy import completed",
		zap.Int("imported", imported),
		zap.Int("errors", len(errors)),
	)
	return imported, len(errors), errors
}

// PreVerifiedProxy carries a verified proxy from the scraper into the pool
// with its check verdict + geo intact, so the pool doesn't re-test what was
// just validated 30 seconds ago and the UI shows "live" immediately.
type PreVerifiedProxy struct {
	Type      string
	Host      string
	Port      int
	Region    string
	LatencyMs int
}

func (pp *ProxyPool) ImportPreVerified(items []PreVerifiedProxy) int {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	imported := 0
	now := time.Now()
	for _, it := range items {
		if pp.findByHostPort(it.Host, it.Port) != nil {
			continue
		}
		p := &models.Proxy{
			ID:        generateProxyID(),
			Enabled:   true,
			Type:      it.Type,
			Host:      it.Host,
			Port:      it.Port,
			Status:    models.ProxyStatusLive,
			Region:    it.Region,
			LatencyMs: it.LatencyMs,
			LastCheck: now,
			CreatedAt: now,
		}
		pp.proxies[p.ID] = p
		imported++
	}
	pp.persist()
	pp.logger.Info("pre-verified proxy import completed", zap.Int("imported", imported))
	return imported
}

func (pp *ProxyPool) findByHostPort(host string, port int) *models.Proxy {
	for _, p := range pp.proxies {
		if p.Host == host && p.Port == port {
			return p
		}
	}
	return nil
}

func (pp *ProxyPool) parseProxyURL(raw string) (*models.Proxy, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}

	proxyType := "http"
	switch u.Scheme {
	case "https":
		proxyType = "https"
	case "socks5":
		proxyType = "socks5"
	case "http":
		proxyType = "http"
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	host := u.Hostname()
	port := 80
	if u.Port() != "" {
		fmt.Sscanf(u.Port(), "%d", &port)
	} else if proxyType == "socks5" {
		port = 1080
	} else {
		port = 80
	}

	username, password := "", ""
	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}

	id := generateProxyID()
	return &models.Proxy{
		ID:        id,
		Enabled:   true,
		Type:      proxyType,
		Host:      host,
		Port:      port,
		Username:  username,
		Password:  password,
		Status:    models.ProxyStatusUnknown,
		Region:    "",
		LatencyMs: 0,
		CreatedAt: time.Now(),
	}, nil
}

func (pp *ProxyPool) List() []*models.Proxy {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	result := make([]*models.Proxy, 0, len(pp.proxies))
	for _, p := range pp.proxies {
		result = append(result, p)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func (pp *ProxyPool) Get(id string) (*models.Proxy, error) {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	p, exists := pp.proxies[id]
	if !exists {
		return nil, fmt.Errorf("proxy not found: %s", id)
	}
	return p, nil
}

func (pp *ProxyPool) Toggle(id string, enabled bool) error {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	p, exists := pp.proxies[id]
	if !exists {
		return fmt.Errorf("proxy not found: %s", id)
	}
	p.Enabled = enabled
	pp.persist()
	return nil
}

func (pp *ProxyPool) Delete(id string) error {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if _, exists := pp.proxies[id]; !exists {
		return fmt.Errorf("proxy not found: %s", id)
	}
	delete(pp.proxies, id)
	pp.persist()
	return nil
}

func (pp *ProxyPool) BatchDelete(ids []string) int {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	deleted := 0
	for _, id := range ids {
		if _, exists := pp.proxies[id]; exists {
			delete(pp.proxies, id)
			deleted++
		}
	}
	if deleted > 0 {
		pp.persist()
	}
	return deleted
}

func (pp *ProxyPool) DeleteDead() int {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	deleted := 0
	for id, p := range pp.proxies {
		if p.Status == models.ProxyStatusDead {
			delete(pp.proxies, id)
			deleted++
		}
	}
	if deleted > 0 {
		pp.persist()
	}
	return deleted
}

func (pp *ProxyPool) TestAll() []models.ProxyTestResult {
	pp.mu.RLock()
	proxies := make([]*models.Proxy, 0, len(pp.proxies))
	for _, p := range pp.proxies {
		proxies = append(proxies, p)
	}
	pp.mu.RUnlock()

	pp.mu.RLock()
	testEndpoint := pp.config.TestEndpoint
	pp.mu.RUnlock()

	if testEndpoint == "" {
		testEndpoint = "https://google.com"
	}

	results := pp.checker.CheckBatch(proxies, testEndpoint)

	pp.mu.Lock()
	for _, r := range results {
		if p, ok := pp.proxies[r.ID]; ok {
			p.Status = r.Status
			p.LatencyMs = r.LatencyMs
			p.LastCheck = time.Now()
			if r.Region != "" {
				p.Region = r.Region
			}
			if r.Status == models.ProxyStatusDead {
				p.FailCount++
				if pp.config.AutoDeleteFailed && p.FailCount >= 3 {
					delete(pp.proxies, p.ID)
					pp.logger.Info("auto-deleted failed proxy", zap.String("proxy_id", p.ID), zap.String("host", p.Host))
				}
			} else {
				p.FailCount = 0
			}
		}
	}
	pp.persist()
	pp.mu.Unlock()

	return results
}

func (pp *ProxyPool) TestSingle(id string) (*models.ProxyTestResult, error) {
	p, err := pp.Get(id)
	if err != nil {
		return nil, err
	}

	pp.mu.RLock()
	testEndpoint := pp.config.TestEndpoint
	pp.mu.RUnlock()

	if testEndpoint == "" {
		testEndpoint = "https://google.com"
	}

	p.Status = models.ProxyStatusChecking
	result := pp.checker.Check(p, testEndpoint)

	pp.mu.Lock()
	p.Status = result.Status
	p.LatencyMs = result.LatencyMs
	p.LastCheck = time.Now()
	if result.Region != "" {
		p.Region = result.Region
	}
	if result.Status == models.ProxyStatusDead {
		p.FailCount++
	} else {
		p.FailCount = 0
	}
	pp.persist()
	pp.mu.Unlock()

	return &result, nil
}

func (pp *ProxyPool) GetBestProxy() *models.Proxy {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	var best *models.Proxy
	for _, p := range pp.proxies {
		if !p.Enabled || p.Status != models.ProxyStatusLive {
			continue
		}
		if best == nil || scoreProxyForRegistration(p) > scoreProxyForRegistration(best) {
			best = p
		}
	}
	return best
}

func scoreProxyForRegistration(p *models.Proxy) int {
	if p == nil || !p.Enabled || p.Status != models.ProxyStatusLive {
		return -1 << 30
	}

	score := 1000
	score += p.RegisterSuccessCount * 25
	score -= p.RegisterFailureCount * 15
	score -= p.CaptchaFailureCount * 30
	score -= p.FailCount * 20
	if p.LatencyMs > 0 {
		score -= p.LatencyMs / 10
	}
	return score
}

func (pp *ProxyPool) GetProxyCounts() map[string]int {
	pp.mu.RLock()
	defer pp.mu.RUnlock()

	counts := map[string]int{"total": 0, "live": 0, "dead": 0, "unknown": 0}
	for _, p := range pp.proxies {
		counts["total"]++
		switch p.Status {
		case models.ProxyStatusLive:
			counts["live"]++
		case models.ProxyStatusDead:
			counts["dead"]++
		default:
			counts["unknown"]++
		}
	}
	return counts
}

func (pp *ProxyPool) persist() {
	if pp.db == nil {
		return
	}
	list := make([]*models.Proxy, 0, len(pp.proxies))
	for _, p := range pp.proxies {
		list = append(list, p)
	}
	pp.db.Set(list)
	if err := pp.db.Save(); err != nil {
		pp.logger.Error("failed to save proxy data", zap.Error(err))
	}
}

func generateProxyID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}
