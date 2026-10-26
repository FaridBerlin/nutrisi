// Package server builds the Gin engine: middleware, static files and routes.
package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"

	"github.com/FaridBerlin/nutrisi/backend/internal/coach"
	"github.com/FaridBerlin/nutrisi/backend/internal/config"
	"github.com/FaridBerlin/nutrisi/frontend"
)

// handlers holds what the HTTP handlers need. Handlers are methods on it, so
// dependencies are passed in once instead of living in global variables.
type handlers struct {
	cfg     config.Config
	coach   *coach.Service
	pending *coach.Pending // questions waiting for their stream
}

// New returns a ready-to-serve Gin engine.
func New(cfg config.Config, coachSvc *coach.Service, log *slog.Logger) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	h := &handlers{
		cfg:   cfg,
		coach: coachSvc,
		// A question waits at most 2 minutes for the browser to open its
		// stream; at most 1000 can wait at once.
		pending: coach.NewPending(2*time.Minute, 1000),
	}

	// gin.New() starts without any middleware. We add ours in this order:
	r := gin.New()
	r.Use(
		requestID(log),                      // 1. ID + per-request logger
		requestLogger(),                     // 2. one log line per request (runs "around" the rest)
		recovery(h.serverError),             // 3. panic -> 500 page, not a crash
		securityHeaders(cfg.IsProduction()), // 4. CSP and friends on every response
		rateLimit( //                            5. general limit for everything...
			newLimiter(cfg.RateLimitGeneralPerMin, time.Minute, cfg.RateLimitGeneralPerMin/2),
			isStaticOrHealth, // ...except static files and the health check
			h.tooManyRequests,
		),
	)

	// We are not behind a reverse proxy yet, so trust no X-Forwarded-For
	// headers. (Revisit when deploying behind nginx/Caddy.)
	_ = r.SetTrustedProxies(nil)

	// Static files (CSS, JS, images) are embedded into the binary, so the
	// server is one file you can copy anywhere.
	static, err := fs.Sub(frontend.Static, "static")
	if err != nil {
		panic(err) // only fails if the embed path is wrong - a programming error
	}
	r.StaticFS("/static", http.FS(static))

	// The AI Coach is expensive (it runs a model), so it gets its own, much
	// stricter limit: RATE_LIMIT_COACH_PER_HOUR, with bursts of up to 10.
	coachLimit := rateLimit(
		newLimiter(cfg.RateLimitCoachPerHour, time.Hour, min(10, cfg.RateLimitCoachPerHour)),
		nil,
		h.tooManyRequests,
	)

	h.registerRoutes(r, coachLimit)
	return r
}

// registerRoutes is the one place to see every URL the app answers.
func (h *handlers) registerRoutes(r *gin.Engine, coachLimit gin.HandlerFunc) {
	r.GET("/healthz", h.health)

	// Full pages
	r.GET("/", h.home)
	r.GET("/coach", h.coachPage)

	// htmx endpoints: they return HTML fragments, not whole pages.
	// Middleware can also be added per route: coachLimit runs before coachChat.
	r.POST("/coach/chat", coachLimit, h.coachChat)
	r.GET("/coach/stream/:id", h.coachStream) // Server-Sent Events (one-time IDs)
	r.GET("/partials/server-time", h.serverTime)

	r.NoRoute(h.notFound)
}

func isStaticOrHealth(c *gin.Context) bool {
	return strings.HasPrefix(c.Request.URL.Path, "/static/") || c.Request.URL.Path == "/healthz"
}

// render writes a templ component as the HTML response.
func render(c *gin.Context, status int, component templ.Component) {
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(c.Request.Context(), c.Writer); err != nil {
		_ = c.Error(err) // recorded for logging; headers are already sent
	}
}
