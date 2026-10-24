# NutriSI

A nutrition and wellness app built to learn **Go**, **Gin**, **templ** and **htmx**.
It is a rebuild of NutriVa (React + Express) as a server-rendered Go app.

## Tech stack

- **Go 1.26** with **Gin** for routing and middleware
- **templ** for type-safe HTML templates (compiled to Go)
- **htmx 2** for interactivity without a JS framework
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
│       └── server/             Gin engine, routes, HTTP handlers
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
| POST   | `/coach/chat`           | htmx: question + answer bubbles |
| GET    | `/healthz`              | `{"status":"ok"}`               |
| GET    | `/partials/server-time` | htmx fragment (demo)            |
| GET    | `/static/*`             | Embedded static files           |

## Roadmap

1. ✅ Skeleton: Gin, templ, htmx, config, graceful shutdown, tests
2. ✅ AI Coach chat with a local Ollama model (complete answers, not streamed yet)
3. Streaming answers over SSE, cancelled when the browser closes
4. Middleware: rate limiting, structured logging (slog), security headers
5. Chat UI polish: Markdown rendering, history, error states
6. Docker, then PWA (manifest + service worker) → Google Play via a Trusted Web Activity
7. Later: accounts, profile, meal plans, trackers, dashboard, llama.cpp support

## How the AI Coach works

1. The form on `/coach` posts `message` plus the hidden `role`/`content` inputs
   from every earlier chat bubble (`hx-include="#messages"`). The page holds the
   conversation; the server stores nothing yet.
2. `coach.Service.Ask` validates the question, rebuilds the history (only
   user/assistant turns, last 20, max 4000 characters each), adds the system
   prompt, and calls the model.
3. `llm.OpenAIClient` sends `POST /v1/chat/completions` to Ollama.
   `LLM_REASONING_EFFORT=none` turns off "thinking" for speed.
4. On success the handler returns two bubbles that htmx appends. On failure it
   returns an error box plus `HX-Retarget`/`HX-Reswap` headers, and the
   `htmx-config` meta tag tells htmx to show 422 and 50x responses.

## Disclaimer

NutriSI gives general nutrition information. It is not medical advice.
