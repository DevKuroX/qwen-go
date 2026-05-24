package core

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
)

// Curated source list. Quality-tracked + auto-disabled when alive rate stays
// below threshold (see updateStats). Persisted to data/proxy_sources.json so
// admin overrides survive restarts.
var defaultProxySources = []*models.ProxySource{
	{URL: "https://vwmhbpgwhfwuwtattset.supabase.co/functions/v1/fetch-proxies", Enabled: true, SourceType: "supabase",
		APIKey: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6InZ3bWhicGd3aGZ3dXd0YXR0c2V0Iiwicm9sZSI6ImFub24iLCJpYXQiOjE3NjczMjc0NjYsImV4cCI6MjA4MjkwMzQ2Nn0.LSMD2P4whDzoIW4UCig0ly0j6UOxd5fHhIkUhywnmrg"},
	{URL: "https://raw.githubusercontent.com/TheSpeedX/SOCKS-List/master/http.txt", Enabled: true},
	{URL: "https://raw.githubusercontent.com/TheSpeedX/SOCKS-List/master/socks5.txt", Enabled: true},
	{URL: "https://raw.githubusercontent.com/TheSpeedX/SOCKS-List/master/socks4.txt", Enabled: true},
	{URL: "https://api.proxyscrape.com/v2/?request=displayproxies&protocol=http&timeout=10000&country=all", Enabled: true},
	{URL: "https://api.proxyscrape.com/v2/?request=displayproxies&protocol=socks5&timeout=10000&country=all", Enabled: true},
	{URL: "https://raw.githubusercontent.com/jetkai/proxy-list/main/online-proxies/txt/proxies-http.txt", Enabled: true},
	{URL: "https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/http.txt", Enabled: true},
	{URL: "https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/socks5.txt", Enabled: true},
	{URL: "https://proxylist.geonode.com/api/proxy-list?limit=500&sort_by=lastChecked&sort_type=desc&filterUpTime=90", Enabled: true, SourceType: "geonode"},
}

const (
	tcpProbeTimeout    = 2 * time.Second
	httpProbeTimeout   = 5 * time.Second
	staleStagingWindow = 15 * time.Minute
	tcpWorkers         = 500
	httpWorkers        = 200
	pipelineBufSize    = 1000

	traceTarget = "https://www.cloudflare.com/cdn-cgi/trace"
)

var traceIPRegex = regexp.MustCompile(`(?m)^ip=([0-9a-fA-F:.]+)\s*$`)

type ProxyScraperManager struct {
	jobs        map[string]*models.ScrapeJob
	cancelFuncs map[string]context.CancelFunc
	staging     map[string]*models.ScrapedProxy
	sources     []*models.ProxySource
	mu          sync.RWMutex
	pool        *ProxyPool
	logger      *zap.Logger
	store       *ProxySourceStore
	live        *LiveCheck
}

func NewProxyScraperManager(pool *ProxyPool, dataDir string, live *LiveCheck) *ProxyScraperManager {
	store := NewProxySourceStore(filepath.Join(dataDir, "proxy_sources.json"))
	sources := store.Load(defaultProxySources)

	return &ProxyScraperManager{
		jobs:        make(map[string]*models.ScrapeJob),
		cancelFuncs: make(map[string]context.CancelFunc),
		staging:     make(map[string]*models.ScrapedProxy),
		sources:     sources,
		pool:        pool,
		logger:      zap.L(),
		store:       store,
		live:        live,
	}
}

func (psm *ProxyScraperManager) StartScrape(ctx context.Context, req models.ScrapeRequest) (string, error) {
	psm.mu.Lock()

	jobID := generateScrapeID()
	jobCtx, cancel := context.WithCancel(ctx)

	var sources []*models.ProxySource
	if len(req.Sources) > 0 {
		for _, u := range req.Sources {
			src := psm.findSourceByURL(u)
			if src != nil {
				sources = append(sources, src)
			} else {
				sources = append(sources, &models.ProxySource{URL: u, Enabled: true})
			}
		}
	} else {
		for _, s := range psm.sources {
			if s.Enabled {
				sources = append(sources, s)
			}
		}
	}

	if len(sources) == 0 {
		psm.mu.Unlock()
		cancel()
		return "", fmt.Errorf("no enabled sources to scrape")
	}

	sourceURLs := make([]string, len(sources))
	for i, s := range sources {
		sourceURLs[i] = s.URL
	}

	job := &models.ScrapeJob{
		ID:        jobID,
		Status:    models.ScrapeStatusRunning,
		Sources:   sourceURLs,
		Logs:      make([]models.LogEntry, 0),
		StartedAt: time.Now(),
	}

	psm.jobs[jobID] = job
	psm.cancelFuncs[jobID] = cancel
	psm.mu.Unlock()

	go psm.runScrape(jobCtx, cancel, job, sources, req.Count)

	return jobID, nil
}

func (psm *ProxyScraperManager) findSourceByURL(u string) *models.ProxySource {
	for _, s := range psm.sources {
		if s.URL == u {
			return s
		}
	}
	return nil
}

// rawProxy carries a candidate through the pipeline.
type rawProxy struct {
	host    string
	port    int
	pType   string
	country string // pre-filled by structured sources (supabase, geonode)
	city    string
	isp     string
	source  *models.ProxySource
}

func (rp rawProxy) key() string { return fmt.Sprintf("%s:%d", rp.host, rp.port) }

func (psm *ProxyScraperManager) runScrape(ctx context.Context, cancel context.CancelFunc, job *models.ScrapeJob, sources []*models.ProxySource, perSourceCap int) {
	defer func() {
		if r := recover(); r != nil {
			job.AddLog("error", fmt.Sprintf("Scrape panicked: %v", r))
			job.Status = models.ScrapeStatusFailed
			now := time.Now()
			job.CompletedAt = &now
			psm.logger.Error("scrape panicked", zap.Any("recover", r), zap.String("job_id", job.ID))
		}
		psm.mu.Lock()
		delete(psm.cancelFuncs, job.ID)
		psm.mu.Unlock()
		cancel()
	}()

	job.AddLog("info", fmt.Sprintf("Starting scrape: %d sources (streaming pipeline)", len(sources)))
	if !psm.live.Geo().Ready() {
		job.AddLog("warning", "Geo DB not yet downloaded; proxies will be marked live without country/isp until it lands")
	}

	var (
		totalFound  int64
		totalAlive  int64
		totalFailed int64
		jobAliveOK  int64
	)

	// per-source counters for stats persistence
	srcFound := make(map[string]*int64, len(sources))
	srcAlive := make(map[string]*int64, len(sources))
	for _, s := range sources {
		var f, a int64
		srcFound[s.URL] = &f
		srcAlive[s.URL] = &a
	}

	rawCh := make(chan rawProxy, pipelineBufSize)
	dedupedCh := make(chan rawProxy, pipelineBufSize)
	tcpOKCh := make(chan rawProxy, pipelineBufSize)

	// Stage 1: fetchers (one goroutine per source, no concurrency cap — sources are I/O bound and few)
	var fetcherWG sync.WaitGroup
	for _, src := range sources {
		fetcherWG.Add(1)
		go func(s *models.ProxySource) {
			defer fetcherWG.Done()
			n, err := psm.fetchSourceInto(ctx, s, perSourceCap, rawCh)
			if err != nil {
				atomic.AddInt64(&totalFailed, 1)
				job.AddLog("error", fmt.Sprintf("Source %s failed: %s", s.URL, err.Error()))
				return
			}
			atomic.AddInt64(srcFound[s.URL], int64(n))
			job.AddLog("info", fmt.Sprintf("Source %s: %d candidates", s.URL, n))
		}(src)
	}
	go func() {
		fetcherWG.Wait()
		close(rawCh)
	}()

	// Stage 2: dedupe (single goroutine, owns staging map writes for new entries)
	go func() {
		defer close(dedupedCh)
		for rp := range rawCh {
			select {
			case <-ctx.Done():
				return
			default:
			}
			key := rp.key()
			psm.mu.Lock()
			existing, exists := psm.staging[key]
			if !exists {
				psm.staging[key] = &models.ScrapedProxy{
					ID:      generateScrapeID(),
					Type:    rp.pType,
					Host:    rp.host,
					Port:    rp.port,
					Status:  "unknown",
					Country: rp.country,
					City:    rp.city,
					ISP:     rp.isp,
					Source:  rp.source.URL,
				}
				psm.mu.Unlock()
				atomic.AddInt64(&totalFound, 1)
				dedupedCh <- rp
				continue
			}
			// already in staging; skip re-check if recent
			if !existing.LastCheckedAt.IsZero() && time.Since(existing.LastCheckedAt) < staleStagingWindow {
				psm.mu.Unlock()
				continue
			}
			psm.mu.Unlock()
			dedupedCh <- rp
		}
	}()

	// Stage 3: TCP pre-check (fast-fail dead majority)
	var tcpWG sync.WaitGroup
	for i := 0; i < tcpWorkers; i++ {
		tcpWG.Add(1)
		go func() {
			defer tcpWG.Done()
			for rp := range dedupedCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if tcpReachable(rp.host, rp.port, tcpProbeTimeout) {
					tcpOKCh <- rp
				} else {
					psm.markStatus(rp, "dead", 0)
				}
			}
		}()
	}
	go func() {
		tcpWG.Wait()
		close(tcpOKCh)
	}()

	// Stage 4: HTTP forwarding test + inline geo (writes final status to staging)
	var httpWG sync.WaitGroup
	for i := 0; i < httpWorkers; i++ {
		httpWG.Add(1)
		go func() {
			defer httpWG.Done()
			for rp := range tcpOKCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				v := psm.live.Validate(rp.pType, rp.host, rp.port, "", "")
				if v.Alive {
					atomic.AddInt64(&totalAlive, 1)
					atomic.AddInt64(srcAlive[rp.source.URL], 1)
					atomic.AddInt64(&jobAliveOK, 1)
					psm.markLive(rp, v)
				} else {
					psm.markStatus(rp, "dead", v.LatencyMs)
				}
				job.Total = int(atomic.LoadInt64(&totalFound))
				job.Found = int(atomic.LoadInt64(&totalFound))
				job.Alive = int(atomic.LoadInt64(&totalAlive))
				job.Failed = int(atomic.LoadInt64(&totalFailed))
			}
		}()
	}
	httpWG.Wait()

	// finalize job stats
	job.Total = int(atomic.LoadInt64(&totalFound))
	job.Found = int(atomic.LoadInt64(&totalFound))
	job.Alive = int(atomic.LoadInt64(&totalAlive))
	job.Failed = int(atomic.LoadInt64(&totalFailed))

	select {
	case <-ctx.Done():
		job.Status = models.ScrapeStatusStopped
	default:
		job.Status = models.ScrapeStatusCompleted
	}
	now := time.Now()
	job.CompletedAt = &now

	// persist source stats
	psm.mu.Lock()
	for _, s := range psm.sources {
		if fp, ok := srcFound[s.URL]; ok {
			updateStats(s, int(atomic.LoadInt64(fp)), int(atomic.LoadInt64(srcAlive[s.URL])))
			if !s.Enabled {
				job.AddLog("warning", fmt.Sprintf("Source auto-disabled (alive rate %.1f%%): %s",
					s.Stats.AliveRate*100, s.URL))
			}
		}
	}
	sourcesCopy := cloneSources(psm.sources)
	psm.mu.Unlock()
	if err := psm.store.Save(sourcesCopy); err != nil {
		psm.logger.Warn("source stats persist failed", zap.Error(err))
	}

	job.AddLog("info", fmt.Sprintf("Scrape done: %d found, %d alive, %d sources",
		job.Found, job.Alive, len(sources)))
}

