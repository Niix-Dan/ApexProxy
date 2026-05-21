# Apex Proxy Architectural Documentation

This document describes the CLI layout, and administrative mechanisms implemented inside Apex Proxy.

## 1. CLI Structure & Commands

The CLI layer uses `github.com/spf13/cobra` to expose operational controls:

*   `apex init`: Installs the running binary into `/usr/local/bin`, creates the configuration path `/etc/apex/`, writes the default `apex.yaml` file, and hooks the runtime up to `systemd` via a managed `apexproxy.service` file. Must be executed with root privileges (`UID 0`).
*   `apex start`: Starts the reverse proxy engine. It binds to the configured server ports and mounts an unexposed internal metrics runtime on a separate goroutine (`127.0.0.1:9090`).
*   `apex status`: Fires an isolated Bubble Tea TUI instance that reads real-time stats from the internal metrics endpoint.

### Systemd Service Blueprint
The service file deployed by the lifecycle initialization runs with a high file descriptor threshold to sustain heavy connection volumes:

```ini
[Unit]
Description=Apex Proxy Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/apex start --config /etc/apex/apex.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

## 2. Metrics Engine & Inter-process Communication

Telemetry is separated from standard client data traffic pipelines. The backend mounts a dedicated telemetry router bound to `127.0.0.1:9090` exposing two structural endpoints:

* `GET /metrics`: Returns a structural JSON block mapping active operational status (`ProxyStats`).
* `GET /reset`: Clears the stored metric state variables.

### Security Token Enforcement

To guarantee that local processes requesting system state have authorization clearance, the communication layer utilizes an inline cryptographic signature check via `metrics.SignRequest(req)`. Requests hitting the internal management endpoints without verified authentication headers or from interfaces outside localhost are rejected.

## 3. TUI Architecture (`apex status`)

The terminal visual system uses `github.com/charmbracelet/bubbletea` as an Event-Driven loop executing asynchronously from the primary proxy system.

### Loop Cycle Lifecycle

1. **Init Event**: Fires concurrent `fetchMetrics()` and a time-based tick command (`time.Tick` every 1 second).
2. **Update Event**: Receives either a metric package payload (`responseMsg`), an error context (`errorMsg`), or standard keystrokes.
3. **State Modifiers**:
* `q` / `ctrl+c`: Aborts execution cleanly.
* `p`: Sets a `paused` Boolean flag inside the internal data model, halting viewport updates while the backend continue accumulating structural traffic data.
* `r`: Dispatches an automated network trigger payload targeting the `/reset` handler to set operational values back to baseline zero.
