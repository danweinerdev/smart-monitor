package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type InfluxDBConfig struct {
	URL           string `yaml:"url"`
	Token         string `yaml:"token"`
	Org           string `yaml:"org"`
	Bucket        string `yaml:"bucket"`
	RetryAttempts int    `yaml:"retry_attempts"`
	RetryDelay    time.Duration `yaml:"retry_delay"`
}

type Config struct {
	InfluxDB InfluxDBConfig `yaml:"influxdb"`
	Interval time.Duration  `yaml:"interval"`
	Hostname string         `yaml:"hostname"`
	LogLevel string         `yaml:"log_level"`
	RunOnce  bool           `yaml:"-"`
}

// ValidationError represents a single config validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors collects multiple validation errors.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 1 {
		return e[0].Error()
	}
	msgs := make([]string, len(e))
	for i, err := range e {
		msgs[i] = err.Error()
	}
	return fmt.Sprintf("config validation failed:\n  - %s", strings.Join(msgs, "\n  - "))
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
	var errs ValidationErrors

	if cfg.InfluxDB.URL == "" {
		errs = append(errs, ValidationError{Field: "influxdb.url", Message: "is required"})
	}
	if cfg.InfluxDB.Token == "" {
		errs = append(errs, ValidationError{Field: "influxdb.token", Message: "is required"})
	}
	if cfg.InfluxDB.Org == "" {
		errs = append(errs, ValidationError{Field: "influxdb.org", Message: "is required"})
	}
	if cfg.InfluxDB.Bucket == "" {
		errs = append(errs, ValidationError{Field: "influxdb.bucket", Message: "is required"})
	}
	if cfg.Interval <= 0 {
		errs = append(errs, ValidationError{Field: "interval", Message: "must be positive"})
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(cfg.LogLevel)] {
		errs = append(errs, ValidationError{Field: "log_level", Message: "must be one of: debug, info, warn, error"})
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// ParseLogLevel converts a log level string to slog.Level.
func ParseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
