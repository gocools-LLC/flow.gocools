package ingestion

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/analyzer/timeline"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
)

type fakeMetricsCollector struct {
	responses [][]cloudwatch.MetricPoint
	err       error
	calls     int
}

func (f *fakeMetricsCollector) Collect(_ context.Context, _ []cloudwatch.ResourceTarget, _ time.Duration) ([]cloudwatch.MetricPoint, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.responses) == 0 {
		return nil, nil
	}

	index := f.calls
	if index >= len(f.responses) {
		index = len(f.responses) - 1
	}
	f.calls++

	points := make([]cloudwatch.MetricPoint, len(f.responses[index]))
	copy(points, f.responses[index])
	return points, nil
}

type fakeMetricPointSink struct {
	points []cloudwatch.MetricPoint
}

func (f *fakeMetricPointSink) AddMetricPoints(points ...cloudwatch.MetricPoint) {
	f.points = append(f.points, points...)
}

func TestMetricsTimelineIngestorIngestOnceAddsOnlyThresholdBreachesAndDedupes(t *testing.T) {
	pointTS := time.Date(2026, 3, 6, 10, 0, 0, 0, time.UTC)
	collector := &fakeMetricsCollector{
		responses: [][]cloudwatch.MetricPoint{
			{
				{
					ResourceID: "i-123",
					Namespace:  "AWS/EC2",
					MetricName: "CPUUtilization",
					Timestamp:  pointTS,
					Value:      45.0,
				},
				{
					ResourceID: "i-123",
					Namespace:  "AWS/EC2",
					MetricName: "CPUUtilization",
					Timestamp:  pointTS,
					Value:      72.3,
				},
				{
					ResourceID: "cluster-a/service-a",
					Namespace:  "AWS/ECS",
					MetricName: "MemoryUtilization",
					Timestamp:  pointTS,
					Value:      93.1,
				},
			},
			{
				{
					ResourceID: "i-123",
					Namespace:  "AWS/EC2",
					MetricName: "CPUUtilization",
					Timestamp:  pointTS,
					Value:      72.3,
				},
			},
		},
	}

	timelineSink := &fakeTimeline{}
	metricSink := &fakeMetricPointSink{}
	targets := []cloudwatch.ResourceTarget{
		{Kind: cloudwatch.ResourceKindEC2, InstanceID: "i-123"},
	}

	ingestor := NewMetricsTimelineIngestor(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		RuntimeConfig{
			Mode:             ModeCloudWatchMetric,
			CollectWindow:    5 * time.Minute,
			UtilizationWarn:  70,
			UtilizationError: 90,
		},
		targets,
		collector,
		timelineSink,
	).WithMetricPointSink(metricSink)

	if err := ingestor.ingestOnce(context.Background()); err != nil {
		t.Fatalf("first metrics ingestion failed: %v", err)
	}
	if err := ingestor.ingestOnce(context.Background()); err != nil {
		t.Fatalf("second metrics ingestion failed: %v", err)
	}

	if len(timelineSink.events) != 2 {
		t.Fatalf("expected 2 timeline events after dedupe/thresholds, got %d", len(timelineSink.events))
	}
	if len(metricSink.points) != 2 {
		t.Fatalf("expected 2 stored metric points after dedupe/thresholds, got %d", len(metricSink.points))
	}
	if timelineSink.events[0].Severity != timeline.SeverityWarning {
		t.Fatalf("expected warning severity, got %s", timelineSink.events[0].Severity)
	}
	if timelineSink.events[1].Severity != timeline.SeverityError {
		t.Fatalf("expected error severity, got %s", timelineSink.events[1].Severity)
	}
	if !strings.HasPrefix(timelineSink.events[0].ID, "cwmetric-") {
		t.Fatalf("expected metric event id prefix, got %q", timelineSink.events[0].ID)
	}
}

func TestMetricsTimelineIngestorIngestOnceReturnsCollectorError(t *testing.T) {
	collector := &fakeMetricsCollector{
		err: errors.New("boom"),
	}

	ingestor := NewMetricsTimelineIngestor(
		nil,
		RuntimeConfig{Mode: ModeCloudWatchMetric},
		[]cloudwatch.ResourceTarget{{Kind: cloudwatch.ResourceKindEC2, InstanceID: "i-123"}},
		collector,
		&fakeTimeline{},
	)

	if err := ingestor.ingestOnce(context.Background()); err == nil {
		t.Fatal("expected ingestion error")
	}
}

func TestMetricSeverity(t *testing.T) {
	severity, emit := metricSeverity(cloudwatch.MetricPoint{MetricName: "CPUUtilization", Value: 95}, 70, 90)
	if !emit || severity != timeline.SeverityError {
		t.Fatalf("expected error emit for 95 CPU, got emit=%v severity=%s", emit, severity)
	}

	severity, emit = metricSeverity(cloudwatch.MetricPoint{MetricName: "MemoryUtilization", Value: 75}, 70, 90)
	if !emit || severity != timeline.SeverityWarning {
		t.Fatalf("expected warning emit for 75 Memory, got emit=%v severity=%s", emit, severity)
	}

	_, emit = metricSeverity(cloudwatch.MetricPoint{MetricName: "NetworkIn", Value: 100}, 70, 90)
	if emit {
		t.Fatal("expected non-utilization metric to be ignored")
	}
}
