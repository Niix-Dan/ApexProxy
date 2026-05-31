# Apex Proxy

![status](https://img.shields.io/badge/status-WIP-yellow)
[![Go Report Card](https://goreportcard.com/badge/github.com/niix-dan/apexproxy)](https://goreportcard.com/report/github.com/niix-dan/apexproxy)
[![Go Reference](https://pkg.go.dev/badge/github.com/niix-dan/apexproxy.svg)](https://pkg.go.dev/github.com/niix-dan/apexproxy)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

A lightweight, high-performance reverse proxy and load balancer written in Go, featuring a zero-dependency TUI dashboard for real-time traffic monitoring.

> **Note:** Work in progress. The core proxy engine and TUI dashboard are functional.

## Features

- Declarative YAML configuration
- Host and path-based routing with priority ordering
- Wildcard subdomain matching (`*.domain.com`)
- Load balancing: round-robin (weighted), ip-hash, single
- Real-time TUI dashboard (`apex status`)
- Internal metrics endpoint on `:9090` (localhost-only, HMAC-signed)

## Quick Start

### Installation


### Installation
```bash
go install github.com/niix-dan/apexproxy@latest
```

### Initialization (Linux / systemd)

Run the init command as root to install the binary to `/usr/local/bin`, generate the default configuration in `/etc/apex/apex.yaml`, and start the systemd background service:

```bash
sudo $(which apexproxy) init
```

### CLI Usage

```bash
# Start the proxy server in foreground (uses apex.yaml by default)
apex start --config /etc/apex/apex.yaml

# Open the real-time TUI metrics dashboard
apex status
```

## Configuration Example

The configuration is managed via `/etc/apex/apex.yaml`:

```yaml
# Apex Proxy Configuration
# High-performance reverse proxy configuration file

server:
  http_port: 80
  https_port: 443
  auto_tls: true

logging:
  csv_enabled: true
  csv_path: "/var/log/apex.csv"
  redact_headers: ["Authorization", "Cookie"]

middlewares:
  ip_filter:
    blacklist_cidrs:
      - "10.0.0.0/8"
    whitelist_cidrs: []
  rate_limit:
    enabled: true
    requests_per_minute: 300
  compression:
    enabled: true
    level: 5 # Compression level (1-9)
    types: ["text/html", "application/json"]
  cache:
    enabled: true
    max_entries: 2000
    ttl_seconds: 60
    max_body_bytes: 524288 # 512 KB

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
    path: /
    strategy: single
    priority: 10
    targets:
      - url: "http://127.0.0.1:5173"
```

## Roadmap

- [x] YAML config parser
- [x] Reverse proxy via `httputil.ReverseProxy`
- [x] Host, path, and wildcard routing
- [x] Round-robin (weighted) and ip-hash load balancing
- [x] Metrics collection (latency, bandwidth, status codes, per-route stats)
- [x] TUI dashboard
- [x] `apex init` command
- [x] Automatic TLS via Let's Encrypt
- [x] Response compression middleware
- [x] IpFilter middleware (Whitelist & Blacklist)
- [x] RateLimit middleware
- [x] Cache middleware
- [ ] Hot-reload via `fsnotify` (no dropped connections)
- [ ] `dynamic-lookup` strategy (Redis)
- [ ] Unit tests

## License

MIT
