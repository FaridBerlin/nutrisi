// Command server starts the NutriSI web app.
//
// Run it from the repository root:
//
//	go run ./backend/cmd/server
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

	"github.com/FaridBerlin/nutrisi/backend/internal/config"
	"github.com/FaridBerlin/nutrisi/backend/internal/server"
)

func main() {
	cfg := config.Load()

	// Gin's *gin.Engine is an http.Handler, so we hand it to a plain
	// http.Server. That gives us timeouts and graceful shutdown, which
	// engine.Run() alone does not offer.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(cfg),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout on purpose: the AI Coach will stream answers that can
		// take minutes. We will limit those per request instead (step 3).
	}

	// ctx is cancelled when you press Ctrl+C (SIGINT) or the OS asks the
	// process to stop (SIGTERM, e.g. from Docker).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ListenAndServe blocks, so it runs in its own goroutine.
	go func() {
		log.Printf("NutriSI listening on http://localhost:%s (env=%s)", cfg.Port, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done() // wait here until a shutdown signal arrives
	log.Println("shutting down...")

	// Give requests that are still running up to 10 seconds to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}
	log.Println("bye")
}