func (psm *ProxyScraperManager) markStatus(rp rawProxy, status string, latency int) {
	psm.mu.Lock()
	if existing, ok := psm.staging[rp.key()]; ok {
		existing.Status = status
		existing.LatencyMs = latency
		existing.LastCheckedAt = time.Now()
	}
	psm.mu.Unlock()
}

// markLive applies a live verdict to the staging entry. Source-provided geo
// (supabase/geonode) wins over MMDB; only fill from the verdict when empty.
func (psm *ProxyScraperManager) markLive(rp rawProxy, v Verdict) {
	country, city, isp := rp.country, rp.city, rp.isp
	if country == "" {
		country = v.Country
	}
	if city == "" {
		city = v.City
	}
	if isp == "" {
		isp = v.ISP
	}
	psm.mu.Lock()
	if existing, ok := psm.staging[rp.key()]; ok {
		existing.Status = "live"
		existing.LatencyMs = v.LatencyMs
		existing.LastCheckedAt = time.Now()
		if country != "" {
			existing.Country = country
		}
		if city != "" {
			existing.City = city
		}
		if isp != "" {
			existing.ISP = isp
		}
	}
	psm.mu.Unlock()
}

func tcpReachable(host string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (psm *ProxyScraperManager) fetchSourceInto(ctx context.Context, source *models.ProxySource, cap int, out chan<- rawProxy) (int, error) {
	switch source.SourceType {
	case "supabase":
		return psm.fetchSupabaseSource(ctx, source, cap, out)
	case "geonode":
		return psm.fetchGeoNodeSource(ctx, source, cap, out)
	default:
		return psm.fetchTextSource(ctx, source, cap, out)
	}
}

var ipPortRegex = regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):(\d{2,5})`)

func (psm *ProxyScraperManager) fetchTextSource(ctx context.Context, source *models.ProxySource, cap int, out chan<- rawProxy) (int, error) {
	body, err := fetchURL(ctx, source.URL, 15*time.Second)
	if err != nil {
		return 0, err
	}
	pType := detectProxyType(source.URL)
	count := 0

	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if cap > 0 && count >= cap {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		m := ipPortRegex.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		var port int
		fmt.Sscanf(m[2], "%d", &port)
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		case out <- rawProxy{host: m[1], port: port, pType: pType, source: source}:
			count++
		}
	}
	return count, nil
}

func (psm *ProxyScraperManager) fetchSupabaseSource(ctx context.Context, source *models.ProxySource, cap int, out chan<- rawProxy) (int, error) {
	limit := 9999
	if cap > 0 {
		limit = cap
	}
	body := fmt.Sprintf(`{"limit":%d}`, limit)
	if source.APIKey == "" {
		return 0, fmt.Errorf("missing API key for supabase source")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, source.URL, strings.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("apikey", source.APIKey)
	req.Header.Set("Authorization", "Bearer "+source.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var apiResp struct {
		Success bool `json:"success"`
		Proxies []struct {
			IP      string `json:"ip"`
			Port    int    `json:"port"`
			Type    string `json:"type"`
			Country string `json:"country"`
		} `json:"proxies"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return 0, err
	}
	if !apiResp.Success {
		return 0, fmt.Errorf("supabase API returned success=false")
	}

	count := 0
	for _, p := range apiResp.Proxies {
		if cap > 0 && count >= cap {
			break
		}
		country := p.Country
		if country == "Unknown" {
			country = ""
		}
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		case out <- rawProxy{host: p.IP, port: p.Port, pType: strings.ToLower(p.Type), country: country, source: source}:
			count++
		}
	}
	return count, nil
}

