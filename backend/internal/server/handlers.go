package server

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/FaridBerlin/nutrisi/frontend/views"
)

// health tells monitoring tools (and you) that the server is up.
// It returns JSON, not HTML, because machines read it.
func (h *handlers) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// home renders the full home page.
func (h *handlers) home(c *gin.Context) {
	render(c, http.StatusOK, views.HomePage())
}

// serverTime returns only a small fragment. The button on the home page asks
// for it with hx-get, and htmx swaps it into the page - no reload.
func (h *handlers) serverTime(c *gin.Context) {
	now := time.Now().Format("15:04:05")
	render(c, http.StatusOK, views.ServerTime(now))
}

// notFound renders a friendly 404 page for unknown URLs.
func (h *handlers) notFound(c *gin.Context) {
	render(c, http.StatusNotFound, views.NotFoundPage())
}

// tooManyRequests answers a rate-limited request with status 429.
// For htmx requests it fills the chat's error box; otherwise a full page.
func (h *handlers) tooManyRequests(c *gin.Context) {
	msg := "You're sending requests very quickly. Please wait " +
		waitText(c.Writer.Header().Get("Retry-After")) + " and try again."
	if isHTMX(c) {
		c.Header("HX-Retarget", "#chat-error")
		c.Header("HX-Reswap", "innerHTML")
		render(c, http.StatusTooManyRequests, views.ChatError(msg))
		return
	}
	render(c, http.StatusTooManyRequests, views.ErrorPage("Slow down a little", msg))
}

// serverError renders the 500 page (used after a panic).
func (h *handlers) serverError(c *gin.Context) {
	render(c, http.StatusInternalServerError,
		views.ErrorPage("Something went wrong", "Sorry, that didn't work. Please try again in a moment."))
}

// waitText turns a Retry-After value in seconds into friendly words.
func waitText(retryAfter string) string {
	secs, err := strconv.Atoi(retryAfter)
	switch {
	case err != nil || secs <= 0:
		return "a moment"
	case secs < 60:
		return "a few seconds"
	case secs < 120:
		return "about a minute"
	default:
		return fmt.Sprintf("about %d minutes", (secs+59)/60) // round up
	}
}
