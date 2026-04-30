// Package web carries the panel UI: templ templates and Tailwind-built static assets.
// Generated artifacts (*_templ.go, static/output.css) are gitignored and rebuilt by
// `task generate` + `task tailwind:build`.
package web

import "embed"

// Static is the compiled-in static asset tree (CSS, htmx.js, favicons).
// The directory is created at build time by `task tailwind:build`; an empty
// placeholder lives in the repo so this embed compiles before the first build.
//
//go:embed all:static
var Static embed.FS
