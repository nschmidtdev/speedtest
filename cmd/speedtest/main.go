package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"speedtest/internal/api"
	"speedtest/internal/engine"
	"speedtest/internal/scheduler"
	"speedtest/internal/storage"
	"speedtest/web"
)

func getPort() string {
	port := os.Getenv("SPEEDTEST_PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
}

func getDBPath() string {
	if p := os.Getenv("SPEEDTEST_DB"); p != "" {
		return p
	}
	return "speedtest.db"
}

func main() {
	port := getPort()
	dbPath := getDBPath()

	// Database
	db, err := storage.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := storage.SeedDefaults(db); err != nil {
		log.Fatalf("Failed to seed default profiles: %v", err)
	}
	log.Printf("Database initialized at %s", dbPath)

	// Engine
	eng := engine.NewSpeedtestEngine()

	// App State
	state := api.NewAppState(db, eng)

	// Scheduler
	sched := scheduler.New(db, state)
	state.Scheduler = sched
	if err := sched.Start(); err != nil {
		log.Printf("Warning: scheduler failed to start: %v", err)
	}

	mux := http.NewServeMux()

	// === API Routes ===
	mux.HandleFunc("/api/health", state.HealthHandler)
	mux.HandleFunc("/api/test/run", state.RunTestHandler)
	mux.HandleFunc("/api/test/status", state.TestStatusHandler)
	mux.HandleFunc("/api/results", state.ResultsHandler)
	mux.HandleFunc("/api/stats", state.StatsHandler)
	mux.HandleFunc("/api/servers", state.ServersHandler)
	mux.HandleFunc("/api/scheduler/status", state.SchedulerStatusHandler)
	mux.HandleFunc("/api/tariffs", state.TariffsHandler)
	mux.HandleFunc("/api/tariff-catalog", state.TariffCatalogHandler)
	mux.HandleFunc("/api/tariff-comparison", state.TariffComparisonHandler)
	mux.HandleFunc("/api/tariff-streak", state.TariffStreakHandler)
	mux.HandleFunc("/api/complaint", state.ComplaintHandler)
	mux.HandleFunc("/api/settings", state.SettingsHandler)
	// HTMX Partials
	mux.HandleFunc("/api/tariff/providers", state.TariffProvidersPartial)
	mux.HandleFunc("/api/tariff/options", state.TariffOptionsPartial)
	mux.HandleFunc("/api/profile/options", state.ProfileOptionsPartial)
	mux.HandleFunc("/api/tariffs/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			state.TariffDetailHandler(w, r)
		case http.MethodDelete:
			state.TariffDeleteHandler(w, r)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/profiles", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			state.ListProfilesHandler(w, r)
		case http.MethodPost:
			state.CreateProfileHandler(w, r)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/profiles/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/enable") || strings.HasSuffix(path, "/disable") {
			state.ProfileToggleHandler(w, r)
			return
		}
		state.ProfileDetailHandler(w, r)
	})

	// === SSE (Live-Push) ===
	mux.HandleFunc("/events", state.SSEHandler)

	// === Frontend: Dev-Mode (Festplatte) oder Prod (embed.FS) ===
	var webRoot fs.FS
	if os.Getenv("SPEEDTEST_DEV") != "" {
		// Dev: live von Festplatte — kein Neubau nach HTML/CSS/JS-Änderungen
		webRoot = os.DirFS("web")
		log.Println("DEV MODE: serving web/ from disk")
	} else {
		// Prod: aus der Binary
		webRoot, err = fs.Sub(web.Files, ".")
		if err != nil {
			log.Fatalf("Failed to create sub-FS for web: %v", err)
		}
	}
	fileServer := http.FileServer(http.FS(webRoot))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if f, err := webRoot.Open(path); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		r2 := r
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})

	// === HTTP Server with timeouts ===
	server := &http.Server{
		Addr:         port,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute, // long for SSE + speedtest
		IdleTimeout:  120 * time.Second,
	}

	// === Graceful Shutdown ===
	go func() {
		log.Printf("Speedtest server starting on http://localhost%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Give connections up to 15 seconds to finish
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Warning: forced shutdown: %v", err)
	}

	sched.Stop()
	log.Println("Server stopped")
}

// loggingMiddleware logs each HTTP request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		// Skip SSE and static asset noise
		if !strings.HasPrefix(r.URL.Path, "/events") {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
		}
	})
}
