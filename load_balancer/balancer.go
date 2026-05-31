//THIS FILE CONTAINS THE LOGIC FOR THE LOAD BALANCER IMPLEMENTATION.
//it uses a round-robbin algorithm to distribute the traffic among the backend servers
//checks and sets the health status of the backend servers concurrently using go routines.
//if a server is down, it will be removed from the pool and will not be served requests.

package main

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
)

// Backend represents a single upstream server, maintaining its URL,
// its active status, and its reverse proxy handler.
type Backend struct {
	URL          *url.URL
	Alive        bool
	mux          sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
}

// SetAlive updates the health status of the backend in a thread-safe manner.
func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	b.Alive = alive
	b.mux.Unlock()
}

// IsAlive returns the health status of the backend in a thread-safe manner.
func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	alive := b.Alive
	b.mux.RUnlock()
	return alive
}

// ServerPool manages the list of registered backends and selection state.
type ServerPool struct {
	backends []*Backend
	current  uint64
}

// AddBackend registers a new backend into the server pool.
func (s *ServerPool) AddBackend(b *Backend) {
	s.backends = append(s.backends, b)
}

// GetNextPeer performs a thread-safe round-robin selection of an active backend.
// It cycles through registered backends using an atomic counter and modular arithmetic.
// If all backends are offline, it returns nil.
func (s *ServerPool) GetNextPeer() *Backend {
	n := len(s.backends)
	if n == 0 {
		return nil
	}

	// Iterate through the backends at most once to locate an alive backend node
	for i := 0; i < n; i++ {
		// Safely increment the index using atomic operations
		next := atomic.AddUint64(&s.current, 1)
		idx := (next - 1) % uint64(n)

		if s.backends[idx].IsAlive() {
			return s.backends[idx]
		}
	}

	return nil
}

// MarkBackendStatus updates the health status of a backend matching the provided URL.
func (s *ServerPool) MarkBackendStatus(backendURL *url.URL, alive bool) {
	for _, b := range s.backends {
		if b.URL.String() == backendURL.String() {
			b.SetAlive(alive)
			break
		}
	}
}

// ServeHTTP implements the http.Handler interface. It retrieves the next healthy backend peer
// via GetNextPeer() and routes the client request to its corresponding reverse proxy.
func (s *ServerPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	peer := s.GetNextPeer()
	if peer != nil {
		// Populate the selected backend target address inside the request context for logging
		ctx := context.WithValue(r.Context(), SelectedBackendKey, peer.URL.String())
		peer.ReverseProxy.ServeHTTP(w, r.WithContext(ctx))
		return
	}
	http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
}
