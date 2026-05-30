package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func main() {
	portsFlag := flag.String("ports", "8081,8082,8083", "Comma-separated list of ports to run backend servers on")
	flag.Parse()

	ports := strings.Split(*portsFlag, ",")
	if len(ports) == 0 || ports[0] == "" {
		log.Fatal("Please specify at least one port via the -ports flag")
	}

	for _, port := range ports {
		port = strings.TrimSpace(port)
		go startServer(port)
	}

	// Keep the main goroutine alive
	select {}
}

func startServer(port string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Support simulating latency
		delayStr := r.URL.Query().Get("delay")
		if delayStr != "" {
			if delay, err := time.ParseDuration(delayStr); err == nil {
				time.Sleep(delay)
			}
		}

		// Support simulating errors
		if r.URL.Query().Get("fail") == "true" {
			http.Error(w, fmt.Sprintf("Backend on port %s failed purposefully", port), http.StatusInternalServerError)
			return
		}

		// Standard successful response
		w.Header().Set("X-Backend-Port", port)
		fmt.Fprintf(w, "Hello from Backend on port %s\n", port)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Support simulating health failure
		if r.URL.Query().Get("fail") == "true" {
			http.Error(w, "Unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Starting mock backend server on port %s...", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server on port %s failed to start: %v", port, err)
	}
}
