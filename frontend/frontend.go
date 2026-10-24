// Package frontend holds everything the browser sees: templ views (in
// ./views) and static files (in ./static).
package frontend

import "embed"

// Static contains the files under ./static, compiled into the Go binary.
// The backend serves them at /static/...
//
//go:embed static
var Static embed.FS
