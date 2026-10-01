package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	apiToken := os.Getenv("PROXY_API_TOKEN")
	if err := validateAPIToken(apiToken); err != nil {
		log.Fatal(err)
	}

	dataFile := envOr("PROXY_DATA_FILE", "/data/users.json")
	adminUsername := envOr("PROXY_ADMIN_USERNAME", "admin")
	users, err := openUserStore(dataFile, adminUsername, os.Getenv("PROXY_ADMIN_PASSWORD"))
	if err != nil {
		log.Fatalf("initialize proxy user store: %v", err)
	}
	if !setActiveProxyLogLevel(users.logLevel()) {
		log.Fatalf("invalid persisted proxy log level: %s", users.logLevel())
	}

	address := envOr("PROXY_LISTEN_ADDR", ":8080")
	service := &server{
		users:        users,
		sessions:     newSessionStore(),
		activity:     newProxyActivity(),
		apiToken:     apiToken,
		dataHostPath: envOr("PROXY_DATA_HOST_PATH", "Docker-managed volume"),
	}
	httpServer := &http.Server{
		Addr:              address,
		Handler:           service,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}

	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- httpServer.ListenAndServe() }()
	log.Printf("Homerouter proxy listening on %s", address)

	select {
	case <-shutdownContext.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("proxy server stopped: %v", err)
		}
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
