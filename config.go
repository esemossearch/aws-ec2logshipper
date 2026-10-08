package main

import (
	"encoding/json"
	"errors"
	"os"
)

// Config holds the JSON configuration for the EC2 log shipper.
type Config struct {
	LogFile         string `json:"log_file"`          // Local log file or glob pattern to tail.
	LogFileMode     string `json:"log_file_mode"`     // Match mode: latest or all.
	LogGroup        string `json:"log_group"`         // CloudWatch Logs group name.
	LogStream       string `json:"log_stream"`        // CloudWatch Logs stream name (empty means use instance ID).
	TimestampLayout string `json:"timestamp_layout"`  // Go time layout for parsing timestamps.
	TimestampRegex  string `json:"timestamp_regex"`   // Regex with a capturing group for the timestamp.
	Region          string `json:"region"`            // AWS region for CloudWatch Logs.
	IMDSv2          bool   `json:"imdsv2"`            // Use IMDSv2 when resolving the instance ID.
	BatchMaxSize    int    `json:"batch_max_size"`    // Maximum events to send in a single PutLogEvents call.
	FlushIntervalMs int    `json:"flush_interval_ms"` // Flush pending events on this interval.
	CreateLogGroup  bool   `json:"create_log_group"`  // Create the CloudWatch Logs group if missing.
	CreateLogStream bool   `json:"create_log_stream"` // Create the CloudWatch Logs stream if missing.
}

// Defaults fills in zero values that should use a built-in default.
func (c *Config) Defaults() {
	if c.LogFileMode == "" {
		c.LogFileMode = "latest"
	}
	if c.BatchMaxSize <= 0 {
		c.BatchMaxSize = 100
	}
	if c.FlushIntervalMs <= 0 {
		c.FlushIntervalMs = 500
	}
	if !c.IMDSv2 {
		c.IMDSv2 = true
	}
}

// Validate returns an error if any required configuration is missing.
func (c *Config) Validate() error {
	if c.LogFile == "" {
		return errors.New("log_file is required")
	}
	if c.LogFileMode != "latest" && c.LogFileMode != "all" {
		return errors.New("log_file_mode must be latest or all")
	}
	if c.LogGroup == "" {
		return errors.New("log_group is required")
	}
	if c.Region == "" {
		return errors.New("region is required")
	}
	if c.TimestampLayout == "" {
		return errors.New("timestamp_layout is required")
	}
	return nil
}

// LoadConfig reads a JSON configuration from path, applies defaults, and validates it.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	cfg.Defaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}
