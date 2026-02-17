package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	monitor "github.com/danweinerdev/go-monitor"
	"gopkg.in/yaml.v3"
)

type InfluxDBConfig struct {
	URL           string        `yaml:"url"`
	Token         string        `yaml:"token"`
	Org           string        `yaml:"org"`
	Bucket        string        `yaml:"bucket"`
	RetryAttempts int           `yaml:"retry_attempts"`
	RetryDelay    time.Duration `yaml:"retry_delay"`
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
		InfluxDB: InfluxDBConfig{
			RetryAttempts: 3,
			RetryDelay:    2 * time.Second,
		},
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
	var errs monitor.ValidationErrors

	if cfg.InfluxDB.URL == "" {
		errs = append(errs, monitor.ValidationError{Field: "influxdb.url", Message: "is required"})
	}
	if cfg.InfluxDB.Token == "" {
		errs = append(errs, monitor.ValidationError{Field: "influxdb.token", Message: "is required"})
	}
	if cfg.InfluxDB.Org == "" {
		errs = append(errs, monitor.ValidationError{Field: "influxdb.org", Message: "is required"})
	}
	if cfg.InfluxDB.Bucket == "" {
		errs = append(errs, monitor.ValidationError{Field: "influxdb.bucket", Message: "is required"})
	}
	if cfg.Interval <= 0 {
		errs = append(errs, monitor.ValidationError{Field: "interval", Message: "must be positive"})
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(cfg.LogLevel)] {
		errs = append(errs, monitor.ValidationError{Field: "log_level", Message: "must be one of: debug, info, warn, error"})
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// monitorConfig converts our YAML config to a monitor.Config for go-monitor.
func (c *Config) monitorConfig() *monitor.Config {
	return &monitor.Config{
		Global: monitor.GlobalConfig{
			PollInterval:  monitor.Duration{Duration: c.Interval},
			LogLevel:      c.LogLevel,
			BatchSize:     100,
			RetryAttempts: c.InfluxDB.RetryAttempts,
			RetryDelay:    monitor.Duration{Duration: c.InfluxDB.RetryDelay},
		},
		InfluxDB: monitor.InfluxDBConfig{
			Enabled: true,
			URL:     c.InfluxDB.URL,
			Token:   c.InfluxDB.Token,
			Org:     c.InfluxDB.Org,
			Bucket:  c.InfluxDB.Bucket,
		},
	}
}

// influxDBConfig converts our InfluxDBConfig to go-monitor's type.
func (c *Config) influxDBConfig() monitor.InfluxDBConfig {
	return monitor.InfluxDBConfig{
		Enabled: true,
		URL:     c.InfluxDB.URL,
		Token:   c.InfluxDB.Token,
		Org:     c.InfluxDB.Org,
		Bucket:  c.InfluxDB.Bucket,
	}
}
