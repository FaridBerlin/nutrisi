// Package server builds the Gin engine: middleware, static files and routes.
package server

import (
	"io/fs"
	"net/http"
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
func New(cfg config.Config, coachSvc *coach.Service) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	// gin.New() starts empty; gin.Default() would add Logger and Recovery for
	// us. Adding them by hand makes it clear what runs on every request.
	r := gin.New()
	r.Use(gin.Logger())   // one log line per request
	r.Use(gin.Recovery()) // a panic in a handler becomes a 500, not a crash

	// We are not behind a reverse proxy yet, so trust no X-Forwarded-For
	// headers. (Revisit when deploying behind nginx/Caddy.)
	_ = r.SetTrustedProxies(nil)

	// Static files (CSS, htmx.min.js, images) are embedded into the binary,
	// so the server is one file you can copy anywhere.
	static, err := fs.Sub(frontend.Static, "static")
	if err != nil {
		panic(err) // only fails if the embed path is wrong - a programming error
	}
	r.StaticFS("/static", http.FS(static))

	h := &handlers{
		cfg:   cfg,
		coach: coachSvc,
		// A question waits at most 2 minutes for the browser to open its
		// stream; at most 1000 can wait at once.
		pending: coach.NewPending(2*time.Minute, 1000),
	}
	h.registerRoutes(r)
	return r
}

// registerRoutes is the one place to see every URL the app answers.
func (h *handlers) registerRoutes(r *gin.Engine) {
	r.GET("/healthz", h.health)

	// Full pages
	r.GET("/", h.home)
	r.GET("/coach", h.coachPage)

	// htmx endpoints: they return HTML fragments, not whole pages.
	r.POST("/coach/chat", h.coachChat)
	r.GET("/coach/stream/:id", h.coachStream) // Server-Sent Events
	r.GET("/partials/server-time", h.serverTime)

	r.NoRoute(h.notFound)
}

// render writes a templ component as the HTML response.
func render(c *gin.Context, status int, component templ.Component) {
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(c.Request.Context(), c.Writer); err != nil {
		_ = c.Error(err) // recorded for logging; headers are already sent
	}
}
