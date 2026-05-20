# Apex Proxy

A high-performance, CLI-first reverse proxy and load balancer written in Go. Built to replace legacy web servers with a developer-first experience, native hot-reloading, real-time metrics, and dynamic routing for modern microservices and SaaS architectures.

## Core Features

*   **Single Binary:** No complex dependencies or background daemons. Download and run.
*   **Declarative Configuration:** Human-readable YAML mapping for hosts, paths, and middlewares.
*   **Hot Reloading:** Watch mode applies configuration changes instantly without dropping active TCP connections.
*   **Dynamic Subdomain Routing:** Built-in support for host-based routing, including wildcard domains.
*   **Load Balancing:** Native strategies including Round-Robin, IP-Hash, and Least-Connections.
*   **TUI Dashboard:** Real-time terminal user interface for monitoring traffic, latencies, and node health.
*   **Edge Middlewares:** Rate limiting, IP blacklisting, and payload compression out of the box.

## Installation

```bash
go install github.com/niix-dan/apex@latest
```

## CLI Reference

The CLI is the primary interface for managing the proxy.

* `apex init` - Generates a boilerplate `apex.yaml` configuration file in the current directory.
* `apex start --config ./apex.yaml` - Starts the proxy server based on the provided configuration.
* `apex watch --config ./apex.yaml` - Starts the server and listens for filesystem changes on the config file, reloading the routing table seamlessly.
* `apex status` - Launches the interactive TUI (Terminal User Interface) to display real-time metrics.

## Configuration Specification

Apex uses a straightforward YAML configuration. It matches incoming requests by `host` and `path`, processes them through `middlewares`, and forwards them to `targets` using a defined `strategy`.

```yaml
# apex.yaml

server:
  http_port: 80
  https_port: 443
  auto_tls: true

middlewares:
  rate_limit:
    enabled: true
    requests_per_minute: 1000
  compression:
    enabled: true
    types: ["text/html", "application/json"]

routing:
  # Host-based routing for API subdomain
  - host: api.example.com
    path: /
    strategy: round-robin
    targets:
      - url: "http://127.0.0.1:3000"
      - url: "http://127.0.0.1:3001"

  # Path-based routing for the main domain
  - host: example.com
    path: /auth
    strategy: ip-hash
    targets:
      - url: "http://10.0.0.5:8080"

  # Catch-all for the main application
  - host: example.com
    path: /
    strategy: single
    targets:
      - url: "http://127.0.0.1:5173"

  # Wildcard SaaS routing
  - host: "*.saas-app.com"
    strategy: dynamic-lookup
    resolver: "redis://localhost:6379"

```

## Architecture & Data Flow

When developing or contributing to Apex, keep the following request lifecycle in mind:

1. **Listener Layer:** Go `net/http` server accepts incoming connections.
2. **Middleware Chain:** Request passes through global and route-specific interceptors (Rate Limit, Auth, Logging).
3. **Routing Engine:** The system extracts the `Host` header and URL `Path` to find a matching rule in the routing table (stored in memory).
4. **Load Balancer:** If multiple targets exist, the selected strategy algorithm determines the backend node.
5. **Reverse Proxy Transport:** `httputil.ReverseProxy` (or custom transport) pipes the request to the target and streams the response back to the client.

## Development Roadmap

This section serves as the internal checklist for building the core engine in Go.

### Phase 1: Core Proxy Engine

* [ ] Setup Go project and CLI framework (using `spf13/cobra`).
* [ ] Implement YAML parser to load structures into memory.
* [ ] Create the basic Reverse Proxy using Go's `httputil.ReverseProxy`.
* [ ] Implement host and path matching logic.

### Phase 2: Load Balancing & State

* [ ] Implement `round-robin` load balancing algorithm.
* [ ] Implement `ip-hash` load balancing algorithm.
* [ ] Add basic health-checking for target nodes (remove dead nodes from rotation).

### Phase 3: Developer Experience

* [ ] Implement `fsnotify` to watch the YAML file for changes.
* [ ] Build the graceful reload mechanism (swapping the routing table pointer using atomic operations or mutexes without dropping connections).
* [ ] Build the terminal dashboard using a library like `charmbracelet/bubbletea` or `gizak/termui`.

### Phase 4: Middlewares & Production Readiness

* [ ] Implement Rate Limiting using Token Bucket algorithm.
* [ ] Integrate automatic TLS via Let's Encrypt (using `golang.org/x/crypto/acme/autocert`).
* [ ] Write unit tests for the routing and load balancing algorithms.

## License

MIT License.
