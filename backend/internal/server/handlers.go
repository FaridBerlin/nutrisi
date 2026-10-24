package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/FaridBerlin/nutrisi/frontend/views"
)

// handleHealth tells monitoring tools (and you) that the server is up.
// It returns JSON, not HTML, because machines read it.
func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleHome renders the full home page.
func handleHome(c *gin.Context) {
	render(c, http.StatusOK, views.HomePage())
}

// handleServerTime returns only a small fragment. The button on the home page
// asks for it with hx-get, and htmx swaps it into the page - no reload.
func handleServerTime(c *gin.Context) {
	now := time.Now().Format("15:04:05")
	render(c, http.StatusOK, views.ServerTime(now))
}

// handleNotFound renders a friendly 404 page for unknown URLs.
func handleNotFound(c *gin.Context) {
	render(c, http.StatusNotFound, views.NotFoundPage())
}
