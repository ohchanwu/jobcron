// Package web holds the embedded HTML templates and static assets for the
// job-scraper web UI.
package web

import "embed"

// FS holds the HTML templates and static assets (CSS, JS) for the UI.
//
//go:embed *.html *.css *.js
var FS embed.FS
