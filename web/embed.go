// Package web holds the embedded HTML templates and static assets for the
// job-scraper web UI.
package web

import "embed"

// FS holds the HTML templates and static assets (CSS, JS, favicons,
// vendored fonts) for the UI. Fonts are vendored under vendor/fonts/ so the
// app renders correctly offline — no CDN fetches at runtime.
//
//go:embed *.html *.css *.js *.svg *.ico vendor/fonts/*.woff2
var FS embed.FS
