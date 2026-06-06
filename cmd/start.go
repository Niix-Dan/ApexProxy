package cmd

import (
	"log"
	"net/http"

	"github.com/niix-dan/apexproxy/internal/config"
	"github.com/niix-dan/apexproxy/internal/httpproxy"
	"github.com/niix-dan/apexproxy/internal/metrics"
	"github.com/niix-dan/apexproxy/internal/tcpproxy"

	"github.com/spf13/cobra"
)

var configPath string

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts the proxy server based on configuration",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load(configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}

		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/metrics", metrics.HTTPHandler)
			mux.HandleFunc("/reset", metrics.ResetHandler)
			if err := http.ListenAndServe("127.0.0.1:9090", mux); err != nil {
				log.Printf("Metrics server error: %v", err)
			}
		}()

		if cfg.MQTT.Enabled {
			mqttProxy := tcpproxy.NewMQTTProxy(cfg.MQTT.Port, cfg.MQTT.Targets)

			go func() {
				if err := mqttProxy.Listen(); err != nil {
					log.Fatalf("fatal error in MQTT proxy: %v", err)
				}
			}()
		}

		server, err := httpproxy.NewServer(cfg)
		if err != nil {
			log.Fatalf("Failed to create server: %v", err)
		}

		log.Printf("Starting Apex Proxy (HTTP:%d, HTTPS:%d, AutoTLS:%v)...",
			cfg.Server.HTTPPort, cfg.Server.HTTPSPort, cfg.Server.AutoTLS)

		if err := server.Start(); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	},
}

func init() {
	startCmd.Flags().StringVarP(&configPath, "config", "c", "apex.yaml", "Path to configuration file")
	rootCmd.AddCommand(startCmd)
}
