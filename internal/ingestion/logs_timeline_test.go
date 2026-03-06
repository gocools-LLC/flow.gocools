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
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
)

type fakeLogsCollector struct {
	responses [][]cloudwatchlogs.LogRecord
	err       error
	calls     int
	requests  []cloudwatchlogs.CollectRequest
}

func (f *fakeLogsCollector) Collect(_ context.Context, req cloudwatchlogs.CollectRequest) ([]cloudwatchlogs.LogRecord, error) {
	f.requests = append(f.requests, req)
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

	records := make([]cloudwatchlogs.LogRecord, len(f.responses[index]))
	copy(records, f.responses[index])
	return records, nil
}

type fakeTimeline struct {
	events []timeline.Event
}

func (f *fakeTimeline) AddEvents(events ...timeline.Event) {
	f.events = append(f.events, events...)
}

type fakeLogRecordSink struct {
	records []cloudwatchlogs.LogRecord
}

func (f *fakeLogRecordSink) AddLogRecords(records ...cloudwatchlogs.LogRecord) {
	f.records = append(f.records, records...)
}

func TestLogsTimelineIngestorIngestOnceDedupesEventIDsAcrossPolls(t *testing.T) {
	baseTime := time.Date(2026, 3, 6, 8, 0, 0, 0, time.UTC)
	collector := &fakeLogsCollector{
		responses: [][]cloudwatchlogs.LogRecord{
			{
				{
					LogGroupName:  "/aws/ecs/dev",
					LogStreamName: "service-a/1",
					EventID:       "event-1",
					Timestamp:     baseTime.Add(-2 * time.Second),
					Message:       "{\"level\":\"error\",\"message\":\"request failed\"}",
					Level:         "error",
					CorrelationID: "req-1",
				},
				{
					LogGroupName:  "/aws/ecs/dev",
					LogStreamName: "service-a/1",
					Timestamp:     baseTime.Add(-1 * time.Second),
					Message:       "warning: retrying request",
					Level:         "warning",
				},
			},
			{
				{
					LogGroupName:  "/aws/ecs/dev",
					LogStreamName: "service-a/1",
					EventID:       "event-1",
					Timestamp:     baseTime.Add(-2 * time.Second),
					Message:       "{\"level\":\"error\",\"message\":\"request failed\"}",
					Level:         "error",
					CorrelationID: "req-1",
				},
				{
					LogGroupName:  "/aws/ecs/dev",
					LogStreamName: "service-b/1",
					EventID:       "event-2",
					Timestamp:     baseTime,
					Message:       "panic: worker crashed",
				},
			},
		},
	}
	timelineSink := &fakeTimeline{}
	logSink := &fakeLogRecordSink{}

	ingestor := NewLogsTimelineIngestor(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		RuntimeConfig{
			Mode:          ModeCloudWatchLog,
			LogGroupName:  "/aws/ecs/dev",
			FilterPattern: "",
			CollectWindow: 5 * time.Minute,
		},
		collector,
		timelineSink,
	).WithLogRecordSink(logSink)

	nowCalls := 0
	ingestor.now = func() time.Time {
		nowCalls++
		return baseTime.Add(time.Duration(nowCalls) * time.Second)
	}

	if err := ingestor.ingestOnce(context.Background()); err != nil {
		t.Fatalf("first ingestion failed: %v", err)
	}
	if err := ingestor.ingestOnce(context.Background()); err != nil {
		t.Fatalf("second ingestion failed: %v", err)
	}

	if len(timelineSink.events) != 3 {
		t.Fatalf("expected 3 timeline events after dedupe, got %d", len(timelineSink.events))
	}
	if len(logSink.records) != 3 {
		t.Fatalf("expected 3 stored log records after dedupe, got %d", len(logSink.records))
	}
	if timelineSink.events[0].ID != "event-1" {
		t.Fatalf("expected first event id event-1, got %s", timelineSink.events[0].ID)
	}
	if timelineSink.events[0].Severity != timeline.SeverityError {
		t.Fatalf("expected first event severity error, got %s", timelineSink.events[0].Severity)
	}
	if timelineSink.events[1].Severity != timeline.SeverityWarning {
		t.Fatalf("expected second event severity warning, got %s", timelineSink.events[1].Severity)
	}
	if timelineSink.events[2].Severity != timeline.SeverityCritical {
		t.Fatalf("expected third event severity critical, got %s", timelineSink.events[2].Severity)
	}

	if len(collector.requests) != 2 {
		t.Fatalf("expected 2 collector requests, got %d", len(collector.requests))
	}
	if !collector.requests[1].StartTime.Equal(baseTime.Add(1 * time.Second).UTC()) {
		t.Fatalf("expected second start time to match previous end time, got %s", collector.requests[1].StartTime.UTC().Format(time.RFC3339))
	}
}

func TestLogsTimelineIngestorIngestOnceReturnsCollectorError(t *testing.T) {
	collector := &fakeLogsCollector{
		err: errors.New("boom"),
	}

	ingestor := NewLogsTimelineIngestor(
		nil,
		RuntimeConfig{
			Mode:         ModeCloudWatchLog,
			LogGroupName: "/aws/ecs/dev",
		},
		collector,
		&fakeTimeline{},
	)

	if err := ingestor.ingestOnce(context.Background()); err == nil {
		t.Fatal("expected ingestion error")
	}
}

func TestEventIDFallsBackWhenEventIDMissing(t *testing.T) {
	record := cloudwatchlogs.LogRecord{
		LogGroupName:  "/aws/ecs/dev",
		LogStreamName: "service-a/1",
		Timestamp:     time.Date(2026, 3, 6, 8, 0, 0, 0, time.UTC),
		Message:       "request failed",
	}

	id := eventID(record)
	if id == "" {
		t.Fatal("expected synthetic event id")
	}
	if !strings.HasPrefix(id, "cwlog-") {
		t.Fatalf("expected synthetic cwlog prefix, got %q", id)
	}
}
