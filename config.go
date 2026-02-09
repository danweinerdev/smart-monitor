package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type InfluxDBConfig struct {
	URL    string `yaml:"url"`
	Token  string `yaml:"token"`
	Org    string `yaml:"org"`
	Bucket string `yaml:"bucket"`
}

type Config struct {
	InfluxDB InfluxDBConfig `yaml:"influxdb"`
	Interval time.Duration  `yaml:"interval"`
	Hostname string         `yaml:"hostname"`
	LogLevel string         `yaml:"log_level"`
}

func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		Interval: 30 * time.Minute,
		LogLevel: "info",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyEnvOverrides(cfg)

	if cfg.Hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("getting hostname: %w", err)
		}
		cfg.Hostname = hostname
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SM_INFLUXDB_URL"); v != "" {
		cfg.InfluxDB.URL = v
	}
	if v := os.Getenv("SM_INFLUXDB_TOKEN"); v != "" {
		cfg.InfluxDB.Token = v
	}
	if v := os.Getenv("SM_INFLUXDB_ORG"); v != "" {
		cfg.InfluxDB.Org = v
	}
	if v := os.Getenv("SM_INFLUXDB_BUCKET"); v != "" {
		cfg.InfluxDB.Bucket = v
	}
	if v := os.Getenv("SM_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Interval = d
		}
	}
	if v := os.Getenv("SM_HOSTNAME"); v != "" {
		cfg.Hostname = v
	}
}

func validateConfig(cfg *Config) error {
	if cfg.InfluxDB.URL == "" {
		return fmt.Errorf("influxdb.url is required")
	}
	if cfg.InfluxDB.Token == "" {
		return fmt.Errorf("influxdb.token is required")
	}
	if cfg.InfluxDB.Org == "" {
		return fmt.Errorf("influxdb.org is required")
	}
	if cfg.InfluxDB.Bucket == "" {
		return fmt.Errorf("influxdb.bucket is required")
	}
	if cfg.Interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}
	return nil
}
