package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig `yaml:"server"`
	Proxy   Middlewares  `yaml:"middlewares"`
	Routing []Route      `yaml:"routing"`
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
	Compression CompressionConfig `yaml:"compression"`
}

type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
}

type CompressionConfig struct {
	Enabled bool     `yaml:"enabled"`
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

	return &cfg, nil
}
