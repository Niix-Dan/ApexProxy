# Apex Proxy

![status](https://img.shields.io/badge/status-WIP-yellow)
[![Go Report Card](https://goreportcard.com/badge/github.com/niix-dan/apexproxy)](https://goreportcard.com/report/github.com/niix-dan/apexproxy)
[![Go Reference](https://pkg.go.dev/badge/github.com/niix-dan/apexproxy.svg)](https://pkg.go.dev/github.com/niix-dan/apexproxy)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

A lightweight, high-performance reverse proxy and load balancer written in Go, featuring an instant, zero-dependency TUI dashboard for real-time traffic monitoring.

> Work in progress. Core proxy engine and TUI dashboard are functional.

## Features

- Declarative YAML configuration
- Host and path-based routing with priority ordering
- Wildcard subdomain matching (`*.domain.com`)
- Load balancing: round-robin (weighted), ip-hash, single
- Real-time TUI dashboard (`apex status`)
- Internal metrics endpoint on `:9090` (localhost-only, HMAC-signed)

## Installation

```bash
go install github.com/niix-dan/apex@latest
```

## Usage

```bash
apex start --config ./apex.yaml
apex status
```

## Configuration

```yaml
server:
  http_port: 80
  https_port: 443
  auto_tls: false
  tls:
    cert_file: "/etc/ssl/certs/server.crt"
    key_file: "/etc/ssl/certs/server.key"

middlewares:
  rate_limit:
    enabled: true
    requests_per_minute: 1000
  compression:
    enabled: true
    types: ["text/html", "application/json"]

routing:
  - host: api.example.com
    path: /
    strategy: round-robin
    priority: 100
    targets:
      - url: "http://127.0.0.1:3000"
        weight: 3
      - url: "http://127.0.0.1:3001"
        weight: 1

  - host: example.com
    path: /auth
    strategy: ip-hash
    priority: 90
    targets:
      - url: "http://10.0.0.5:8080"

  - host: "*.saas-app.com"
    strategy: dynamic-lookup
    priority: 50
    resolver: "redis://localhost:6379"

  - path: /
    strategy: single
    priority: 1
    targets:
      - url: "http://127.0.0.1:3000"
```

## Roadmap

- [x] YAML config parser
- [x] Reverse proxy via `httputil.ReverseProxy`
- [x] Host, path, and wildcard routing
- [x] Round-robin (weighted) and ip-hash load balancing
- [x] Metrics collection (latency, bandwidth, status codes, per-route stats)
- [x] TUI dashboard
- [ ] Hot-reload via `fsnotify` (no dropped connections)
- [ ] Rate limiting (token bucket)
- [ ] Response compression
- [x] Automatic TLS via Let's Encrypt
- [ ] `dynamic-lookup` strategy (Redis)
- [ ] `apex init` command
- [ ] Unit tests

## License

MIT
