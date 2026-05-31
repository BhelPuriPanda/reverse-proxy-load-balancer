# Reverse Proxy Load Balancer in Go

A concurrent, active health-checking, failover-capable HTTP Reverse Proxy Load Balancer built in Go, leveraging the standard library (`net/http` and `net/http/httputil`). 

It cycles incoming client requests among active upstream backend nodes using an atomic Round Robin algorithm, isolates routing paths from background health assessments, and automatically reroutes connections if any registered server goes offline.

---

## High-Level Architecture

The system coordinates incoming traffic from clients, acts as an intermediary gateway, schedules background checks, and catches transport-level failures to invoke automatic failover retries.

```
                  ┌──────────────────────────────┐
                  │            Client            │
                  └──────────────┬───────────────┘
                                 │ HTTP requests (port 8080)
                                 ▼
                  ┌──────────────────────────────┐
                  │     Load Balancer Engine     │
                  │   (logger.go Middleware)     │
                  └──────────────┬───────────────┘
                                 │
         ┌───────────────────────┼───────────────────────┐
         │ (ServeHTTP)           │ (ServeHTTP)           │ (ServeHTTP)
         ▼                       ▼                       ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│ Backend 1 (8081)│     │ Backend 2 (8082)│     │ Backend 3 (8083)│
│ [IsAlive()=true]│     │ [IsAlive()=true]│     │ [IsAlive()=true]│
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         └───────────┬───────────┴───────────┬───────────┘
                     ▲                       ▲
                     │ health check ping     │ health check ping
                     │ (GET /health or TCP)  │ (GET /health or TCP)
           ┌─────────┴───────────────────────┴─────────┐
           │        Background Health Checker          │
           │       (healthcheck.go every 5-10s)        │
           └───────────────────────────────────────────┘
```

### Components and Responsibilities:
- **Client**: Triggers HTTP requests on the public load balancer port.
- **Logger Middleware (`logger.go`)**: Wraps incoming handler calls, tracks transaction timestamps, retrieves the selected routing path, and writes a single consolidated log recording method, path, target upstream node, HTTP status, and duration.
- **Server Pool (`balancer.go`)**: Holds pointers to the `Backend` registry structures and implements a thread-safe selection algorithm leveraging modular sequence counters.
- **Reverse Proxy Handler (`proxy.go`)**: Coordinates standard single-host reverse proxy instances, setting tracking headers (`X-Forwarded-For`, `X-Forwarded-Proto`, and `X-Real-IP`). Assigns custom retry hooks using the proxy's `ErrorHandler`.
- **Health Checker Daemon (`healthcheck.go`)**: Periodically scheduler that polls every registered server concurrently using parallel goroutines, keeping status data updated.

---

## Codebase Map

The project is structured under the `load_balancer` directory:

```
load_balancer/
├── go.mod                     # Go module definitions
├── main.go                    # Central entry point; parses configuration flags and starts the server
├── balancer.go                # Holds ServerPool registry, locks, and Round Robin selector
├── proxy.go                   # Connects ReverseProxy instances and manages failover logic
├── healthcheck.go             # Background scheduler that concurrently checks backend statuses
├── logger.go                  # Structured request logger middleware
└── backends/
    └── main.go                # Multi-port mock upstream utility to simulate servers for testing
```

---

## Compilation

Open a command line in the `load_balancer` directory and compile both binaries:

```powershell
# Navigate to the project directory
cd c:\Users\verma\reverse-proxy-load-balancer\load_balancer

# Compile mock backend executable
go build -o backends.exe ./backends

# Compile load balancer engine
go build -o balancer.exe
```

---

## Running and Testing

### 1. Launch Upstream Nodes
Start the mock servers running in parallel on ports `8081`, `8082`, and `8083`. Wrap the list in double quotes to satisfy Windows PowerShell:
```powershell
./backends.exe -ports "8081,8082,8083"
```

### 2. Launch the Load Balancer
In a separate terminal window, start the load balancer on port `8080`, pointing to the upstream targets with a health check tick configured for every 5 seconds:
```powershell
./balancer.exe -port 8080 -backends "http://127.0.0.1:8081,http://127.0.0.1:8082,http://127.0.0.1:8083" -health-check-interval 5
```

### 3. Verify Load Balancing (Round Robin)
In a third terminal window, send client queries using `curl.exe` (using the `.exe` extension in PowerShell ensures raw HTTP bodies without interactive parsing):
```powershell
curl.exe http://127.0.0.1:8080/
curl.exe http://127.0.0.1:8080/
curl.exe http://127.0.0.1:8080/
```
The output will cycle sequentially across ports `8081`, `8082`, and `8083`. The load balancer window logs details for each query:
`2026/06/01 02:00:00 Client: 127.0.0.1:54321 | GET / -> Backend: http://127.0.0.1:8081 | Status: 200 | Duration: 1.12ms`

### 4. Verify Active Health Check Bypass
Shut down your mock servers by pressing `Ctrl+C` in their terminal window.
- Within 5 seconds, the balancer daemon detects the outage and prints:
  `2026/06/01 02:00:05 Backend [http://127.0.0.1:8081] became unreachable. Status set to OFFLINE.` (repeated for ports 8082 and 8083).
- Issuing queries at this stage returns a `503 Service Unavailable` error because no healthy nodes remain.
- Restart the mock servers: `./backends.exe -ports "8081,8082,8083"`. Within 5 seconds, the checker daemon discovers the ports, logs their recovery, and traffic resumes immediately.

### 5. Verify Automatic Failover (Retries)
To test how the system reacts to a mid-transit crash (before the health check tick has updated the backend's status):
- Restart the load balancer including a non-existent fourth port, `8084` (which is not listening):
  ```powershell
  ./balancer.exe -port 8080 -backends "http://127.0.0.1:8081,http://127.0.0.1:8084,http://127.0.0.1:8082,http://127.0.0.1:8083" -health-check-interval 15
  ```
- Send queries using `curl.exe http://127.0.0.1:8080/`.
- When modular selection hits `8084`, the connection is refused.
- The load balancer intercepts the connection failure, immediately flags `8084` as `OFFLINE`, and silently redirects the query to an alternative active port (e.g., `8082`). 
- The client receives a successful response from a working server transparently, and the load balancer records the failover event:
  `2026/06/01 02:10:00 Proxy connection failed for target http://127.0.0.1:8084: dial tcp 127.0.0.1:8084: connectex: No connection could be made because the target machine actively refused it. Initiating failover...`

---

## Future Enhancements
- **Dynamic Configuration**: Implement hot-reloading configurations using file watchers (e.g. `fsnotify`) to add or remove backend nodes without engine restarts.
- **SSL/TLS Termination**: Mount HTTPS listeners inside `main.go` using standard Go certificates support to handle secure termination.
- **Rate Limiting**: Integrate bucket rate-limiting middleware to shield upstreams from flash-crowd bursts.
- **Telemetry Exposure**: Set up a `/metrics` scrape target on a separate admin port using Prometheus client interfaces.