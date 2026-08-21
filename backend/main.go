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
	handlers := NewHandlers(storage, panelAPI, config, syncState)

	// Router
	r := chi.NewRouter()

	// Middleware
	r.Use(corsMiddleware)
	r.Use(loggingMiddleware)

	// API routes
	r.Route("/api", func(r chi.Router) {
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
