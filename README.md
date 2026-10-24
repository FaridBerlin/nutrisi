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

```bash
# 1. Download dependencies (first time only)
go mod tidy

# 2. Generate Go code from the .templ files and start the server
make run           # http://localhost:8080

# or, with live reload on every save:
make dev           # http://localhost:7331
```

No `make`? Use the commands inside the `Makefile` directly, for example
`go tool templ generate && go run ./backend/cmd/server`.

You don't need to install templ separately: it is listed as a tool in `go.mod`,
and `go tool templ` runs it. If your Go version is older than 1.26, Go downloads
the right toolchain automatically the first time.

## Tests

```bash
make test          # or: go test ./...
```

## Project layout

```
nutrisi/
├── go.mod                      one Go module for the whole repo
├── backend/
│   ├── cmd/server/main.go      entry point: config, HTTP server, graceful shutdown
│   └── internal/
│       ├── config/             settings from environment variables / .env
│       └── server/             Gin engine, routes, handlers, tests
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
| GET    | `/healthz`              | `{"status":"ok"}`               |
| GET    | `/partials/server-time` | htmx fragment (demo)            |
| GET    | `/static/*`             | Embedded static files           |

## Roadmap

1. ✅ Skeleton: Gin, templ, htmx, config, graceful shutdown, tests
2. AI Coach: `POST /coach/chat` with a local Ollama model
3. Streaming answers over SSE, cancelled when the browser closes
4. Middleware: rate limiting, structured logging (slog), security headers
5. Chat UI polish: Markdown rendering, history, error states
6. Docker, then PWA (manifest + service worker) → Google Play via a Trusted Web Activity
7. Later: accounts, profile, meal plans, trackers, dashboard, llama.cpp support

## Disclaimer

NutriSI gives general nutrition information. It is not medical advice.
