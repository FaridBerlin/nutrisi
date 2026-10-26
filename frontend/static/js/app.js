// NutriSI page behaviour.
//
// This lives in a file instead of inline hx-on="..." attributes because our
// Content-Security-Policy only allows scripts from our own files. Each handler
// listens on `document`, so it also works for elements htmx adds later.

// Enter sends the chat message, Shift+Enter makes a new line.
// Any <textarea data-submit-on-enter> gets this behaviour.
document.addEventListener("keydown", (event) => {
  const el = event.target;
  if (event.key === "Enter" && !event.shiftKey && el.matches?.("textarea[data-submit-on-enter]")) {
    event.preventDefault();
    el.form.requestSubmit();
  }
});

// After the chat form was posted successfully: clear it and keep it locked
// while the coach's answer streams in.
document.addEventListener("htmx:afterRequest", (event) => {
  const form = event.detail.elt;
  if (form.id === "chat-form" && event.detail.xhr.status === 200) {
    form.reset();
    document.getElementById("ask").disabled = true;
  }
});

// The SSE extension fires htmx:sseClose when an answer stream ends (also after
// an error). Then the user may ask the next question.
document.addEventListener("htmx:sseClose", () => {
  const ask = document.getElementById("ask");
  if (ask) {
    ask.disabled = false;
    document.getElementById("message").focus();
  }
});
