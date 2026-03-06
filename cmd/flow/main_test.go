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
