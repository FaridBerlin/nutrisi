package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/FaridBerlin/nutrisi/backend/internal/coach"
	"github.com/FaridBerlin/nutrisi/backend/internal/config"
	"github.com/FaridBerlin/nutrisi/backend/internal/llm"
)

// fakeLLM stands in for Ollama. It remembers the last conversation it got.
type fakeLLM struct {
	got   []llm.Message
	reply string
	err   error
}

func (f *fakeLLM) Chat(_ context.Context, msgs []llm.Message) (string, error) {
	f.got = msgs
	return f.reply, f.err
}

// ChatStream sends the reply word by word, like a real model would.
func (f *fakeLLM) ChatStream(_ context.Context, msgs []llm.Message, onDelta func(string) error) error {
	f.got = msgs
	if f.err != nil {
		return f.err
	}
	for _, word := range strings.SplitAfter(f.reply, " ") {
		if err := onDelta(word); err != nil {
			return err
		}
	}
	return nil
}

// newTestServer builds the real engine in test mode with a fake model.
func newTestServer(fake *fakeLLM) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := config.Config{Port: "0", AppEnv: "test", LLMModel: "fake-model", LLMTimeout: time.Second}
	return New(cfg, coach.NewService(fake))
}

// get sends a GET request straight to the engine - no network needed.
func get(t *testing.T, r http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// postForm sends a form like the browser (and htmx) does.
func postForm(t *testing.T, r http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHealth(t *testing.T) {
	w := get(t, newTestServer(&fakeLLM{}), "/healthz")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"status":"ok"}` {
		t.Errorf("body = %s", got)
	}
}

// Table-driven test: one loop, many cases. Very common style in Go.
func TestPages(t *testing.T) {
	r := newTestServer(&fakeLLM{})

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantText   string
	}{
		{"home page", "/", http.StatusOK, "AI Coach"},
		{"home loads htmx", "/", http.StatusOK, "/static/js/htmx.min.js"},
		{"coach page", "/coach", http.StatusOK, `hx-post="/coach/chat"`},
		{"coach page shows model", "/coach", http.StatusOK, "fake-model"},
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
	w := get(t, newTestServer(&fakeLLM{}), "/partials/server-time")
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("partial should not contain a full <html> document")
	}
}
