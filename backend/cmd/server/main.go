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
	"slices"
	"syscall"
	"time"

	"github.com/FaridBerlin/nutrisi/backend/internal/coach"
	"github.com/FaridBerlin/nutrisi/backend/internal/config"
	"github.com/FaridBerlin/nutrisi/backend/internal/llm"
	"github.com/FaridBerlin/nutrisi/backend/internal/server"
)

func main() {
	cfg := config.Load()

	// Build the pieces from the inside out: model client -> coach -> HTTP.
	llmClient := llm.NewOpenAIClient(llm.Options{
		BaseURL:         cfg.LLMBaseURL,
		Model:           cfg.LLMModel,
		APIKey:          cfg.LLMAPIKey,
		ReasoningEffort: cfg.LLMReasoningEffort,
		Timeout:         cfg.LLMTimeout,
	})
	coachSvc := coach.NewService(llmClient)

	// Gin's *gin.Engine is an http.Handler, so we hand it to a plain
	// http.Server. That gives us timeouts and graceful shutdown, which
	// engine.Run() alone does not offer.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(cfg, coachSvc),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout on purpose: model answers can take minutes. Each
		// coach request has its own limit (LLM_TIMEOUT) instead.
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

	// Check the model in the background: it only logs, it never stops the app.
	go checkModel(ctx, llmClient, cfg)

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

// checkModel logs whether the model server is reachable and has our model.
// Same idea as verifyOllamaModels() in NutriVa.
func checkModel(ctx context.Context, c *llm.OpenAIClient, cfg config.Config) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	models, err := c.ListModels(ctx)
	switch {
	case err != nil:
		log.Printf("WARNING: AI model server not reachable at %s (%v). Start it with: ollama serve", cfg.LLMBaseURL, err)
	case !slices.Contains(models, cfg.LLMModel):
		log.Printf("WARNING: model %q not found at %s. Available: %v", cfg.LLMModel, cfg.LLMBaseURL, models)
	default:
		log.Printf("AI model ready: %s at %s", cfg.LLMModel, cfg.LLMBaseURL)
	}
}
