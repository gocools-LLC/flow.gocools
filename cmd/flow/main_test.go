package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/gocools-LLC/flow.gocools/internal/analyzer/timeline"
	internalaws "github.com/gocools-LLC/flow.gocools/internal/aws"
	"github.com/gocools-LLC/flow.gocools/internal/ingestion"
	telemetryarchive "github.com/gocools-LLC/flow.gocools/internal/telemetry/archive"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
)

func TestStartTimelineIngestionDisabledMode(t *testing.T) {
	err := startTimelineIngestion(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		internalaws.RuntimeConfig{},
		ingestion.RuntimeConfig{Mode: ingestion.ModeDisabled},
		timeline.NewInMemoryService(nil),
		nil,
	)
	if err != nil {
		t.Fatalf("expected disabled mode to succeed, got %v", err)
	}
}

func TestStartTimelineIngestionCloudWatchLogsModeRequiresRegion(t *testing.T) {
	err := startTimelineIngestion(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		internalaws.RuntimeConfig{},
		ingestion.RuntimeConfig{
			Mode:         ingestion.ModeCloudWatchLog,
			LogGroupName: "/aws/ecs/dev",
		},
		timeline.NewInMemoryService(nil),
		nil,
	)
	if err == nil {
		t.Fatal("expected missing region error")
	}
	if !strings.Contains(err.Error(), "FLOW_AWS_REGION or AWS_REGION") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartTimelineIngestionCloudWatchMetricsModeRequiresRegion(t *testing.T) {
	err := startTimelineIngestion(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		internalaws.RuntimeConfig{},
		ingestion.RuntimeConfig{
			Mode:             ingestion.ModeCloudWatchMetric,
			MetricTargetsRaw: "ec2:i-123",
		},
		timeline.NewInMemoryService(nil),
		nil,
	)
	if err == nil {
		t.Fatal("expected missing region error")
	}
	if !strings.Contains(err.Error(), "FLOW_AWS_REGION or AWS_REGION") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartTimelineIngestionCloudWatchMetricsModeRequiresTargets(t *testing.T) {
	err := startTimelineIngestion(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		internalaws.RuntimeConfig{
			Session: internalaws.SessionConfig{
				Region: "us-east-1",
			},
		},
		ingestion.RuntimeConfig{
			Mode: ingestion.ModeCloudWatchMetric,
		},
		timeline.NewInMemoryService(nil),
		nil,
	)
	if err == nil {
		t.Fatal("expected missing targets error")
	}
	if !strings.Contains(err.Error(), "FLOW_CW_METRIC_TARGETS") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartTimelineIngestionCloudWatchMetricsModeRejectsBadTargets(t *testing.T) {
	err := startTimelineIngestion(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		internalaws.RuntimeConfig{
			Session: internalaws.SessionConfig{
				Region: "us-east-1",
			},
		},
		ingestion.RuntimeConfig{
			Mode:             ingestion.ModeCloudWatchMetric,
			MetricTargetsRaw: "bad-target",
		},
		timeline.NewInMemoryService(nil),
		nil,
	)
	if err == nil {
		t.Fatal("expected malformed targets error")
	}
	if !strings.Contains(err.Error(), "FLOW_CW_METRIC_TARGETS") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartTimelineIngestionCloudWatchAllModeRequiresRegion(t *testing.T) {
	err := startTimelineIngestion(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		internalaws.RuntimeConfig{},
		ingestion.RuntimeConfig{
			Mode:             ingestion.ModeCloudWatchAll,
			LogGroupName:     "/aws/ecs/dev",
			MetricTargetsRaw: "ec2:i-123",
		},
		timeline.NewInMemoryService(nil),
		nil,
	)
	if err == nil {
		t.Fatal("expected missing region error")
	}
	if !strings.Contains(err.Error(), "FLOW_AWS_REGION or AWS_REGION") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartTimelineIngestionCloudWatchAllModeRequiresLogGroup(t *testing.T) {
	err := startTimelineIngestion(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		internalaws.RuntimeConfig{
			Session: internalaws.SessionConfig{
				Region: "us-east-1",
			},
		},
		ingestion.RuntimeConfig{
			Mode:             ingestion.ModeCloudWatchAll,
			LogGroupName:     "",
			MetricTargetsRaw: "ec2:i-123",
		},
		timeline.NewInMemoryService(nil),
		nil,
	)
	if err == nil {
		t.Fatal("expected missing log group error")
	}
	if !strings.Contains(err.Error(), "FLOW_CW_LOG_GROUP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartTimelineIngestionCloudWatchAllModeRequiresMetricTargets(t *testing.T) {
	err := startTimelineIngestion(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		internalaws.RuntimeConfig{
			Session: internalaws.SessionConfig{
				Region: "us-east-1",
			},
		},
		ingestion.RuntimeConfig{
			Mode:             ingestion.ModeCloudWatchAll,
			LogGroupName:     "/aws/ecs/dev",
			MetricTargetsRaw: "",
		},
		timeline.NewInMemoryService(nil),
		nil,
	)
	if err == nil {
		t.Fatal("expected missing metric targets error")
	}
	if !strings.Contains(err.Error(), "FLOW_CW_METRIC_TARGETS") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartTimelineIngestionRejectsUnknownMode(t *testing.T) {
	err := startTimelineIngestion(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		internalaws.RuntimeConfig{},
		ingestion.RuntimeConfig{Mode: "unknown"},
		timeline.NewInMemoryService(nil),
		nil,
	)
	if err == nil {
		t.Fatal("expected unknown mode error")
	}
}

func TestBuildArchiveSinkDisabledModeReturnsNil(t *testing.T) {
	sink, err := buildArchiveSink(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		telemetryarchive.RuntimeConfig{Mode: telemetryarchive.ModeDisabled},
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if sink != nil {
		t.Fatal("expected nil sink in disabled mode")
	}
}

func TestBuildArchiveSinkLocalModeReturnsSink(t *testing.T) {
	sink, err := buildArchiveSink(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		telemetryarchive.RuntimeConfig{
			Mode:     telemetryarchive.ModeLocal,
			LocalDir: t.TempDir(),
		},
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if sink == nil {
		t.Fatal("expected non-nil sink in local mode")
	}
}

func TestTelemetrySignalFanoutForwardsToAllSinks(t *testing.T) {
	left := &fakeTelemetrySink{}
	right := &fakeTelemetrySink{}
	sink := newTelemetrySignalFanout(left, nil, right)
	if sink == nil {
		t.Fatal("expected non-nil fanout sink")
	}

	sink.AddLogRecords(cloudwatchlogs.LogRecord{EventID: "evt-1"})
	sink.AddMetricPoints(cloudwatch.MetricPoint{ResourceID: "i-123"})

	if len(left.logs) != 1 || len(right.logs) != 1 {
		t.Fatalf("expected log fanout to both sinks, got left=%d right=%d", len(left.logs), len(right.logs))
	}
	if len(left.metrics) != 1 || len(right.metrics) != 1 {
		t.Fatalf("expected metric fanout to both sinks, got left=%d right=%d", len(left.metrics), len(right.metrics))
	}
}

type fakeTelemetrySink struct {
	logs    []cloudwatchlogs.LogRecord
	metrics []cloudwatch.MetricPoint
}

func (f *fakeTelemetrySink) AddLogRecords(records ...cloudwatchlogs.LogRecord) {
	f.logs = append(f.logs, records...)
}

func (f *fakeTelemetrySink) AddMetricPoints(points ...cloudwatch.MetricPoint) {
	f.metrics = append(f.metrics, points...)
}
