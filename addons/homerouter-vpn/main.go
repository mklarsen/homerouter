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

// newHandler serves the peer API only when svc is set, and the sign-in and
// self-service routes only when auth is set.
func newHandler(cfg Config, svc *Service, backend Backend, auth *Authenticator) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
	})
	mux.HandleFunc("GET /api/v1/brand", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, cfg.Brand)
	})
	if svc != nil && cfg.APIToken != "" {
		(&api{cfg: cfg, svc: svc, backend: backend}).register(mux)
	}
	if auth != nil {
		auth.register(mux)
	}
	registerUI(mux)

	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		// Only same-origin scripts and styles; logos may come from https and QR codes from data: URLs.
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; "+
			"img-src 'self' data: https:; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
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
	var auth *Authenticator
	if cfg.APIToken != "" || cfg.OIDC != nil {
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
	}
	if cfg.APIToken == "" {
		log.Print("VPN_API_TOKEN not set: admin peer API disabled")
	}
	if cfg.OIDC != nil {
		owners, err := NewOwnerStore(cfg.DataDir)
		if err != nil {
			log.Fatalf("owners: %v", err)
		}
		auth, err = NewAuthenticator(ctx, *cfg.OIDC, &api{cfg: cfg, svc: svc, backend: backend}, owners)
		if err != nil {
			log.Fatalf("oidc: %v", err)
		}
		go auth.RunReconciler(ctx)
	} else {
		log.Print("VPN_OIDC_ISSUER not set: SSO sign-in disabled")
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           newHandler(cfg, svc, backend, auth),
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
