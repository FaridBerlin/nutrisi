package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"
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

// coachChat is step 1 of a chat turn. htmx posts the form here:
//
//	message   = the new question
//	role[]    = roles of earlier turns    (hidden inputs in each chat bubble)
//	content[] = texts of earlier turns
//
// It only checks the question and stores it - it does not call the model, so
// it answers in milliseconds. The response is the user's bubble plus an empty
// coach bubble that opens an SSE connection to /coach/stream/{id} (step 2).
func (h *handlers) coachChat(c *gin.Context) {
	// Stop reading after maxChatBody bytes, so nobody can send us 1 GB.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChatBody)

	history := historyFromForm(c.PostFormArray("role"), c.PostFormArray("content"))
	conv, err := h.coach.Prepare(c.PostForm("message"), history)
	if err != nil {
		h.chatError(c, err)
		return
	}

	id, err := h.pending.Put(conv)
	if err != nil {
		h.chatError(c, err)
		return
	}
	render(c, http.StatusOK, views.ChatTurnStreaming(conv.Question, id))
}

// coachStream is step 2: it streams the answer as Server-Sent Events.
//
// Events the page listens for (see ChatTurnStreaming in coach.templ):
//
//	token  - a piece of the answer, appended to the bubble
//	final  - hidden inputs with the whole answer, so it becomes history
//	failed - an error message shown inside the bubble
//	done   - always last; tells htmx to close the connection
//
// "done" matters: without it the browser's EventSource would reconnect
// forever, because reconnecting is what EventSource does when a stream ends.
func (h *handlers) coachStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no") // tell nginx (later) not to buffer the stream
	c.Status(http.StatusOK)

	conv, ok := h.pending.Take(c.Param("id"))
	if !ok {
		// Unknown, used or expired ID - for example after a reconnect.
		h.streamFailed(c, "This answer is no longer available. Please ask your question again.")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.cfg.LLMTimeout)
	defer cancel()

	answer, err := h.coach.Stream(ctx, conv, func(text string) error {
		// html.EscapeString turns "<" into "&lt;" etc., because htmx inserts
		// the data as HTML. Model output must never become real HTML tags.
		return sendEvent(c, "token", html.EscapeString(text))
	})

	if err != nil {
		if c.Request.Context().Err() != nil {
			// The browser left (tab closed, page reloaded). The model call
			// was cancelled through ctx; there is nobody to write to.
			logger(c).Info("coach: client disconnected during stream")
			return
		}
		h.streamFailed(c, h.userMessage(c, err))
		return
	}

	// The answer is complete: add it to the page's history, then close.
	// Log sizes only - never the question or answer text (personal data).
	logger(c).Info("coach: answer streamed",
		"question_chars", len([]rune(conv.Question)),
		"answer_chars", len([]rune(answer)),
		"history_turns", len(conv.Messages)-2,
	)
	_ = sendEvent(c, "final", renderString(ctx, views.HistoryInputs(llm.RoleAssistant, answer)))
	_ = sendEvent(c, "done", "")
}

// streamFailed sends an error message into the bubble and closes the stream.
func (h *handlers) streamFailed(c *gin.Context, msg string) {
	_ = sendEvent(c, "failed", renderString(c.Request.Context(), views.ChatError(msg)))
	_ = sendEvent(c, "done", "")
}

// sendEvent writes one Server-Sent Event and flushes it to the browser:
//
//	event: token
//	data: first line
//	data: second line
//	(empty line = end of event)
//
// Data with line breaks needs one "data:" line per line; the browser joins
// them again with "\n".
func sendEvent(c *gin.Context, event, data string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "event: %s\n", event)
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(&b, "data: %s\n", line)
	}
	b.WriteString("\n")

	if _, err := c.Writer.WriteString(b.String()); err != nil {
		return err
	}
	c.Writer.Flush() // send now, don't wait for a buffer to fill up
	return nil
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

// chatError answers the POST with an error box. htmx puts it into
// #chat-error because of the HX-Retarget and HX-Reswap headers.
func (h *handlers) chatError(c *gin.Context, err error) {
	status := http.StatusUnprocessableEntity // invalid input
	if errors.Is(err, coach.ErrTooManyPending) {
		status = http.StatusServiceUnavailable
	}
	c.Header("HX-Retarget", "#chat-error") // put the response into the error box...
	c.Header("HX-Reswap", "innerHTML")     // ...replacing what is inside it
	render(c, status, views.ChatError(h.userMessage(c, err)))
}

// userMessage turns an error into text for the user. The real error is only
// logged, never shown: it may contain internal details.
func (h *handlers) userMessage(c *gin.Context, err error) string {
	log := logger(c)
	switch {
	case errors.Is(err, coach.ErrEmptyMessage):
		return "Please type a question first."
	case errors.Is(err, coach.ErrMessageTooLong):
		return fmt.Sprintf("Your message is too long. Please keep it under %d characters.", coach.MaxMessageLength)
	case errors.Is(err, coach.ErrTooManyPending):
		log.Warn("coach: pending store full", "error", err)
		return "The coach is very busy right now. Please try again in a minute."
	case errors.Is(err, context.DeadlineExceeded):
		log.Warn("coach: model timeout", "timeout", h.cfg.LLMTimeout, "error", err)
		return "The coach took too long to answer. Please try a shorter question."
	default: // llm.ErrUnavailable and anything unexpected
		log.Error("coach: model error", "error", err)
		msg := "The coach can't reach its AI model right now. Please try again in a moment."
		if !h.cfg.IsProduction() {
			msg += fmt.Sprintf(" (Dev hint: is Ollama running at %s with model %q?)", h.cfg.LLMBaseURL, h.cfg.LLMModel)
		}
		return msg
	}
}

// renderString renders a templ component to a string (for SSE data).
func renderString(ctx context.Context, component templ.Component) string {
	var buf bytes.Buffer
	if err := component.Render(ctx, &buf); err != nil {
		slog.Error("render failed", "error", err)
	}
	return buf.String()
}
