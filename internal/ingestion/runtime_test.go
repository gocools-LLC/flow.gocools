package ingestion

import (
	"strings"
	"testing"

	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
)

func TestRuntimeConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("FLOW_INGEST_MODE", "")
	t.Setenv("FLOW_INGEST_INTERVAL", "")
	t.Setenv("FLOW_INGEST_WINDOW", "")
	t.Setenv("FLOW_INGEST_TIMEOUT", "")
	t.Setenv("FLOW_CW_LOG_GROUP", "")
	t.Setenv("FLOW_CW_FILTER_PATTERN", "")
	t.Setenv("FLOW_CW_LIMIT", "")
	t.Setenv("FLOW_CW_METRIC_TARGETS", "")
	t.Setenv("FLOW_CW_METRIC_UTIL_WARN", "")
	t.Setenv("FLOW_CW_METRIC_UTIL_ERROR", "")

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
	if cfg.MetricWarnThreshold() != defaultUtilizationWarnPct {
		t.Fatalf("expected default warn threshold %f, got %f", defaultUtilizationWarnPct, cfg.MetricWarnThreshold())
	}
	if cfg.MetricErrorThreshold() != defaultUtilizationErrorPct {
		t.Fatalf("expected default error threshold %f, got %f", defaultUtilizationErrorPct, cfg.MetricErrorThreshold())
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

func TestRuntimeConfigValidateCloudWatchMetrics(t *testing.T) {
	t.Setenv("FLOW_INGEST_MODE", "cloudwatch_metrics")
	t.Setenv("FLOW_CW_METRIC_TARGETS", "ec2:i-123,ecs:cluster-a/service-a")
	t.Setenv("FLOW_CW_METRIC_UTIL_WARN", "65")
	t.Setenv("FLOW_CW_METRIC_UTIL_ERROR", "85")

	cfg := RuntimeConfigFromEnv()
	if cfg.NormalizedMode() != ModeCloudWatchMetric {
		t.Fatalf("expected cloudwatch metrics mode, got %q", cfg.NormalizedMode())
	}
	targets, err := cfg.MetricTargets()
	if err != nil {
		t.Fatalf("expected targets to parse: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 metric targets, got %d", len(targets))
	}
	if targets[0].Kind != cloudwatch.ResourceKindEC2 || targets[0].InstanceID != "i-123" {
		t.Fatalf("unexpected first target: %#v", targets[0])
	}
	if targets[1].Kind != cloudwatch.ResourceKindECSService || targets[1].ClusterName != "cluster-a" || targets[1].ServiceName != "service-a" {
		t.Fatalf("unexpected second target: %#v", targets[1])
	}
	if cfg.MetricWarnThreshold() != 65 {
		t.Fatalf("expected warn threshold 65, got %f", cfg.MetricWarnThreshold())
	}
	if cfg.MetricErrorThreshold() != 85 {
		t.Fatalf("expected error threshold 85, got %f", cfg.MetricErrorThreshold())
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected metrics config to validate: %v", err)
	}
}

func TestRuntimeConfigValidateCloudWatchMetricsMissingTargets(t *testing.T) {
	t.Setenv("FLOW_INGEST_MODE", "cloudwatch_metrics")
	t.Setenv("FLOW_CW_METRIC_TARGETS", "")

	cfg := RuntimeConfigFromEnv()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing metric targets")
	}
}

func TestRuntimeConfigValidateCloudWatchMetricsBadTarget(t *testing.T) {
	t.Setenv("FLOW_INGEST_MODE", "cloudwatch_metrics")
	t.Setenv("FLOW_CW_METRIC_TARGETS", "badtoken")

	cfg := RuntimeConfigFromEnv()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for malformed metric target")
	}
	if !strings.Contains(err.Error(), "FLOW_CW_METRIC_TARGETS") {
		t.Fatalf("expected FLOW_CW_METRIC_TARGETS validation context, got %v", err)
	}
}

func TestRuntimeConfigValidateThresholdOrder(t *testing.T) {
	t.Setenv("FLOW_INGEST_MODE", "cloudwatch_metrics")
	t.Setenv("FLOW_CW_METRIC_TARGETS", "ec2:i-123")
	t.Setenv("FLOW_CW_METRIC_UTIL_WARN", "90")
	t.Setenv("FLOW_CW_METRIC_UTIL_ERROR", "80")

	cfg := RuntimeConfigFromEnv()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected threshold ordering validation error")
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
