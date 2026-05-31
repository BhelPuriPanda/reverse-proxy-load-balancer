//This file contains the logic for the active health-checking.
// It runs a background loop to verify the health of each registered backend node.
// Checks are done concurrently using goroutines to prevent latency scaling with the pool size.
// If a backend does not respond to HTTP GET /health or a TCP dial within 2 seconds, it is marked OFFLINE.

package main

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// StartHealthChecks spawns a background worker running on a persistent time.Ticker
// to evaluate and update the health status of all registered backend nodes.
func StartHealthChecks(pool *ServerPool, interval time.Duration) {
	ticker := time.NewTicker(interval)

	// Execute an initial health assessment immediately upon start-up
	go checkAllBackends(pool)

	go func() {
		for range ticker.C {
			checkAllBackends(pool)
		}
	}()
}

// checkAllBackends evaluates the connectivity status of all servers in the pool concurrently.
func checkAllBackends(pool *ServerPool) {
	var wg sync.WaitGroup

	for _, b := range pool.backends {
		wg.Add(1)

		// Spawn a dedicated goroutine for each backend so checks run in parallel
		go func(backend *Backend) {
			defer wg.Done()

			alive := pingBackend(backend)
			currentStatus := backend.IsAlive()

			// Log status changes to reduce logging noise while keeping clear audit trails
			if alive != currentStatus {
				backend.SetAlive(alive)
				if alive {
					log.Printf("Backend [%s] recovered. Status set to ONLINE.", backend.URL.String())
				} else {
					log.Printf("Backend [%s] became unreachable. Status set to OFFLINE.", backend.URL.String())
				}
			}
		}(b)
	}

	// Block until all concurrent ping checks complete
	wg.Wait()
}

// pingBackend attempts an HTTP GET request to the backend's health endpoint.
// It falls back to a raw TCP dial check if the HTTP client experiences transient errors.
func pingBackend(b *Backend) bool {
	healthURL := b.URL.String() + "/health"

	client := http.Client{
		Timeout: 2 * time.Second,
		// Disable redirect following so we get the immediate backend response status
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(healthURL)
	if err != nil {
		// HTTP check failed, perform a fallback TCP connection test to verify port availability
		conn, err := net.DialTimeout("tcp", b.URL.Host, 2*time.Second)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}
	defer resp.Body.Close()

	// Consider any HTTP status code in the 2xx range (e.g., 200 OK) as healthy
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
