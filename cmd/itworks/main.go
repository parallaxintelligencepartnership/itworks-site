// Command itworks serves the itworks.dev wall, badge, and admin service.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"itworks.dev/internal/server"
	"itworks.dev/internal/store"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseAdminUsers splits a comma separated list of Authentik usernames,
// trimming whitespace and dropping empty entries. An empty or unset
// ITWORKS_ADMIN_USERS yields an empty slice, which denies every admin
// request.
func parseAdminUsers(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func main() {
	addr := getenv("ITWORKS_ADDR", ":8080")
	dbPath := getenv("ITWORKS_DB", "/data/itworks.db")
	baseURL := getenv("ITWORKS_BASE_URL", "https://itworks.dev")
	trustProxy := getenv("ITWORKS_TRUST_PROXY", "0") == "1"
	adminUsers := parseAdminUsers(getenv("ITWORKS_ADMIN_USERS", ""))

	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	srv, err := server.New(db, server.Config{
		BaseURL:    baseURL,
		TrustProxy: trustProxy,
		AdminUsers: adminUsers,
	})
	if err != nil {
		log.Fatalf("build server: %v", err)
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("itworks.dev listening on %s (db=%s base_url=%s trust_proxy=%v)", addr, dbPath, baseURL, trustProxy)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}
