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
	)
	if err != nil {
		t.Fatalf("expected disabled mode to succeed, got %v", err)
	}
}

func TestStartTimelineIngestionCloudWatchModeRequiresRegion(t *testing.T) {
	err := startTimelineIngestion(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		internalaws.RuntimeConfig{},
		ingestion.RuntimeConfig{
			Mode:         ingestion.ModeCloudWatchLog,
			LogGroupName: "/aws/ecs/dev",
		},
		timeline.NewInMemoryService(nil),
	)
	if err == nil {
		t.Fatal("expected missing region error")
	}
	if !strings.Contains(err.Error(), "FLOW_AWS_REGION or AWS_REGION") {
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
	)
	if err == nil {
		t.Fatal("expected unknown mode error")
	}
}
