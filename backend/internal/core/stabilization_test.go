package core

import (
	"testing"
	"time"

	"github.com/qwenpi/qwenpi-go/internal/models"
)

func TestAccountPoolStopRecoveryLoopClosesSignal(t *testing.T) {
	pool := NewAccountPool()
	pool.StartRecoveryLoop()
	pool.StopRecoveryLoop()

	select {
	case <-pool.RecoveryStopped():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("RecoveryStopped() not closed")
	}
}

func TestProxyPoolStopTickerStopsBackgroundLoop(t *testing.T) {
	pool := NewProxyPool(models.ProxyPoolConfig{Enabled: true, AutoTestInterval: 1}, NewProxyChecker(time.Second, nil))
	pool.restartTicker()
	pool.StopTicker()
	if pool.ticker != nil {
		t.Fatal("ticker not cleared")
	}
}
