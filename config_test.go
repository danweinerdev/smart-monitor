package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig_FullYAML(t *testing.T) {
	path := writeTestConfig(t, `
influxdb:
  url: "http://10.0.0.1:8086"
  token: "my-token"
  org: "my-org"
  bucket: "my-bucket"
interval: 15m
hostname: "nas01"
log_level: debug
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.InfluxDB.URL != "http://10.0.0.1:8086" {
		t.Errorf("url = %q, want %q", cfg.InfluxDB.URL, "http://10.0.0.1:8086")
	}
	if cfg.InfluxDB.Token != "my-token" {
		t.Errorf("token = %q, want %q", cfg.InfluxDB.Token, "my-token")
	}
	if cfg.InfluxDB.Org != "my-org" {
		t.Errorf("org = %q, want %q", cfg.InfluxDB.Org, "my-org")
	}
	if cfg.InfluxDB.Bucket != "my-bucket" {
		t.Errorf("bucket = %q, want %q", cfg.InfluxDB.Bucket, "my-bucket")
	}
	if cfg.Interval != 15*time.Minute {
		t.Errorf("interval = %v, want %v", cfg.Interval, 15*time.Minute)
	}
	if cfg.Hostname != "nas01" {
		t.Errorf("hostname = %q, want %q", cfg.Hostname, "nas01")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log_level = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	path := writeTestConfig(t, `
influxdb:
  url: "http://10.0.0.1:8086"
  token: "my-token"
  org: "my-org"
  bucket: "my-bucket"
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Interval != 30*time.Minute {
		t.Errorf("default interval = %v, want %v", cfg.Interval, 30*time.Minute)
	}
	if cfg.Hostname == "" {
		t.Error("hostname should default to os.Hostname()")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("default log_level = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	path := writeTestConfig(t, `
influxdb:
  url: "http://10.0.0.1:8086"
  token: "yaml-token"
  org: "yaml-org"
  bucket: "yaml-bucket"
hostname: "yaml-host"
`)

	t.Setenv("SM_INFLUXDB_URL", "http://env-host:8086")
	t.Setenv("SM_INFLUXDB_TOKEN", "env-token")
	t.Setenv("SM_INFLUXDB_ORG", "env-org")
	t.Setenv("SM_INFLUXDB_BUCKET", "env-bucket")
	t.Setenv("SM_INTERVAL", "5m")
	t.Setenv("SM_HOSTNAME", "env-host")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.InfluxDB.URL != "http://env-host:8086" {
		t.Errorf("url = %q, want env override", cfg.InfluxDB.URL)
	}
	if cfg.InfluxDB.Token != "env-token" {
		t.Errorf("token = %q, want env override", cfg.InfluxDB.Token)
	}
	if cfg.InfluxDB.Org != "env-org" {
		t.Errorf("org = %q, want env override", cfg.InfluxDB.Org)
	}
	if cfg.InfluxDB.Bucket != "env-bucket" {
		t.Errorf("bucket = %q, want env override", cfg.InfluxDB.Bucket)
	}
	if cfg.Interval != 5*time.Minute {
		t.Errorf("interval = %v, want env override 5m", cfg.Interval)
	}
	if cfg.Hostname != "env-host" {
		t.Errorf("hostname = %q, want env override", cfg.Hostname)
	}
}

func TestLoadConfig_PartialYAMLWithEnv(t *testing.T) {
	path := writeTestConfig(t, `
influxdb:
  url: "http://10.0.0.1:8086"
  org: "my-org"
`)

	t.Setenv("SM_INFLUXDB_TOKEN", "env-token")
	t.Setenv("SM_INFLUXDB_BUCKET", "env-bucket")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.InfluxDB.URL != "http://10.0.0.1:8086" {
		t.Errorf("url = %q, want yaml value", cfg.InfluxDB.URL)
	}
	if cfg.InfluxDB.Token != "env-token" {
		t.Errorf("token = %q, want env value", cfg.InfluxDB.Token)
	}
	if cfg.InfluxDB.Org != "my-org" {
		t.Errorf("org = %q, want yaml value", cfg.InfluxDB.Org)
	}
	if cfg.InfluxDB.Bucket != "env-bucket" {
		t.Errorf("bucket = %q, want env value", cfg.InfluxDB.Bucket)
	}
}

func TestLoadConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "missing url",
			yaml:    `influxdb: {token: "t", org: "o", bucket: "b"}`,
			wantErr: "influxdb.url is required",
		},
		{
			name:    "missing token",
			yaml:    `influxdb: {url: "http://x", org: "o", bucket: "b"}`,
			wantErr: "influxdb.token is required",
		},
		{
			name:    "missing org",
			yaml:    `influxdb: {url: "http://x", token: "t", bucket: "b"}`,
			wantErr: "influxdb.org is required",
		},
		{
			name:    "missing bucket",
			yaml:    `influxdb: {url: "http://x", token: "t", org: "o"}`,
			wantErr: "influxdb.bucket is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, tt.yaml)
			_, err := LoadConfig(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	path := writeTestConfig(t, `{{{invalid yaml`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
