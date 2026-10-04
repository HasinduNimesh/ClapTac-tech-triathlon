// Command agent-manager collects agent traces and shows what each agent did and why.
//
// It is infrastructure, not a business service: it has no business data and no
// database, and agents only push traces to it (see docs/architecture/agent-plane.md).
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func main() {
	environment := env("ENVIRONMENT", "local")
	srv := Server{IngestToken: os.Getenv("AGENT_TRACE_INGEST_TOKEN"), ViewToken: os.Getenv("AGENT_TRACE_VIEW_TOKEN")}
	// Local development may run open (like Grafana's admin/admin); anywhere else both
	// sides need a token because traces describe what staff asked agents to do.
	if environment != "local" && (srv.IngestToken == "" || srv.ViewToken == "") {
		log.Fatal("AGENT_TRACE_INGEST_TOKEN and AGENT_TRACE_VIEW_TOKEN are required when ENVIRONMENT is not local")
	}
	max, _ := strconv.Atoi(env("AGENT_TRACE_MAX", "1000"))
	store, err := NewStore(max, os.Getenv("AGENT_TRACE_FILE"))
	if err != nil {
		log.Fatalf("open trace store: %v", err)
	}
	srv.Store = store

	httpServer := &http.Server{Addr: env("HTTP_ADDR", ":8085"), Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()
	log.Printf("agent-manager listening on %s (max %d traces)", httpServer.Addr, store.max)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
