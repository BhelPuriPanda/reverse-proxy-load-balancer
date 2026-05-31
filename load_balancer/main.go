// This file serves as the main orchestrator of the reverse proxy load balancer.
// It parses command-line flags to specify ports, target backend URLs, and health-checking intervals.
// It initializes the backend server pool, constructs corresponding reverse proxies,
// spawns background health check routines, and binds the HTTP server wrapped in our logging middleware.

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func main() {
	// 1. Define command-line configuration parameters
	port := flag.Int("port", 8080, "Port on which the load balancer will accept client HTTP traffic")
	backendsOpt := flag.String("backends", "http://127.0.0.1:8081,http://127.0.0.1:8082,http://127.0.0.1:8083", "Comma-separated list of target backend server URLs")
	healthCheckSec := flag.Int("health-check-interval", 10, "Periodic frequency in seconds to run active health check pings")
	flag.Parse()

	// 2. Parse the backend server addresses
	backendList := strings.Split(*backendsOpt, ",")
	if len(backendList) == 0 || backendList[0] == "" {
		log.Fatal("Invalid configuration: Please provide a valid list of backend server URLs")
	}

	pool := &ServerPool{}

	// 3. Populate our ServerPool registry with configured backends
	for _, address := range backendList {
		address = strings.TrimSpace(address)
		targetURL, err := url.Parse(address)
		if err != nil {
			log.Fatalf("Failed to parse backend target URL '%s': %v", address, err)
		}

		// Configure reverse proxy and customize the failover error handler
		proxy := createReverseProxy(targetURL, pool)

		backend := &Backend{
			URL:          targetURL,
			Alive:        true, // Set initially to true; health checker will correct state on first tick
			ReverseProxy: proxy,
		}

		pool.AddBackend(backend)
		log.Printf("Registered backend upstream target: %s", targetURL.String())
	}

	// 4. Start active health check daemons in the background
	interval := time.Duration(*healthCheckSec) * time.Second
	StartHealthChecks(pool, interval)
	log.Printf("Launched active health checking scheduler running every %v", interval)

	// 5. Wrap our main server pool handler inside our timing & metadata Logger Middleware
	loggedHandler := LoggingMiddleware(pool)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      loggedHandler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Reverse Proxy Load Balancer started successfully on port %d...", *port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Load balancer HTTP listener crashed: %v", err)
	}
}
