package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/go-chi/chi/v5"

	"vlesspanel/metrics"
)

func main() {
	config := LoadConfig()

	log.Println("========================================")
	log.Println("  VlessPanel v0.1")
	log.Println("========================================")
	log.Printf("  Port:            %s", config.Port)
	log.Printf("  Aggregator dir:  %s", config.AggregatorDir)
	log.Printf("  Panels file:     %s", config.PanelsFilePath)
	log.Printf("  Static dir:      %s", config.StaticDir)
	log.Printf("  VlessSubTest:    %s", config.VlessSubTestDaemonURL)
	log.Println("========================================")

	// Initialize storage
	storage := NewStorage(config.PanelsFilePath, config.AggregatorDir, config.DataDir)
	panelAPI := NewPanelAPI()
	syncState := NewSyncState()
	auth := NewTokenAuth(config.AdminToken)
	if auth.Enabled() {
		if tokens, err := storage.LoadTokens(); err == nil {
			hashes := make(map[string]struct{}, len(tokens))
			for _, t := range tokens {
				hashes[t.TokenHash] = struct{}{}
			}
			auth.SetIssued(hashes)
		}
	}

	// Раздел статистики: SQLite-хранилище метрик + коллектор.
	metricsDB, err := metrics.NewMetricsDB(config.MetricsDBPath, log.Default())
	if err != nil {
		log.Fatalf("metrics db: %v", err)
	}
	defer metricsDB.Close()

	// TG-алерты (Этап 2): проверка порогов при каждом цикле сбора коллектора.
	alertCfg := metrics.LoadAlertConfig()
	var alerts *metrics.AlertManager
	if alertCfg.Enabled {
		alerts = metrics.NewAlertManager(metricsDB,
			metrics.NewTGClient(alertCfg.BotToken, alertCfg.ChatID, alertCfg.SendTimeout, log.Default()),
			alertCfg, log.Default())
		log.Printf("  TG-алерты:     включены (RAM > %g%%, load1 > %g ядер×%.1f, трафик > %g×среднего за %s, тестер > %s, cooldown %s)",
			alertCfg.RAMThresholdPct, alertCfg.LoadCores, alertCfg.LoadFactor,
			alertCfg.TrafficMultiplier, alertCfg.TrafficWindow, alertCfg.StaleTesterAfter, alertCfg.Cooldown)
	} else {
		log.Printf("  TG-алерты:     выключены (нужны VLESSPANEL_TG_BOT_TOKEN и VLESSPANEL_TG_CHAT_ID)")
	}

	// Services (use-case layer).
	daemon := NewVlessSubTestClient(config.VlessSubTestDaemonURL)
	panels := NewPanelService(storage, panelAPI)
	subscriptions := NewSubscriptionService(storage, panelAPI, syncState, daemon)
	keySources := NewKeySourceService(storage, panelAPI, syncState, daemon)
	syncSvc := NewSyncService(syncState, NewScriptSyncer(config.SyncScript))
	tokens := NewTokenService(storage, auth)

	handlers := NewHandlers(auth, panels, subscriptions, keySources, syncSvc, tokens, daemon, config.PublicURL)
	metricsHandlers := metrics.NewMetricsHandlers(metricsDB)

	// Router
	r := chi.NewRouter()

	// Middleware
	r.Use(loggingMiddleware)

	// Без аутентификации (на корневом роутере, вне /api-группы с authMiddleware).
	r.Get("/api/auth-status", handlers.GetAuthStatus)
	r.Get("/api/config", handlers.GetConfig)

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Use(authMiddleware(auth))

		// Panels
		r.Get("/panels", handlers.ListPanels)
		r.Post("/panels", handlers.CreatePanel)
		r.Get("/panels/{id}/clients", handlers.ListClients)
		r.Post("/panels/{id}/clients", handlers.CreateClient)
		r.Get("/panels/{id}/clients/{email}/keys", handlers.GetClientKeys)
		r.Post("/panels/{id}/clients/{email}/attach", handlers.AttachInbound)
		r.Post("/panels/{id}/clients/{email}/detach", handlers.DetachInbound)
		r.Post("/panels/{id}/clients/{email}/update", handlers.UpdateClient)
		r.Post("/panels/{id}/inbounds", handlers.ListInbounds)
		r.Delete("/panels/{id}", handlers.DeletePanel)

		// Subscriptions
		r.Get("/subscriptions", handlers.ListSubscriptions)
		r.Post("/subscriptions", handlers.CreateSubscription)
		r.Post("/subscriptions/regenerate-all", handlers.RegenerateAllSubscriptions)
		r.Get("/subscriptions/{id}", handlers.GetSubscription)
		r.Put("/subscriptions/{id}", handlers.UpdateSubscription)
		r.Delete("/subscriptions/{id}", handlers.DeleteSubscription)
		r.Get("/subscriptions/{id}/raw", handlers.GetSubscriptionRaw)
		r.Post("/subscriptions/{id}/test", handlers.TestSubscription)

		// Key sources
		r.Get("/key-sources", handlers.ListKeySources)
		r.Post("/key-sources", handlers.CreateKeySource)
		r.Get("/key-sources/{id}", handlers.GetKeySource)
		r.Delete("/key-sources/{id}", handlers.DeleteKeySource)
		r.Get("/key-sources/{id}/key", handlers.GetKeySourceKey)
		r.Get("/key-sources/{id}/test", handlers.TestKeySource)
		r.Get("/key-sources/{id}/traffic", handlers.GetKeySourceTraffic)

		// Sync with the aggregator
		r.Post("/sync", handlers.SyncToAggregator)

		// Utility
		r.Get("/vlesssubtest-status", handlers.GetVlessSubTestStatus)

		// Статистика (Этап 1: коллектор + БД + API метрик, без UI)
		r.Get("/metrics/testers", metricsHandlers.Testers)
		r.Get("/metrics/snapshots", metricsHandlers.Snapshots)
		r.Get("/metrics/traffic", metricsHandlers.Traffic)
		r.Get("/metrics/test-runs", metricsHandlers.TestRuns)
		r.Get("/metrics/test-runs/{id}", metricsHandlers.TestRunDetail)
		r.Get("/metrics/availability", metricsHandlers.Availability)

		// API tokens (admin only)
		r.Route("/tokens", func(r chi.Router) {
			r.Use(requireAdmin)
			r.Get("/", handlers.ListTokens)
			r.Post("/", handlers.CreateToken)
			r.Delete("/{id}", handlers.DeleteToken)
		})
	})

	// Serve static files
	staticDir := config.StaticDir
	if _, err := os.Stat(staticDir); err == nil {
		fileServer := http.FileServer(http.Dir(staticDir))
		// SPA fallback: serve real files, otherwise index.html
		r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" || path == "index.html" {
				http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
				return
			}
			if _, err := os.Stat(filepath.Join(staticDir, path)); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
		}))
	} else {
		log.Printf("Static directory %s not found, API-only mode", staticDir)
	}

	// Start server
	addr := ":" + config.Port
	log.Printf("Starting server on %s", addr)

	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
	}

	// Graceful shutdown: по SIGINT/SIGTERM (docker stop) дожидаемся завершения
	// текущих запросов в пределах config.ShutdownTimeout.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Коллектор метрик: телеметрия панелей (5 мин) + прогоны тестов (6ч :15)
	// + retention (сутки). Останавливается вместе с сервером по ctx.
	collector := metrics.NewMetricsCollector(metricsDB, storage, panelAPI, panelAPI, daemon,
		config.VlessSubTestDaemonURL, log.Default())
	collector.SetAlerts(alerts) // nil = алерты выключены
	collector.Start(ctx)

	go func() {
		<-ctx.Done()
		log.Println("shutdown signal received, draining connections...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("server stopped")
}
