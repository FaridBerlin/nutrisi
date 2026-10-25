package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrUnavailable means the model server could not be reached or answered with
// an error. Handlers turn it into a friendly "coach is offline" message.
var ErrUnavailable = errors.New("llm: model server unavailable")

// OpenAIClient calls an OpenAI-compatible server such as Ollama or llama.cpp.
type OpenAIClient struct {
	baseURL         string // e.g. "http://localhost:11434" (without /v1)
	model           string // e.g. "qwen3.5:0.8b"
	apiKey          string // optional; local Ollama does not need one
	reasoningEffort string // "none" turns thinking off; "" = model default
	http            *http.Client
}

// Options configures NewOpenAIClient.
type Options struct {
	BaseURL         string
	Model           string
	APIKey          string
	ReasoningEffort string
	Timeout         time.Duration // whole request, including generation
}

// NewOpenAIClient builds a client. It does not contact the server yet.
func NewOpenAIClient(o Options) *OpenAIClient {
	return &OpenAIClient{
		baseURL:         strings.TrimRight(o.BaseURL, "/"),
		model:           o.Model,
		apiKey:          o.APIKey,
		reasoningEffort: o.ReasoningEffort,
		http:            &http.Client{Timeout: o.Timeout},
	}
}

// Model returns the configured model name.
func (c *OpenAIClient) Model() string { return c.model }

// --- request / response shapes -------------------------------------------
// Only the fields we use. encoding/json ignores everything else, so extra
// fields from Ollama or llama.cpp (like "reasoning") don't break anything.

type chatRequest struct {
	Model           string    `json:"model"`
	Messages        []Message `json:"messages"`
	Stream          bool      `json:"stream"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"` // omitted when ""
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

// streamChunk is one "data:" line of a streamed answer. The new text is in
// choices[0].delta.content. Thinking models also send delta.reasoning, which
// we simply don't decode.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// post sends a chat request and checks the status code. The caller must
// close the response body.
func (c *OpenAIClient) post(ctx context.Context, messages []Message, stream bool) (*http.Response, error) {
	body, err := json.Marshal(chatRequest{
		Model:           c.model,
		Messages:        messages,
		Stream:          stream,
		ReasoningEffort: c.reasoningEffort,
	})
	if err != nil {
		return nil, fmt.Errorf("llm: encode request: %w", err)
	}

	// NewRequestWithContext: if ctx is cancelled (e.g. the browser closed the
	// tab), the HTTP call to the model is cancelled too.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// %w wraps both errors, so callers can check errors.Is(err, ErrUnavailable)
		// and errors.Is(err, context.Canceled).
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		var e errorResponse
		_ = json.Unmarshal(raw, &e) // best effort; message may stay empty
		return nil, fmt.Errorf("%w: status %d: %s", ErrUnavailable, resp.StatusCode, e.Error.Message)
	}
	return resp, nil
}

// Chat implements Client: one request, one complete answer.
func (c *OpenAIClient) Chat(ctx context.Context, messages []Message) (string, error) {
	resp, err := c.post(ctx, messages, false)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read at most 1 MB - never trust a remote server to send a sane amount.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("%w: read body: %w", ErrUnavailable, err)
	}

	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("%w: decode response: %w", ErrUnavailable, err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("%w: response has no choices", ErrUnavailable)
	}

	reply := strings.TrimSpace(out.Choices[0].Message.Content)
	if reply == "" {
		return "", fmt.Errorf("%w: empty reply", ErrUnavailable)
	}
	return reply, nil
}

// ChatStream implements Client: the answer arrives in small pieces.
//
// With "stream": true the server answers with Server-Sent Events, one JSON
// chunk per line:
//
//	data: {"choices":[{"delta":{"content":"Lent"}}]}
//	data: {"choices":[{"delta":{"content":"ils are"}}]}
//	data: [DONE]
func (c *OpenAIClient) ChatStream(ctx context.Context, messages []Message, onDelta func(string) error) error {
	resp, err := c.post(ctx, messages, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20) // allow lines up to 1 MB

	gotText := false
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines (they separate events) and anything that isn't data.
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("%w: decode chunk: %w", ErrUnavailable, err)
		}
		if chunk.Error != nil {
			return fmt.Errorf("%w: %s", ErrUnavailable, chunk.Error.Message)
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue // e.g. a "reasoning" chunk or the role-only first chunk
		}

		gotText = true
		if err := onDelta(chunk.Choices[0].Delta.Content); err != nil {
			return err // the caller wants to stop (e.g. the browser left)
		}
	}

	if err := scanner.Err(); err != nil {
		// A cancelled context shows up here as a read error; return the
		// context's own error so callers can tell "cancelled" from "broken".
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: read stream: %w", ErrUnavailable, err)
	}
	if !gotText {
		return fmt.Errorf("%w: empty reply", ErrUnavailable)
	}
	return nil
}

// ListModels returns the model names the server offers (GET /v1/models).
// Used at startup to warn when the configured model is missing.
func (c *OpenAIClient) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: decode models: %w", ErrUnavailable, err)
	}
	names := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		names = append(names, m.ID)
	}
	return names, nil
}
