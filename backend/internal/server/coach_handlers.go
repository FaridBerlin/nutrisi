package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/FaridBerlin/nutrisi/backend/internal/coach"
	"github.com/FaridBerlin/nutrisi/backend/internal/llm"
	"github.com/FaridBerlin/nutrisi/frontend/views"
)

// maxChatBody limits the size of one chat request (question + history).
const maxChatBody = 512 << 10 // 512 KB

// coachPage renders the chat page.
func (h *handlers) coachPage(c *gin.Context) {
	render(c, http.StatusOK, views.CoachPage(h.cfg.LLMModel))
}

// coachChat answers one question. htmx posts the form here:
//
//	message  = the new question
//	role[]   = roles of earlier turns    (hidden inputs in each chat bubble)
//	content[] = texts of earlier turns
//
// On success it returns two chat bubbles (question + answer) that htmx appends
// to the conversation. On failure it returns an error box and tells htmx to
// put it somewhere else, using the HX-Retarget and HX-Reswap headers.
func (h *handlers) coachChat(c *gin.Context) {
	// Stop reading after maxChatBody bytes, so nobody can send us 1 GB.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChatBody)

	question := c.PostForm("message")
	history := historyFromForm(c.PostFormArray("role"), c.PostFormArray("content"))

	// The answer must arrive within LLMTimeout. The context is also cancelled
	// when the browser disconnects, which stops the model call early.
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.cfg.LLMTimeout)
	defer cancel()

	answer, err := h.coach.Ask(ctx, question, history)
	if err != nil {
		h.chatError(c, err)
		return
	}

	// ValidateMessage already ran inside Ask; call it again only to get the
	// trimmed text to show in the user's bubble.
	shown, _ := coach.ValidateMessage(question)
	render(c, http.StatusOK, views.ChatTurn(shown, answer))
}

// historyFromForm zips the two parallel lists back into messages.
// If the lists have different lengths, the extra items are ignored.
func historyFromForm(roles, contents []string) []llm.Message {
	n := min(len(roles), len(contents)) // min is built in since Go 1.21
	msgs := make([]llm.Message, n)
	for i := range n { // range over an int: Go 1.22+
		msgs[i] = llm.Message{Role: roles[i], Content: contents[i]}
	}
	return msgs
}

// chatError maps an error to a status code and a message for the user.
// The real error is only logged, never shown: it may contain internal details.
func (h *handlers) chatError(c *gin.Context, err error) {
	var (
		status int
		msg    string
	)
	switch {
	case errors.Is(err, coach.ErrEmptyMessage):
		status, msg = http.StatusUnprocessableEntity, "Please type a question first."
	case errors.Is(err, coach.ErrMessageTooLong):
		status, msg = http.StatusUnprocessableEntity,
			fmt.Sprintf("Your message is too long. Please keep it under %d characters.", coach.MaxMessageLength)
	case errors.Is(err, context.Canceled):
		// The browser went away; nobody is waiting for an answer.
		log.Printf("coach: client disconnected")
		c.Status(499) // nginx's "client closed request" convention
		return
	case errors.Is(err, context.DeadlineExceeded):
		log.Printf("coach: timeout after %s: %v", h.cfg.LLMTimeout, err)
		status, msg = http.StatusGatewayTimeout,
			"The coach took too long to answer. Please try a shorter question."
	default: // llm.ErrUnavailable and anything unexpected
		log.Printf("coach: model error: %v", err)
		status, msg = http.StatusBadGateway,
			"The coach can't reach its AI model right now. Please try again in a moment."
		if !h.cfg.IsProduction() {
			msg += fmt.Sprintf(" (Dev hint: is Ollama running at %s with model %q?)", h.cfg.LLMBaseURL, h.cfg.LLMModel)
		}
	}

	// Response headers that htmx understands:
	c.Header("HX-Retarget", "#chat-error") // put the response into the error box...
	c.Header("HX-Reswap", "innerHTML")     // ...replacing what is inside it
	render(c, status, views.ChatError(msg))
}
