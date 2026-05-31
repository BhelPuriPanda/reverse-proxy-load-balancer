package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type contextKey string

const (
	// RetryCountKey is used as a context key to track the number of retries for a given request.
	RetryCountKey contextKey = "retry_count"

	// MaxRetries is the maximum number of times a single incoming client request is allowed to
	// be retried on alternative healthy backend nodes before returning an error page.
	MaxRetries = 3
)

// createReverseProxy instantiates and configures a standard ReverseProxy for a specific backend URL.
// It assigns a custom ErrorHandler to implement seamless automatic failovers when a node goes down.
func createReverseProxy(target *url.URL, pool *ServerPool) *httputil.ReverseProxy {
	// NewSingleHostReverseProxy returns a new ReverseProxy that routes URLs to target.
	// It automatically sets the standard request headers (like X-Forwarded-For, X-Forwarded-Proto, etc.)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Custom ErrorHandler catches connection failures (like dial timeout or connection refused)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy connection failed for target %s: %v. Initiating failover...", target.String(), err)

		// 1. Mark this specific backend node as offline immediately
		pool.MarkBackendStatus(target, false)

		// 2. Fetch the current retry attempt count from the request context
		retries := 0
		if val := r.Context().Value(RetryCountKey); val != nil {
			retries = val.(int)
		}

		// 3. Prevent infinite routing loops if all backends in the pool are offline
		if retries >= MaxRetries {
			log.Printf("Max retries (%d) reached for client request %s. Aborting failover.", MaxRetries, r.URL.Path)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		// 4. Enrich the request context with an incremented retry counter
		ctx := context.WithValue(r.Context(), RetryCountKey, retries+1)

		// 5. Re-dispatch the request to the main server pool.
		// ServeHTTP will select the next active backend via Round Robin and route traffic to it.
		pool.ServeHTTP(w, r.WithContext(ctx))
	}

	return proxy
}
