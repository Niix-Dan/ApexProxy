package config

import (
	"os"

	"github.com/niix-dan/apexproxy/internal/metrics"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Proxy   Middlewares   `yaml:"middlewares"`
	Routing []Route       `yaml:"routing"`
	Logging LoggingConfig `yaml:"logging"`
	MQTT    MQTTConfig    `yaml:"mqtt"`
}

type MQTTConfig struct {
	Enabled bool     `yaml:"enabled"`
	Port    int      `yaml:"port"`
	Targets []string `yaml:"targets"`
}

type LoggingConfig struct {
	CSVEnabled    bool     `yaml:"csv_enabled"`
	CSVPath       string   `yaml:"csv_path"`
	RedactHeaders []string `yaml:"redact_headers"`
}

type ServerConfig struct {
	HTTPPort  int       `yaml:"http_port"`
	HTTPSPort int       `yaml:"https_port"`
	AutoTLS   bool      `yaml:"auto_tls"`
	TLS       TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type Middlewares struct {
	RateLimit   RateLimitConfig   `yaml:"rate_limit"`
	IPFilter    IPFilterConfig    `yaml:"ip_filter"`
	Cache       CacheConfig       `yaml:"cache"`
	Compression CompressionConfig `yaml:"compression"`
}

type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
}

type IPFilterConfig struct {
	BlacklistCIDRs []string `yaml:"blacklist_cidrs"`
	WhitelistCIDRs []string `yaml:"whitelist_cidrs"`
}

type CacheConfig struct {
	Enabled      bool  `yaml:"enabled"`
	MaxEntries   int   `yaml:"max_entries"`
	TTLSeconds   int   `yaml:"ttl_seconds"`
	MaxBodyBytes int64 `yaml:"max_body_bytes"`
}

type CompressionConfig struct {
	Enabled bool     `yaml:"enabled"`
	Level   int      `yaml:"level"`
	Types   []string `yaml:"types"`
}

type Route struct {
	Host     string    `yaml:"host"`
	Path     string    `yaml:"path"`
	Strategy string    `yaml:"strategy"`
	Priority int       `yaml:"priority"`
	Resolver string    `yaml:"resolver"`
	TLS      TLSConfig `yaml:"tls"`
	Targets  []Target  `yaml:"targets"`
}

type Target struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	metrics.InitCSV(cfg.Logging.CSVEnabled, cfg.Logging.CSVPath, cfg.Logging.RedactHeaders)

	return &cfg, nil
}
