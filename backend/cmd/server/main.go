// Command server starts the NutriSI web app.
//
// Run it from the repository root:
//
//	go run ./backend/cmd/server
package main

import (
	"context"
	"errors"
	"log/slog"
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
	log := newLogger(cfg)
	slog.SetDefault(log) // packages that call slog.Info(...) use this logger too

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
		Handler:           server.New(cfg, coachSvc, log),
		ReadHeaderTimeout: 10 * time.Second,  // slow clients can't hold connections open
		ReadTimeout:       30 * time.Second,  // max time to read a whole request
		IdleTimeout:       120 * time.Second, // close unused keep-alive connections
		// No WriteTimeout on purpose: answers stream for up to LLM_TIMEOUT.
		// Errors from the HTTP server itself also go through slog.
		ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	// ctx is cancelled when you press Ctrl+C (SIGINT) or the OS asks the
	// process to stop (SIGTERM, e.g. from Docker).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ListenAndServe blocks, so it runs in its own goroutine.
	go func() {
		log.Info("NutriSI listening",
			"url", "http://localhost:"+cfg.Port,
			"env", cfg.AppEnv,
			"rate_limit_coach_per_hour", cfg.RateLimitCoachPerHour,
			"rate_limit_general_per_min", cfg.RateLimitGeneralPerMin,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Check the model in the background: it only logs, it never stops the app.
	go checkModel(ctx, log, llmClient, cfg)

	<-ctx.Done() // wait here until a shutdown signal arrives
	log.Info("shutting down")

	// Give requests that are still running up to 10 seconds to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("forced shutdown", "error", err)
	}
	log.Info("bye")
}

// newLogger builds the app's structured logger.
//
// Development: easy-to-read text lines
//
//	time=... level=INFO msg="NutriSI listening" url=http://localhost:8080
//
// Production: one JSON object per line, which log tools can search and filter
//
//	{"time":"...","level":"INFO","msg":"NutriSI listening","url":"..."}
func newLogger(cfg config.Config) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo // unknown LOG_LEVEL: fall back to info
	}
	opts := &slog.HandlerOptions{Level: level}

	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// checkModel logs whether the model server is reachable and has our model.
// Same idea as verifyOllamaModels() in NutriVa.
func checkModel(ctx context.Context, log *slog.Logger, c *llm.OpenAIClient, cfg config.Config) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	models, err := c.ListModels(ctx)
	switch {
	case err != nil:
		log.Warn("AI model server not reachable - start it with: ollama serve",
			"url", cfg.LLMBaseURL, "error", err)
	case !slices.Contains(models, cfg.LLMModel):
		log.Warn("AI model not found", "model", cfg.LLMModel, "url", cfg.LLMBaseURL, "available", models)
	default:
		log.Info("AI model ready", "model", cfg.LLMModel, "url", cfg.LLMBaseURL)
	}
}
