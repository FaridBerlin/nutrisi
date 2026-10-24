package server

import (
	"net/http"
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
