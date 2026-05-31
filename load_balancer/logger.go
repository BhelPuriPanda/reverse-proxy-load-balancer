// This file contains the middleware logic for structured request logging.
// It intercepts incoming requests, starts a stopwatch, tracks the selected backend,
// captures the response status code, and outputs a formatted structured log line showing latency.

package main

import (
	"log"
	"net/http"
	"time"
)

type loggerContextKey string

const (
	// SelectedBackendKey is the context key used to store the selected backend URL string.
	SelectedBackendKey loggerContextKey = "selected_backend"
)

// LoggingResponseWriter wraps a standard http.ResponseWriter to capture the HTTP status code
// written by the downstream handlers (like the reverse proxy), which is otherwise hidden.
type LoggingResponseWriter struct {
	http.ResponseWriter
	StatusCode int
}

// WriteHeader records the status code before writing it to the underlying client connection.
func (lrw *LoggingResponseWriter) WriteHeader(code int) {
	lrw.StatusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// LoggingMiddleware wraps an existing http.Handler to intercept requests, measure latency,
// and output structured request metadata to the standard log stream.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap the writer, defaulting status code to 200 OK (since http.Server does the same
		// if WriteHeader is not explicitly called by the handler).
		lrw := &LoggingResponseWriter{
			ResponseWriter: w,
			StatusCode:     http.StatusOK,
		}

		// Process downstream request forwarding
		next.ServeHTTP(lrw, r)

		duration := time.Since(start)

		// Retrieve the selected backend URL from the request context (populated during ServeHTTP)
		backend := "none"
		if val := r.Context().Value(SelectedBackendKey); val != nil {
			backend = val.(string)
		}

		// Log structured metadata: Client IP, HTTP Method, Requested Path, Selected Backend, Status, Latency
		log.Printf(
			"Client: %s | %s %s -> Backend: %s | Status: %d | Duration: %s",
			r.Header.Get("X-Forwarded-For"),
			r.Method,
			r.URL.Path,
			backend,
			lrw.StatusCode,
			duration,
		)
	})
}
