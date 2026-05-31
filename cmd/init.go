package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

const defaultYAML = `# Apex Proxy Configuration
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
`

const systemdTemplate = `[Unit]
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
`

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Installs apex globally and configures the systemd service",
	Run: func(cmd *cobra.Command, args []string) {
		if os.Geteuid() != 0 {
			fmt.Println("Error: apex init must be run as root. Try 'sudo apex init'")
			os.Exit(1)
		}

		execPath, err := os.Executable()
		if err != nil {
			fmt.Printf("Error resolving executable path: %v\n", err)
			os.Exit(1)
		}

		binData, err := os.ReadFile(execPath)
		if err != nil {
			fmt.Printf("Error reading binary: %v\n", err)
			os.Exit(1)
		}

		err = os.WriteFile("/usr/local/bin/apex", binData, 0755)
		if err != nil {
			fmt.Printf("Error writing binary to /usr/local/bin: %v\n", err)
			os.Exit(1)
		}

		err = os.MkdirAll("/etc/apex", 0755)
		if err != nil {
			fmt.Printf("Error creating /etc/apex directory: %v\n", err)
			os.Exit(1)
		}

		if _, err := os.Stat("/etc/apex/apex.yaml"); os.IsNotExist(err) {
			err = os.WriteFile("/etc/apex/apex.yaml", []byte(defaultYAML), 0644)
			if err != nil {
				fmt.Printf("Error writing config file: %v\n", err)
				os.Exit(1)
			}
		}

		err = os.WriteFile("/etc/systemd/system/apexproxy.service", []byte(systemdTemplate), 0644)
		if err != nil {
			fmt.Printf("Error writing systemd service: %v\n", err)
			os.Exit(1)
		}

		exec.Command("systemctl", "daemon-reload").Run()
		exec.Command("systemctl", "enable", "apexproxy").Run()
		exec.Command("systemctl", "restart", "apexproxy").Run()

		fmt.Println("Installation complete.")
		fmt.Println("Binary path: /usr/local/bin/apex")
		fmt.Println("Config path: /etc/apex/apex.yaml")
		fmt.Println("Service:     apexproxy (running)")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