// GeoNode JSON response shape (partial):
// { "data": [ { "ip": "...", "port": "...", "protocols": ["http"], "country": "US", "isp": "...", "city": "..." } ] }
func (psm *ProxyScraperManager) fetchGeoNodeSource(ctx context.Context, source *models.ProxySource, cap int, out chan<- rawProxy) (int, error) {
	body, err := fetchURL(ctx, source.URL, 30*time.Second)
	if err != nil {
		return 0, err
	}
	var apiResp struct {
		Data []struct {
			IP        string   `json:"ip"`
			Port      string   `json:"port"`
			Protocols []string `json:"protocols"`
			Country   string   `json:"country"`
			ISP       string   `json:"isp"`
			City      string   `json:"city"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return 0, err
	}
	count := 0
	for _, p := range apiResp.Data {
		if cap > 0 && count >= cap {
			break
		}
		var port int
		fmt.Sscanf(p.Port, "%d", &port)
		if port == 0 {
			continue
		}
		pType := "http"
		if len(p.Protocols) > 0 {
			pType = strings.ToLower(p.Protocols[0])
			if pType == "https" {
				pType = "http"
			}
		}
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		case out <- rawProxy{host: p.IP, port: port, pType: pType, country: p.Country, city: p.City, isp: p.ISP, source: source}:
			count++
		}
	}
	return count, nil
}

func makeTransport(proxyType, addr string) (*http.Transport, error) {
	return makeTransportAuth(proxyType, addr, "", "")
}

func makeTransportAuth(proxyType, addr, username, password string) (*http.Transport, error) {
	switch proxyType {
	case "socks5", "socks4":
		var auth *proxy.Auth
		if username != "" {
			auth = &proxy.Auth{User: username, Password: password}
		}
		d, err := proxy.SOCKS5("tcp", addr, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}
		return &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return d.Dial(network, addr)
			},
			TLSHandshakeTimeout: 5 * time.Second,
		}, nil
	default:
		var proxyURL *url.URL
		var err error
		if username != "" {
			proxyURL, err = url.Parse(fmt.Sprintf("http://%s:%s@%s",
				url.QueryEscape(username), url.QueryEscape(password), addr))
		} else {
			proxyURL, err = url.Parse(fmt.Sprintf("http://%s", addr))
		}
		if err != nil {
			return nil, err
		}
		return &http.Transport{
			Proxy:               http.ProxyURL(proxyURL),
			TLSHandshakeTimeout: 5 * time.Second,
		}, nil
	}
}

func fetchURL(ctx context.Context, urlStr string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func detectProxyType(rawURL string) string {
	lower := strings.ToLower(rawURL)
	if strings.Contains(lower, "socks5") {
		return "socks5"
	}
	if strings.Contains(lower, "socks4") {
		return "socks4"
	}
	return "http"
}

func generateScrapeID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (psm *ProxyScraperManager) GetJob(id string) (*models.ScrapeJob, error) {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	job, exists := psm.jobs[id]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return job, nil
}

func (psm *ProxyScraperManager) GetAllJobs() []*models.ScrapeJob {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	jobs := make([]*models.ScrapeJob, 0, len(psm.jobs))
	for _, job := range psm.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

func (psm *ProxyScraperManager) StopJob(id string) error {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	job, exists := psm.jobs[id]
	if !exists {
		return fmt.Errorf("job not found: %s", id)
	}
	if job.Status != models.ScrapeStatusRunning {
		return fmt.Errorf("job is not running: %s", job.Status)
	}

	cancel, exists := psm.cancelFuncs[id]
	if exists {
		cancel()
	}

	job.AddLog("warning", "Stop signal sent by user")
	job.Status = models.ScrapeStatusStopped
	now := time.Now()
	job.CompletedAt = &now

	return nil
}

func (psm *ProxyScraperManager) StreamLogs(id string) (<-chan models.LogEntry, error) {
	psm.mu.RLock()
	_, exists := psm.jobs[id]
	psm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	out := make(chan models.LogEntry, 100)

	go func() {
		defer close(out)
		lastIdx := 0
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			<-ticker.C

			psm.mu.RLock()
			currentJob := psm.jobs[id]
			psm.mu.RUnlock()

			if currentJob == nil {
				return
			}

			for i := lastIdx; i < len(currentJob.Logs); i++ {
				out <- currentJob.Logs[i]
			}
			lastIdx = len(currentJob.Logs)

			if currentJob.Status != models.ScrapeStatusRunning {
				return
			}
		}
	}()

	return out, nil
}

func (psm *ProxyScraperManager) GetStagingProxies(filter models.TransferRequest) []*models.ScrapedProxy {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	var result []*models.ScrapedProxy
	for _, sp := range psm.staging {
		if len(filter.IDs) > 0 {
			found := false
			for _, id := range filter.IDs {
				if sp.ID == id {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		} else if sp.Status != "live" {
			// staging area surfaces actionable proxies only; dead/unknown stay
			// in the internal map (used for re-check skip), not the UI.
			continue
		}
		if filter.Type != "" && sp.Type != filter.Type {
			continue
		}
		if filter.Country != "" && sp.Country != filter.Country {
			continue
		}
		if filter.MaxLatency > 0 && sp.LatencyMs > filter.MaxLatency {
			continue
		}
		if filter.MinLatency > 0 && sp.LatencyMs < filter.MinLatency {
			continue
		}
		result = append(result, sp)
	}
	return result
}

func (psm *ProxyScraperManager) TransferToProxyPool(req models.TransferRequest) (int, error) {
	proxies := psm.GetStagingProxies(req)
	if len(proxies) == 0 {
		return 0, fmt.Errorf("no proxies match the filter")
	}

	items := make([]PreVerifiedProxy, 0, len(proxies))
	for _, sp := range proxies {
		items = append(items, PreVerifiedProxy{
			Type:      sp.Type,
			Host:      sp.Host,
			Port:      sp.Port,
			Region:    sp.Country,
			LatencyMs: sp.LatencyMs,
		})
	}
	imported := psm.pool.ImportPreVerified(items)

	psm.mu.Lock()
	for _, sp := range proxies {
		key := fmt.Sprintf("%s:%d", sp.Host, sp.Port)
		if existing, ok := psm.staging[key]; ok && existing.ID == sp.ID {
			delete(psm.staging, key)
		}
	}
	psm.mu.Unlock()

	return imported, nil
}

func (psm *ProxyScraperManager) GetSources() []*models.ProxySource {
	psm.mu.RLock()
	defer psm.mu.RUnlock()
	return cloneSources(psm.sources)
}

func (psm *ProxyScraperManager) UpdateSources(sources []*models.ProxySource) {
	psm.mu.Lock()
	psm.sources = cloneSources(sources)
	snapshot := cloneSources(psm.sources)
	psm.mu.Unlock()
	if err := psm.store.Save(snapshot); err != nil {
		psm.logger.Warn("source persist failed", zap.Error(err))
	}
	psm.logger.Info("proxy sources updated", zap.Int("count", len(sources)))
}
