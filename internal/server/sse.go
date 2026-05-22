package server

import (
	"fmt"
	"net/http"
	"strings"
)

// sseWriter streams Server-Sent Events on an HTTP response, flushing after
// each event so the client sees progress live.
type sseWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

// newSSEWriter sets the SSE response headers and returns a writer, or an error
// when the ResponseWriter cannot stream (no http.Flusher). Compression
// middleware must not wrap this route — it would buffer the stream.
func newSSEWriter(w http.ResponseWriter) (*sseWriter, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("server: response writer does not support streaming")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // disable proxy buffering
	return &sseWriter{w: w, f: f}, nil
}

// event writes one SSE event terminated by a blank line and flushes. Newlines
// in data are collapsed to spaces so they cannot break the SSE framing.
func (s *sseWriter) event(name, data string) {
	data = strings.ReplaceAll(data, "\r", "")
	data = strings.ReplaceAll(data, "\n", " ")
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, data)
	s.f.Flush()
}
