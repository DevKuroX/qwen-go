package core

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// LiveCheck owns the shared verdict logic used by both the scraper pipeline
// and the on-demand pool tests: TCP reachability → cdn-cgi/trace forwarding
// test → offline geo lookup. Sharing one implementation keeps "live" meaning
// the same thing whether a proxy was just scraped or is being re-tested in
// the pool an hour later.
type LiveCheck struct {
	geo      *GeoLookup
	mu       sync.RWMutex
	localIP  string
	logger   *zap.Logger
}

func NewLiveCheck(dataDir string) *LiveCheck {
	lc := &LiveCheck{
		geo:    NewGeoLookup(dataDir),
		logger: zap.L(),
	}
	go lc.refreshLocalIP()
	return lc
}

func (lc *LiveCheck) Geo() *GeoLookup { return lc.geo }

func (lc *LiveCheck) LocalIP() string {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.localIP
}

func (lc *LiveCheck) refreshLocalIP() {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(traceTarget)
	if err != nil {
		lc.logger.Warn("local IP probe failed", zap.Error(err))
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if m := traceIPRegex.FindStringSubmatch(string(body)); len(m) == 2 {
		lc.mu.Lock()
		lc.localIP = m[1]
		lc.mu.Unlock()
		lc.logger.Info("local IP cached for forwarding-test", zap.String("ip", m[1]))
	}
}

// Verdict bundles everything the caller needs after one full check.
type Verdict struct {
	Alive     bool
	LatencyMs int
	Country   string
	City      string
	ISP       string
}

// Validate runs TCP pre-check → HTTP forwarding test → inline geo.
// Username/password are passed through to makeTransport when present.
func (lc *LiveCheck) Validate(proxyType, host string, port int, username, password string) Verdict {
	if !tcpReachable(host, port, tcpProbeTimeout) {
		return Verdict{}
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	transport, err := makeTransportAuth(proxyType, addr, username, password)
	if err != nil {
		return Verdict{}
	}
	client := &http.Client{Transport: transport, Timeout: httpProbeTimeout}

	start := time.Now()
	resp, err := client.Get(traceTarget)
	if err != nil {
		return Verdict{}
	}
	defer resp.Body.Close()
	latency := int(time.Since(start).Milliseconds())

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return Verdict{LatencyMs: latency}
	}
	m := traceIPRegex.FindStringSubmatch(string(body))
	if len(m) != 2 {
		return Verdict{LatencyMs: latency}
	}
	seenIP := m[1]

	local := lc.LocalIP()
	if local != "" && seenIP == local {
		// transparent proxy — didn't actually egress through a different IP
		return Verdict{LatencyMs: latency}
	}

	v := Verdict{Alive: true, LatencyMs: latency}
	if ip := net.ParseIP(host); ip != nil {
		v.Country, v.City, v.ISP = lc.geo.Lookup(ip)
	}
	return v
}
