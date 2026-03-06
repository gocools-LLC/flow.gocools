package ingestion

import "testing"

func TestRuntimeConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("FLOW_INGEST_MODE", "")
	t.Setenv("FLOW_INGEST_INTERVAL", "")
	t.Setenv("FLOW_INGEST_WINDOW", "")
	t.Setenv("FLOW_INGEST_TIMEOUT", "")
	t.Setenv("FLOW_CW_LOG_GROUP", "")
	t.Setenv("FLOW_CW_FILTER_PATTERN", "")
	t.Setenv("FLOW_CW_LIMIT", "")

	cfg := RuntimeConfigFromEnv()
	if cfg.NormalizedMode() != ModeDisabled {
		t.Fatalf("expected disabled mode, got %q", cfg.NormalizedMode())
	}
	if cfg.PollEvery() != defaultPollInterval {
		t.Fatalf("expected default poll interval %s, got %s", defaultPollInterval, cfg.PollEvery())
	}
	if cfg.Window() != defaultCollectWindow {
		t.Fatalf("expected default collect window %s, got %s", defaultCollectWindow, cfg.Window())
	}
	if cfg.Timeout() != defaultRequestTimeout {
		t.Fatalf("expected default timeout %s, got %s", defaultRequestTimeout, cfg.Timeout())
	}
	if cfg.RecordLimit() != defaultRecordLimit {
		t.Fatalf("expected default limit %d, got %d", defaultRecordLimit, cfg.RecordLimit())
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled mode should validate: %v", err)
	}
}

func TestRuntimeConfigValidateCloudWatchLogs(t *testing.T) {
	t.Setenv("FLOW_INGEST_MODE", "CLOUDWATCH_LOGS")
	t.Setenv("FLOW_CW_LOG_GROUP", "/aws/ecs/dev")
	t.Setenv("FLOW_INGEST_INTERVAL", "10s")
	t.Setenv("FLOW_INGEST_WINDOW", "2m")
	t.Setenv("FLOW_INGEST_TIMEOUT", "4s")
	t.Setenv("FLOW_CW_LIMIT", "42")

	cfg := RuntimeConfigFromEnv()
	if cfg.NormalizedMode() != ModeCloudWatchLog {
		t.Fatalf("expected cloudwatch mode, got %q", cfg.NormalizedMode())
	}
	if cfg.PollEvery().String() != "10s" {
		t.Fatalf("expected poll interval 10s, got %s", cfg.PollEvery())
	}
	if cfg.Window().String() != "2m0s" {
		t.Fatalf("expected collect window 2m, got %s", cfg.Window())
	}
	if cfg.Timeout().String() != "4s" {
		t.Fatalf("expected timeout 4s, got %s", cfg.Timeout())
	}
	if cfg.RecordLimit() != 42 {
		t.Fatalf("expected limit 42, got %d", cfg.RecordLimit())
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid cloudwatch runtime config: %v", err)
	}
}

func TestRuntimeConfigValidateCloudWatchLogsMissingGroup(t *testing.T) {
	t.Setenv("FLOW_INGEST_MODE", "cloudwatch_logs")
	t.Setenv("FLOW_CW_LOG_GROUP", "")

	cfg := RuntimeConfigFromEnv()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing log group")
	}
}

func TestRuntimeConfigValidateRejectsUnknownMode(t *testing.T) {
	t.Setenv("FLOW_INGEST_MODE", "unknown")

	cfg := RuntimeConfigFromEnv()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for unsupported mode")
	}
}
