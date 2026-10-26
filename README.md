# NutriSI

A nutrition and wellness app built to learn **Go**, **Gin**, **templ** and **htmx**.
It is a rebuild of NutriVa (React + Express) as a server-rendered Go app.

## Tech stack

- **Go 1.26** with **Gin** for routing and middleware
- **templ** for type-safe HTML templates (compiled to Go)
- **htmx 2** for interactivity without a JS framework, plus its **SSE extension** for streaming
- Plain CSS (light and dark mode)
- AI: local **Ollama** first, **llama.cpp** later

## Run it

You need Go and a running [Ollama](https://ollama.com) (`ollama serve`) with at
least one model, e.g. `ollama pull qwen3.5:0.8b`.

```bash
cp .env.example .env          # then pick your model in .env (LLM_MODEL)
go mod tidy                   # first time only: downloads dependencies
go tool templ generate        # turns *.templ into Go code (after every .templ change)
go run ./backend/cmd/server   # open http://localhost:8080/coach
```

At startup the server checks the model and logs either `AI model ready: ...` or
a warning that explains what is missing.

**Live reload** (regenerates templates, restarts Go and refreshes the browser on save):

```bash
go tool templ generate --watch --proxy="http://localhost:8080" --cmd="go run ./backend/cmd/server"
# then open http://localhost:7331
```

You don't need to install templ separately: it is listed as a tool in `go.mod`,
and `go tool templ` runs it. If your Go version is older than 1.26, Go downloads
the right toolchain automatically the first time.

## Tests

```bash
go test ./...      # add -v to see each test
```

The tests never call a real model: they use a fake `llm.Client` and a fake
HTTP server, so they run in milliseconds and work offline.

## Project layout

```
nutrisi/
├── go.mod                      one Go module for the whole repo
├── backend/
│   ├── cmd/server/main.go      entry point: config, HTTP server, graceful shutdown
│   └── internal/
│       ├── config/             settings from environment variables / .env
│       ├── llm/                OpenAI-compatible client (Ollama, llama.cpp)
│       ├── coach/              AI Coach: input rules, history cleaning, system prompt
│       ├── ratelimit/          per-client token buckets (golang.org/x/time/rate)
│       └── server/             Gin engine, middleware, routes, HTTP handlers
└── frontend/
    ├── frontend.go             embeds ./static into the Go binary
    ├── views/                  templ components (*.templ → *_templ.go)
    └── static/                 CSS, htmx.min.js, images
```

`backend/internal/...` packages can only be imported from inside `backend/`. The
views in `frontend/` receive plain data and never import backend internals.

## Routes

| Method | Path                    | Returns                         |
| ------ | ----------------------- | ------------------------------- |
| GET    | `/`                     | Home page                       |
| GET    | `/coach`                | AI Coach chat page              |
| POST   | `/coach/chat`           | htmx: question bubble + streaming coach bubble |
| GET    | `/coach/stream/:id`     | Server-Sent Events: the answer, piece by piece |
| GET    | `/healthz`              | `{"status":"ok"}`               |
| GET    | `/partials/server-time` | htmx fragment (demo)            |
| GET    | `/static/*`             | Embedded static files           |

## Roadmap

1. ✅ Skeleton: Gin, templ, htmx, config, graceful shutdown, tests
2. ✅ AI Coach chat with a local Ollama model
3. ✅ Streaming answers over SSE, cancelled when the browser closes
4. ✅ Middleware: rate limiting, structured logging (slog), security headers
5. Chat UI polish: Markdown rendering, history, error states
6. Docker, then PWA (manifest + service worker) → Google Play via a Trusted Web Activity
7. Later: accounts, profile, meal plans, trackers, dashboard, llama.cpp support

## How the AI Coach works

One chat turn is two requests:

1. **`POST /coach/chat`** (htmx form). It sends `message` plus the hidden
   `role`/`content` inputs from every earlier bubble (`hx-include="#messages"`),
   so the page holds the conversation. `coach.Service.Prepare` validates the
   question and rebuilds the history (only user/assistant turns, last 20,
   max 4000 characters each). The conversation is stored in `coach.Pending`
   under a random, one-time ID (valid 2 minutes). The response is the user's
   bubble plus an empty coach bubble with `sse-connect="/coach/stream/{id}"`.
2. **`GET /coach/stream/{id}`** (opened by the htmx SSE extension). The handler
   takes the conversation out of the store and streams the model's answer:

   | Event    | Meaning                                       | Swapped into           |
   | -------- | --------------------------------------------- | ---------------------- |
   | `token`  | next piece of text (HTML-escaped)             | the bubble (`beforeend`) |
   | `final`  | hidden inputs with the whole answer (history) | placeholder (`outerHTML`) |
   | `failed` | error message                                 | the bubble             |
   | `done`   | always last; closes the connection            | –                      |

   The form stays locked while an answer streams. The extension fires
   `htmx:sseClose` at the end, and `hx-on:htmx:sse-close` unlocks the form.

`llm.OpenAIClient.ChatStream` sends `"stream": true` to `/v1/chat/completions`
and reads the `data:` lines. If the browser closes the tab, the request context
is cancelled and the model stops generating.

Errors in the POST (empty or too long question) come back as 422 with
`HX-Retarget: #chat-error`; the `htmx-config` meta tag tells htmx to show them.

## Security and logging

Every request passes through this middleware, in order (see `server.New`):

1. **Request ID**: a random ID in the `X-Request-ID` header and on every log line of that request.
2. **Request log**: one structured `slog` line per request (method, path,
   status, duration, IP). Text in development, JSON in production. Questions
   and answers are **never** logged, only their length (GDPR).
3. **Recovery**: a panic becomes a 500 page and an error log with the stack trace.
4. **Security headers**: a strict `Content-Security-Policy` (only our own
   scripts and styles: no inline code, no `eval`), `X-Content-Type-Options`,
   `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, and HSTS in production.
   htmx runs with `allowEval: false`, so page behaviour lives in
   `frontend/static/js/app.js` instead of `hx-on` attributes.
5. **Rate limits** per client IP (token buckets):
   - AI Coach (`POST /coach/chat`): `RATE_LIMIT_COACH_PER_HOUR` (default 30, bursts of 10)
   - everything else: `RATE_LIMIT_GENERAL_PER_MIN` (default 120); static files and `/healthz` are exempt

   A limited request gets `429` with a `Retry-After` header. In the chat, the
   message appears in the error box. Set a limit to `0` to turn it off.

Behind a reverse proxy (later), `SetTrustedProxies` must list the proxy;
otherwise all users would share the proxy's IP address and one rate limit.

## Disclaimer

NutriSI gives general nutrition information. It is not medical advice.
