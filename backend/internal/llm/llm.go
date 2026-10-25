// Package llm talks to a language model server.
//
// It speaks the OpenAI-compatible chat API (POST /v1/chat/completions), which
// both Ollama and llama.cpp's llama-server offer. Switching between them is
// only a different base URL and model name in .env - no code change.
package llm

import "context"

// Roles used in a conversation.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is one turn in a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Client is what the rest of the app needs from a model. Code depends on this
// interface, not on the HTTP implementation, so tests can pass a fake.
type Client interface {
	// Chat sends the whole conversation and returns the model's full reply.
	Chat(ctx context.Context, messages []Message) (string, error)

	// ChatStream sends the conversation and calls onDelta with each piece of
	// text as soon as the model produces it. If onDelta returns an error,
	// streaming stops and ChatStream returns that error.
	ChatStream(ctx context.Context, messages []Message, onDelta func(text string) error) error
}
