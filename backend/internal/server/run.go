package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/qwenpi/qwenpi-go/internal/api"
	"github.com/qwenpi/qwenpi-go/internal/core"
	"github.com/qwenpi/qwenpi-go/internal/models"
	"github.com/qwenpi/qwenpi-go/internal/providers/geminiweb"
	"github.com/qwenpi/qwenpi-go/internal/providers/kiro"
	"github.com/qwenpi/qwenpi-go/internal/providers/opencodezen"
	"github.com/qwenpi/qwenpi-go/internal/providers/qwen"
)

type Options struct {
	Port          int
	DashboardPort int
	DevMode       bool
}

func Run(opts Options) error {
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	zap.ReplaceGlobals(logger)

	config, err := core.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if opts.Port != 0 {
		config.Port = opts.Port
	}

	logger.Info("starting qwen-go",
		zap.Int("api_port", config.Port),
		zap.Int("dashboard_port", opts.DashboardPort),
	)

	// ── Account pool ──
	accountStore := core.NewAccountStore(config.GetAccountsFile())
	accounts, err := accountStore.Load()
	if err != nil {
		logger.Warn("failed to load accounts, starting fresh", zap.Error(err))
	}
	pool := core.NewAccountPool()
	pool.Load(accounts)
	pool.StartRecoveryLoop()
	defer pool.StopRecoveryLoop()
	// Memory-primary, file-as-cadangan snapshot loop (BACKLOG #22). Writes
	// every 60s + on critical state transitions (Banned, CircuitOpen) so a
	// crash doesn't reset hard-error accounts back to Valid on restart.
	pool.StartSnapshotLoop(60*time.Second, accountStore)
	defer pool.StopSnapshotLoop()
	logger.Info("loaded accounts", zap.Int("count", len(accounts)))
	accountsDB := accountStore.DB()

	// ── Provider manager ──
	pm := core.NewProviderManager()
	qwenProvider := qwen.NewQwenProvider(pool)
	pm.Register(qwenProvider)

	// ── Registration engine ──
	var engine core.RegistrationEngine
	switch config.RegistrationEngine {
	case "rod":
		rodEngine := core.NewRodEngine()
		rodEngine.SetProxyProvider(func() (bool, string, string, string) { return api.GetProxyEnv() })
		engine = rodEngine
		logger.Info("registration engine: rod")
	default:
		pythonEngine := core.NewPythonEngine("python3", config.GetPythonRegisterScript())
		pythonEngine.SetProxyProvider(func() (bool, string, string, string) { return api.GetProxyEnv() })
		engine = pythonEngine
		logger.Info("registration engine: python")
	}
	batchManager := core.NewBatchManager(pool, engine, accountsDB)

	// ── Proxy pool ──
	proxyConfig := models.ProxyPoolConfig{
		Enabled: false, TestEndpoint: "https://google.com",
		RotationStrategy: "fastest", AutoTestInterval: 5,
		AutoDeleteFailed: false, FallbackDirect: true,
		AutoLogin: false, ApplyToProviders: []string{"qwen"},
		ApplyTo: models.ProxyApplyToConfig{
			BatchRegistration: true,
			ProviderCall:      false,
		},
	}
	liveCheck := core.NewLiveCheck(config.DataDir)
	proxyChecker := core.NewProxyChecker(10*time.Second, liveCheck)
	proxyPoolManager := core.NewProxyPool(proxyConfig, proxyChecker)
	proxyStore := core.NewProxyStore(config.GetProxiesFile())
	proxies, err := proxyStore.Load()
	if err != nil {
		logger.Warn("failed to load proxies, starting fresh", zap.Error(err))
	}
	proxyPoolManager.Load(proxies)
	proxyDB := proxyStore.DB()
	proxyPoolManager.SetDatabase(proxyDB)
	defer proxyPoolManager.StopTicker()

	proxyScraper := core.NewProxyScraperManager(proxyPoolManager, config.DataDir, liveCheck)
	api.InitProxyScraperHandler(proxyScraper)

	// Wire per-request proxy decisions for Qwen's outbound chat/image/video
	// calls. Honoured only when ApplyTo.ProviderCall=true. FallbackDirect
	// controls behavior when the toggle is on but no live proxy is available.
	qwenProvider.SetProxyResolver(func() (*models.Proxy, error) {
		cfg := proxyPoolManager.GetConfig()
		if !cfg.Enabled || !cfg.ApplyTo.ProviderCall {
			return nil, nil
		}
		p := proxyPoolManager.GetBestProxy()
		if p != nil {
			return p, nil
		}
		if cfg.FallbackDirect {
			return nil, nil
		}
		return nil, fmt.Errorf("no live proxy available and fallback_direct=false")
	})

	// ── Misc services ──
	km := core.NewKeyManager(config.GetAPIKeysFile())
	api.InitKeyManager(km)
	sm := core.NewSettingsManager(config.GetSettingsFile())
	api.InitSettingsManager(sm)
	core.GlobalProviderRegistry = core.NewProviderRegistry(config.GetProviderConfigsFile())
	logger.Info("loaded provider configs", zap.Int("count", len(core.GlobalProviderRegistry.List())))
	pm.Register(opencodezen.NewProvider(pool, core.GlobalProviderRegistry))
	pm.Register(geminiweb.NewProvider(pool, core.GlobalProviderRegistry))
	pm.Register(kiro.NewProvider(pool, core.GlobalProviderRegistry))
	logger.Info("registered providers", zap.Strings("providers", pm.ListProviders()))
	api.InitChatHandler(pm)
	api.InitBatchHandler(batchManager)
	api.InitProviderHandler(pool)
	api.InitAccountHandler(pool, accountsDB)
	api.InitProxyHandler(proxyPoolManager)
	usageDB, err := core.OpenDB(config.GetUsageDB())
	if err != nil {
		logger.Warn("failed to open usage db, usage tracking disabled", zap.Error(err))
	}
	usageTracker := core.NewUsageTracker(usageDB)
	api.InitUsageTracker(usageTracker)
	defer usageTracker.Close()
	requestLogTracker := core.NewRequestLogTracker(usageDB)
	defer requestLogTracker.Stop()

	api.InitHistoryHandler(config.GetHistoryImagesFile(), config.GetHistoryVideosFile())

	// ── API server ──
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key")
		if c.Request.Method == "OPTIONS" { c.AbortWithStatus(204); return }
		c.Next()
	})
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "2.0.0-go"}) })
	r.GET("/", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"name": "Qwenpi API Gateway", "version": "2.0.0-go", "status": "running"}) })
	api.RegisterAdminRoutes(r, pool)
	api.RegisterChatRoutes(r, pm)
	api.RegisterImageRoutes(r, pm)
	api.RegisterBatchRoutes(r, batchManager)
	api.RegisterProviderRoutes(r, pool)
	api.RegisterAccountRoutes(r, pool, accountsDB)
	api.RegisterProxyRoutes(r, proxyPoolManager)
	api.RegisterProxyScraperRoutes(r, proxyScraper)
	api.RegisterAuthRoutes(r)
	api.RegisterUsageRoutes(r)
	api.RegisterHistoryRoutes(r)
	api.RegisterClaudeRoutes(r)
	api.RegisterGeminiRoutes(r)
	api.RegisterResponsesRoutes(r)
	api.RegisterEmbeddingsRoutes(r)
	api.RegisterModelsRoutes(r)

	apiPort := fmt.Sprintf(":%d", config.Port)
	apiSrv := &http.Server{Addr: apiPort, Handler: r}
	go func() {
		logger.Info("API server starting", zap.String("port", apiPort))
		if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("API server failed", zap.Error(err))
		}
	}()

	// ── Dashboard server (dual port) ──
	if opts.DashboardPort > 0 && opts.DashboardPort != config.Port {
		dashMux := http.NewServeMux()

		// Proxy /api/* to the API server with path-split auth:
		//   /api/admin/*  → qwenpi_key  (JWT, AdminMiddleware)
		//   /api/auth/*   → qwenpi_key  (harmless; login itself is unauthed)
		//   everything else (/api/v1/*, /api/chat/*, /api/completions) → qwenpi_apikey
		// Mirrors the path-split in frontend/app/api/[...proxy]/route.ts so the
		// embedded dashboard behaves the same as Next.js dev.
		apiURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", config.Port))
		proxy := httputil.NewSingleHostReverseProxy(apiURL)
		origDirector := proxy.Director
		proxy.Director = func(r *http.Request) {
			origDirector(r)
			if r.Header.Get("Authorization") != "" || r.Header.Get("x-api-key") != "" {
				return
			}
			cookieName := "qwenpi_apikey"
			if strings.HasPrefix(r.URL.Path, "/api/admin/") || strings.HasPrefix(r.URL.Path, "/api/auth/") {
				cookieName = "qwenpi_key"
			}
			if ck, err := r.Cookie(cookieName); err == nil && ck.Value != "" {
				r.Header.Set("x-api-key", ck.Value)
			}
		}
		dashMux.Handle("/api/", proxy)

		// Serve embedded frontend
		var dashHandler http.Handler
		if opts.DevMode {
			dashHandler = DevDashboardHandler()
			if dashHandler != nil {
				logger.Info("dashboard: dev mode, serving from frontend/out/")
			}
		}
		if dashHandler == nil && DashboardExists() {
			dashHandler = DashboardHandler()
			logger.Info("dashboard: embedded mode", zap.Int("port", opts.DashboardPort))
		}
		if dashHandler != nil {
			dashMux.Handle("/", dashHandler)
			dashPort := fmt.Sprintf(":%d", opts.DashboardPort)
			dashSrv := &http.Server{Addr: dashPort, Handler: dashMux}
			go func() {
				logger.Info("Dashboard server starting", zap.String("port", dashPort))
				if err := dashSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Error("Dashboard server failed", zap.Error(err))
				}
			}()
			defer dashSrv.Close()
		} else {
			logger.Warn("dashboard: not available — run scripts/build-frontend.sh first")
		}
	}

	// ── Wait for shutdown ──
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiSrv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", zap.Error(err))
	}

	data := pool.ListAccounts()
	accountsDB.Set(data)
	if err := accountsDB.Save(); err != nil {
		logger.Error("failed to save accounts", zap.Error(err))
	}
	proxyData := proxyPoolManager.List()
	proxyDB.Set(proxyData)
	if err := proxyDB.Save(); err != nil {
		logger.Error("failed to save proxies", zap.Error(err))
	}

	logger.Info("server stopped")
	return nil
}

// Legacy Run with no options (default port 1440)
func RunLegacy() error {
	return Run(Options{Port: 1440, DashboardPort: 1441})
}
