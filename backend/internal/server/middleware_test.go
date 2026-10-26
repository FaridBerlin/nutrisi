package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/FaridBerlin/nutrisi/backend/internal/coach"
	"github.com/gin-gonic/gin"
)

func TestSecurityHeaders(t *testing.T) {
	w := get(t, newTestServer(&fakeLLM{}), "/")

	for header, want := range map[string]string{
		"Content-Security-Policy": "script-src 'self'",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
	} {
		if got := w.Header().Get(header); !strings.Contains(got, want) {
			t.Errorf("%s = %q, want it to contain %q", header, got, want)
		}
	}
	if w.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS must not be sent outside production")
	}
	if len(w.Header().Get("X-Request-ID")) != 12 {
		t.Errorf("X-Request-ID = %q", w.Header().Get("X-Request-ID"))
	}
}

func TestHSTSInProduction(t *testing.T) {
	cfg := testConfig()
	cfg.AppEnv = "production"
	w := get(t, newTestServerWith(cfg, &fakeLLM{}), "/healthz")
	if !strings.Contains(w.Header().Get("Strict-Transport-Security"), "max-age=") {
		t.Error("HSTS missing in production")
	}
}

func TestCoachRateLimit(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimitCoachPerHour = 2 // bucket of 2, then one token every 30 minutes
	r := newTestServerWith(cfg, &fakeLLM{reply: "ok"})

	for i := range 2 {
		if w := postForm(t, r, "/coach/chat", url.Values{"message": {"hi"}}); w.Code != http.StatusOK {
			t.Fatalf("question %d: status %d", i+1, w.Code)
		}
	}

	w := postForm(t, r, "/coach/chat", url.Values{"message": {"hi"}})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd question: status %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") != "1800" {
		t.Errorf("Retry-After = %q, want 1800 (30 min)", w.Header().Get("Retry-After"))
	}
	if w.Header().Get("HX-Retarget") != "#chat-error" {
		t.Error("htmx request should show the message in the chat's error box")
	}
	if !strings.Contains(w.Body.String(), "about 30 minutes") {
		t.Errorf("body = %s", w.Body.String())
	}

	// Other pages are not affected by the coach limit.
	if w := get(t, r, "/coach"); w.Code != http.StatusOK {
		t.Errorf("/coach status = %d", w.Code)
	}
}

func TestGeneralRateLimitSkipsStaticFiles(t *testing.T) {
	cfg := testConfig()
	cfg.RateLimitGeneralPerMin = 2 // bucket of 1
	r := newTestServerWith(cfg, &fakeLLM{})

	get(t, r, "/")
	if w := get(t, r, "/"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	} else if !strings.Contains(w.Body.String(), "Slow down") {
		t.Error("non-htmx request should get the full error page")
	}
	for range 5 {
		if w := get(t, r, "/static/js/htmx.min.js"); w.Code != http.StatusOK {
			t.Fatalf("static file was rate limited: %d", w.Code)
		}
	}
}

func TestPanicBecomes500(t *testing.T) {
	var logs bytes.Buffer
	gin.SetMode(gin.TestMode)
	r := New(testConfig(), coach.NewService(&fakeLLM{}), slog.New(slog.NewJSONHandler(&logs, nil)))
	r.GET("/boom", func(c *gin.Context) { panic("something broke") })

	w := get(t, r, "/boom")
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "Something went wrong") {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(logs.String(), `"msg":"panic"`) || !strings.Contains(logs.String(), "something broke") {
		t.Errorf("panic was not logged: %s", logs.String())
	}
}

// Privacy: the user's question must never appear in the logs.
func TestLogsNeverContainQuestions(t *testing.T) {
	var logs bytes.Buffer
	gin.SetMode(gin.TestMode)
	r := New(testConfig(), coach.NewService(&fakeLLM{reply: "fine"}), slog.New(slog.NewJSONHandler(&logs, nil)))

	secret := "I-have-diabetes-and-weigh-123kg"
	get(t, r, ask(t, r, url.Values{"message": {secret}}))

	if strings.Contains(logs.String(), secret) {
		t.Errorf("question leaked into the logs:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), `"question_chars":31`) {
		t.Errorf("expected a size-only log line:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), `"request_id"`) {
		t.Error("log lines should carry the request ID")
	}
}
