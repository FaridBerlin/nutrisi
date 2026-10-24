// Package coach is the AI Coach feature: input rules, the system prompt and
// the conversation logic. It knows nothing about HTTP or HTML, so the same code
// can later serve a JSON API for a mobile app.
package coach

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/FaridBerlin/nutrisi/backend/internal/llm"
)

// Limits ported from NutriVa's aiDoctorController.js.
const (
	MaxMessageLength        = 2000 // characters in one user message
	MaxHistoryMessages      = 20   // earlier turns sent back to the model
	MaxHistoryContentLength = 4000 // characters kept per earlier turn
)

// Validation errors. Go convention: error strings are lowercase and are not
// shown to users directly. The HTTP handler maps them to friendly text.
var (
	ErrEmptyMessage   = errors.New("coach: message is empty")
	ErrMessageTooLong = fmt.Errorf("coach: message longer than %d characters", MaxMessageLength)
)

// Service answers nutrition questions with the help of a model.
type Service struct {
	llm llm.Client
}

// NewService wires the coach to a model client.
func NewService(client llm.Client) *Service {
	return &Service{llm: client}
}

// Ask validates the question, cleans the history, and asks the model.
// history holds earlier turns, oldest first, as they came from the browser.
func (s *Service) Ask(ctx context.Context, question string, history []llm.Message) (string, error) {
	question, err := ValidateMessage(question)
	if err != nil {
		return "", err
	}

	// system prompt + earlier turns + the new question
	clean := SanitizeHistory(history)
	messages := make([]llm.Message, 0, len(clean)+2)
	messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: SystemPrompt})
	messages = append(messages, clean...)
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: question})

	return s.llm.Chat(ctx, messages)
}

// ValidateMessage trims the message and checks its length.
// It counts characters (runes), not bytes, so "Müsli" is 5, not 6.
func ValidateMessage(msg string) (string, error) {
	msg = strings.TrimSpace(msg)
	switch {
	case msg == "":
		return "", ErrEmptyMessage
	case utf8.RuneCountInString(msg) > MaxMessageLength:
		return "", ErrMessageTooLong
	}
	return msg, nil
}

// SanitizeHistory rebuilds the history instead of trusting it.
//
// The history comes from the browser, so anyone can edit it. Without this a
// caller could add their own "system" message to override the coach's rules,
// or send a huge history to drive up the cost. We keep only user/assistant
// turns, drop empty ones, keep the last MaxHistoryMessages, and cut long ones.
func SanitizeHistory(history []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(history))
	for _, m := range history {
		if m.Role != llm.RoleUser && m.Role != llm.RoleAssistant {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		out = append(out, llm.Message{Role: m.Role, Content: truncateRunes(content, MaxHistoryContentLength)})
	}
	if len(out) > MaxHistoryMessages {
		out = out[len(out)-MaxHistoryMessages:]
	}
	return out
}

// truncateRunes cuts s to at most n characters without splitting a multi-byte
// character (slicing a string by bytes could cut "ü" in half).
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
