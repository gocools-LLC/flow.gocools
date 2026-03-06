package ingestion

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ModeDisabled      = "disabled"
	ModeCloudWatchLog = "cloudwatch_logs"

	defaultPollInterval   = 30 * time.Second
	defaultCollectWindow  = 5 * time.Minute
	defaultRequestTimeout = 20 * time.Second
	defaultRecordLimit    = 500
)

type RuntimeConfig struct {
	Mode           string
	PollInterval   time.Duration
	CollectWindow  time.Duration
	RequestTimeout time.Duration

	LogGroupName  string
	FilterPattern string
	Limit         int32
}

func RuntimeConfigFromEnv() RuntimeConfig {
	return RuntimeConfig{
		Mode:           envOrDefault("FLOW_INGEST_MODE", ModeDisabled),
		PollInterval:   envDurationOrDefault("FLOW_INGEST_INTERVAL", defaultPollInterval),
		CollectWindow:  envDurationOrDefault("FLOW_INGEST_WINDOW", defaultCollectWindow),
		RequestTimeout: envDurationOrDefault("FLOW_INGEST_TIMEOUT", defaultRequestTimeout),
		LogGroupName:   strings.TrimSpace(os.Getenv("FLOW_CW_LOG_GROUP")),
		FilterPattern:  strings.TrimSpace(os.Getenv("FLOW_CW_FILTER_PATTERN")),
		Limit:          int32(envIntOrDefault("FLOW_CW_LIMIT", defaultRecordLimit)),
	}
}

func (c RuntimeConfig) NormalizedMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		return ModeDisabled
	}
	return mode
}

func (c RuntimeConfig) PollEvery() time.Duration {
	if c.PollInterval <= 0 {
		return defaultPollInterval
	}
	return c.PollInterval
}

func (c RuntimeConfig) Window() time.Duration {
	if c.CollectWindow <= 0 {
		return defaultCollectWindow
	}
	return c.CollectWindow
}

func (c RuntimeConfig) Timeout() time.Duration {
	if c.RequestTimeout <= 0 {
		return defaultRequestTimeout
	}
	return c.RequestTimeout
}

func (c RuntimeConfig) RecordLimit() int32 {
	if c.Limit <= 0 {
		return defaultRecordLimit
	}
	return c.Limit
}

func (c RuntimeConfig) Validate() error {
	switch c.NormalizedMode() {
	case ModeDisabled:
		return nil
	case ModeCloudWatchLog:
		if strings.TrimSpace(c.LogGroupName) == "" {
			return fmt.Errorf("flow cloudwatch logs ingestion requires FLOW_CW_LOG_GROUP")
		}
		return nil
	default:
		return fmt.Errorf("unsupported FLOW_INGEST_MODE: %q", c.Mode)
	}
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envIntOrDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
