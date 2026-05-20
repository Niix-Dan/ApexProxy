package cmd

import (
	"fmt"
	"log"
	"net/http"

	"github.com/niix-dan/apexproxy/internal/config"
	"github.com/niix-dan/apexproxy/internal/metrics"
	"github.com/niix-dan/apexproxy/internal/proxy"

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
			http.HandleFunc("/metrics", metrics.HTTPHandler)
			http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
				metrics.Instance.Reset()
				w.WriteHeader(http.StatusOK)
			})
			_ = http.ListenAndServe("127.0.0.1:9090", nil)
		}()

		router := proxy.NewRouter(cfg)
		addr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)

		log.Printf("Starting Apex Proxy on port %d...", cfg.Server.HTTPPort)
		if err := http.ListenAndServe(addr, router); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	},
}

func init() {
	startCmd.Flags().StringVarP(&configPath, "config", "c", "apex.yaml", "Path to configuration file")
	rootCmd.AddCommand(startCmd)
}
