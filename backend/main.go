package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

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
	storage := NewStorage(config.PanelsFilePath, config.AggregatorDir)
	panelAPI := NewPanelAPI()
	handlers := NewHandlers(storage, panelAPI, config)

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
		r.Get("/subscriptions/{id}", handlers.GetSubscription)
		r.Put("/subscriptions/{id}", handlers.UpdateSubscription)
		r.Delete("/subscriptions/{id}", handlers.DeleteSubscription)
		r.Get("/subscriptions/{id}/raw", handlers.GetSubscriptionRaw)
		r.Post("/subscriptions/{id}/test", handlers.TestSubscription)

		// Utility
		r.Get("/vlesssubtest-status", handlers.GetVlessSubTestStatus)
	})

	// Serve static files
	staticDir := config.StaticDir
	if _, err := os.Stat(staticDir); err == nil {
		fileServer := http.FileServer(http.Dir(staticDir))
		r.Handle("/*", fileServer)

		// Also serve index.html for SPA routing
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
		})
	} else {
		log.Printf("Static directory %s not found, API-only mode", staticDir)
	}

	// Start server
	addr := ":" + config.Port
	log.Printf("Starting server on %s", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
