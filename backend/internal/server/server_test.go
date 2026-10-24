package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/FaridBerlin/nutrisi/backend/internal/config"
)

// newTestServer builds the real engine in test mode (quieter logs).
func newTestServer() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return New(config.Config{Port: "0", AppEnv: "test"})
}

// get sends a GET request straight to the engine - no network needed.
func get(t *testing.T, r http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestHealth(t *testing.T) {
	w := get(t, newTestServer(), "/healthz")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"status":"ok"}` {
		t.Errorf("body = %s", got)
	}
}

// Table-driven test: one loop, many cases. Very common style in Go.
func TestPages(t *testing.T) {
	r := newTestServer()

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantText   string
	}{
		{"home page", "/", http.StatusOK, "AI Coach"},
		{"home loads htmx", "/", http.StatusOK, "/static/js/htmx.min.js"},
		{"server time partial", "/partials/server-time", http.StatusOK, "Server time"},
		{"static htmx file", "/static/js/htmx.min.js", http.StatusOK, "htmx"},
		{"unknown page", "/does-not-exist", http.StatusNotFound, "Page not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := get(t, r, tt.path)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if !strings.Contains(w.Body.String(), tt.wantText) {
				t.Errorf("body does not contain %q", tt.wantText)
			}
		})
	}
}

// The partial must be a fragment, not a whole page - otherwise htmx would
// nest a second <html> inside the first.
func TestServerTimeIsFragment(t *testing.T) {
	w := get(t, newTestServer(), "/partials/server-time")
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("partial should not contain a full <html> document")
	}
}
