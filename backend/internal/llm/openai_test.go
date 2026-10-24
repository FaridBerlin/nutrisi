package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeServer starts a local HTTP server that pretends to be Ollama.
// httptest.NewServer picks a free port; t.Cleanup stops it after the test.
func fakeServer(t *testing.T, handler http.HandlerFunc) *OpenAIClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewOpenAIClient(Options{
		BaseURL:         srv.URL,
		Model:           "test-model",
		ReasoningEffort: "none",
		Timeout:         5 * time.Second,
	})
}

func TestChatSendsRequestAndReturnsReply(t *testing.T) {
	var got chatRequest

	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// Extra fields like "reasoning" must not break decoding.
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"  Eat more fibre.  ","reasoning":"..."}}]}`))
	})

	reply, err := c.Chat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if reply != "Eat more fibre." {
		t.Errorf("reply = %q (should be trimmed)", reply)
	}
	if got.Model != "test-model" || got.Stream || got.ReasoningEffort != "none" {
		t.Errorf("request = %+v", got)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "hi" {
		t.Errorf("messages = %+v", got.Messages)
	}
}

func TestChatServerErrorIsUnavailable(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"message":"model \"test-model\" not found"}}`))
	})

	_, err := c.Chat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	t.Logf("error message: %v", err) // shows up with go test -v
}

func TestChatServerDown(t *testing.T) {
	// Port 1 on localhost: nothing listens there, so the connection fails.
	c := NewOpenAIClient(Options{BaseURL: "http://127.0.0.1:1", Model: "m", Timeout: time.Second})
	_, err := c.Chat(context.Background(), nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestChatEmptyReply(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":""}}]}`))
	})
	if _, err := c.Chat(context.Background(), nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestChatCancelledContext(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		// A "slow model": answers only after 2 seconds, far past our 50ms
		// deadline. (Waiting on r.Context() alone is not enough here: the
		// server only notices a client disconnect after the body was read.)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.Chat(ctx, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestListModels(t *testing.T) {
	c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list","data":[{"id":"qwen3.5:0.8b"},{"id":"gemma4:e4b"}]}`))
	})
	names, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "qwen3.5:0.8b" {
		t.Errorf("names = %v", names)
	}
}
