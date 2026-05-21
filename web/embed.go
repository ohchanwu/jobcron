// Package web holds the embedded HTML templates and static assets for the
// job-scraper web UI.
package web

import "embed"

// FS holds the HTML templates (*.html) and static assets (*.css) for the UI.
//
//go:embed *.html *.css
var FS embed.FS
