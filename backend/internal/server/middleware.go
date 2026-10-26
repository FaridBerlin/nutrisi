package server

import (
	"crypto/rand"
	"io"
	"log/slog"
	"math"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/FaridBerlin/nutrisi/backend/internal/ratelimit"
)

// Middleware is a function that runs around every request. In Gin it calls
// c.Next() to run the next middleware / the handler, and can do work before
// and after that call. Order matters: they run in the order they are added.

const loggerKey = "logger" // where requestID stores the per-request logger

// requestID gives every request a short random ID. It is sent back in the
// X-Request-ID header and added to every log line of that request, so you can
// find all lines that belong together.
func requestID(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := rand.Text()[:12]
		c.Header("X-Request-ID", id)
		// slog.With returns a logger that adds request_id to every line.
		c.Set(loggerKey, base.With("request_id", id))
		c.Next()
	}
}

// logger returns the per-request logger set by requestID.
// Falls back to the default logger (e.g. in small unit tests).
func logger(c *gin.Context) *slog.Logger {
	if l, ok := c.Get(loggerKey); ok {
		return l.(*slog.Logger) // type assertion: we know what we stored
	}
	return slog.Default()
}

// requestLogger writes one structured log line per request, after it is done.
// It never logs request bodies or query strings: they can contain users'
// questions, which are personal data (GDPR).
func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next() // run the handler

		status := c.Writer.Status()
		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		case isQuietPath(c.Request.URL.Path):
			level = slog.LevelDebug // static files and health checks are noise
		}

		logger(c).Log(c.Request.Context(), level, "request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", c.Writer.Size(),
			"ip", c.ClientIP(),
		)
	}
}

func isQuietPath(path string) bool {
	return strings.HasPrefix(path, "/static/") || path == "/healthz"
}

// recovery turns a panic in a handler into a 500 page plus an error log with
// the stack trace, instead of crashing the whole server.
func recovery(render500 func(*gin.Context)) gin.HandlerFunc {
	// io.Discard: Gin's own plain-text panic output is replaced by our slog line.
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, err any) {
		logger(c).Error("panic", "error", err, "stack", string(debug.Stack()))
		render500(c)
		c.Abort()
	})
}

// contentSecurityPolicy tells the browser which sources it may load. Only our
// own files are allowed: no inline scripts, no eval, no other websites. If an
// attacker ever managed to inject HTML, the browser would still refuse to run
// their script. (That's why htmx's eval is off and hx-on moved to app.js.)
var contentSecurityPolicy = strings.Join([]string{
	"default-src 'self'",
	"script-src 'self'",
	"style-src 'self'",
	"img-src 'self' data:",
	"connect-src 'self'", // fetch, htmx requests and EventSource (SSE)
	"font-src 'self'",
	"object-src 'none'",
	"base-uri 'self'",
	"form-action 'self'",
	"frame-ancestors 'none'", // nobody may show our pages in an iframe
}, "; ")

// securityHeaders adds response headers that make browsers stricter.
func securityHeaders(production bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")                  // trust our Content-Type
		h.Set("X-Frame-Options", "DENY")                            // old browsers: no iframes
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin") // don't leak full URLs
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if production {
			// Only over HTTPS: "use HTTPS for this site for the next year".
			// Never in development, or localhost would be stuck on HTTPS.
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// rateLimit refuses requests from clients that exceed the limiter.
// onLimited writes the response (HTML for pages, an error box for htmx).
// skip returns true for requests that should never be limited.
func rateLimit(l *ratelimit.Limiter, skip func(*gin.Context) bool, onLimited func(*gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if l == nil || (skip != nil && skip(c)) {
			c.Next()
			return
		}
		// ClientIP is the TCP address for now. Behind a reverse proxy we must
		// configure trusted proxies first, or everyone would share one IP.
		ok, retryAfter := l.Allow(c.ClientIP())
		if !ok {
			seconds := int(math.Ceil(retryAfter.Seconds()))
			c.Header("Retry-After", strconv.Itoa(seconds)) // standard header for 429
			logger(c).Warn("rate limited", "path", c.Request.URL.Path, "retry_after_s", seconds)
			onLimited(c)
			c.Abort() // stop here: the handler does not run
			return
		}
		c.Next()
	}
}

// newLimiter builds a limiter, or returns nil when the limit is 0 (off).
func newLimiter(count int, period time.Duration, burst int) *ratelimit.Limiter {
	if count <= 0 {
		return nil
	}
	return ratelimit.New(count, period, max(1, burst))
}

// isHTMX reports whether htmx sent the request (it adds this header).
func isHTMX(c *gin.Context) bool {
	return c.GetHeader("HX-Request") == "true"
}
