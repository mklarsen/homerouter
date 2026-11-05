package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

// newHandler serves the peer API only when svc is set.
func newHandler(cfg Config, svc *Service, backend Backend) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
	})
	mux.HandleFunc("GET /api/v1/brand", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, cfg.Brand)
	})
	if svc != nil {
		(&api{cfg: cfg, svc: svc, backend: backend}).register(mux)
	}

	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	cfg, err := LoadConfig(osLookup)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var svc *Service
	var backend Backend
	if cfg.APIToken != "" {
		if cfg.Backend == "wg" {
			backend = newWGBackend(cfg.Iface, cfg.Port, cfg.Pool.ServerAddr(), cfg.DataDir)
		} else {
			backend = newFakeBackend()
		}
		if err := backend.Ensure(ctx); err != nil {
			log.Fatalf("backend: %v", err)
		}
		svc = NewService(cfg.DataDir, cfg.Pool, backend)
		if err := svc.Restore(ctx); err != nil {
			log.Fatalf("restore peers: %v", err)
		}
	} else {
		log.Print("VPN_API_TOKEN not set: peer API disabled")
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           newHandler(cfg, svc, backend),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("%s %s listening on %s", cfg.Brand.Name, version, cfg.Listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
