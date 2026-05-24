package core

import (
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
	"go.uber.org/zap"
)

const (
	geoipCityFile = "dbip-city-lite.mmdb"
	geoipASNFile  = "dbip-asn-lite.mmdb"
	geoipMaxAge   = 35 * 24 * time.Hour
)

type GeoLookup struct {
	dir    string
	city   *geoip2.Reader
	asn    *geoip2.Reader
	mu     sync.RWMutex
	logger *zap.Logger
}

func NewGeoLookup(dataDir string) *GeoLookup {
	g := &GeoLookup{
		dir:    filepath.Join(dataDir, "geoip"),
		logger: zap.L(),
	}
	_ = os.MkdirAll(g.dir, 0o755)
	g.tryOpen()
	go g.ensureFresh()
	return g
}

func (g *GeoLookup) tryOpen() {
	cityPath := filepath.Join(g.dir, geoipCityFile)
	asnPath := filepath.Join(g.dir, geoipASNFile)

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.city != nil {
		g.city.Close()
		g.city = nil
	}
	if g.asn != nil {
		g.asn.Close()
		g.asn = nil
	}

	if r, err := geoip2.Open(cityPath); err == nil {
		g.city = r
	}
	if r, err := geoip2.Open(asnPath); err == nil {
		g.asn = r
	}
}

func (g *GeoLookup) Ready() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.city != nil && g.asn != nil
}

func (g *GeoLookup) Lookup(ip net.IP) (country, city, isp string) {
	if ip == nil {
		return "", "", ""
	}
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.city != nil {
		if rec, err := g.city.City(ip); err == nil {
			country = rec.Country.IsoCode
			if name, ok := rec.City.Names["en"]; ok {
				city = name
			}
		}
	}
	if g.asn != nil {
		if rec, err := g.asn.ASN(ip); err == nil {
			isp = rec.AutonomousSystemOrganization
		}
	}
	return
}

func (g *GeoLookup) ensureFresh() {
	cityPath := filepath.Join(g.dir, geoipCityFile)
	asnPath := filepath.Join(g.dir, geoipASNFile)

	needCity := !fileFreshEnough(cityPath, geoipMaxAge)
	needASN := !fileFreshEnough(asnPath, geoipMaxAge)

	if !needCity && !needASN {
		return
	}

	g.logger.Info("downloading geoip databases (offline geo)",
		zap.Bool("city", needCity), zap.Bool("asn", needASN))

	if needCity {
		if err := g.downloadMonthly("dbip-city-lite", cityPath); err != nil {
			g.logger.Error("city db download failed", zap.Error(err))
		} else {
			g.logger.Info("city db downloaded", zap.String("path", cityPath))
		}
	}
	if needASN {
		if err := g.downloadMonthly("dbip-asn-lite", asnPath); err != nil {
			g.logger.Error("asn db download failed", zap.Error(err))
		} else {
			g.logger.Info("asn db downloaded", zap.String("path", asnPath))
		}
	}

	g.tryOpen()
}

func fileFreshEnough(path string, maxAge time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < maxAge
}

func (g *GeoLookup) downloadMonthly(slug, destPath string) error {
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		t := now.AddDate(0, -i, 0)
		urlStr := fmt.Sprintf("https://download.db-ip.com/free/%s-%04d-%02d.mmdb.gz",
			slug, t.Year(), int(t.Month()))
		if err := downloadGzipped(urlStr, destPath); err == nil {
			if _, err := geoip2.Open(destPath); err == nil {
				return nil
			}
			_ = os.Remove(destPath)
		}
	}
	return fmt.Errorf("no fresh db-ip release found for %s", slug)
}

func downloadGzipped(urlStr, destPath string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(urlStr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()

	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, gz); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, destPath)
}

func (g *GeoLookup) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.city != nil {
		g.city.Close()
		g.city = nil
	}
	if g.asn != nil {
		g.asn.Close()
		g.asn = nil
	}
}
