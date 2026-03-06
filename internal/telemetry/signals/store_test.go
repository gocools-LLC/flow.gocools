package signals

import (
	"testing"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
)

func TestInMemoryStoreMetricRetentionAndFiltering(t *testing.T) {
	store := NewInMemoryStore(Config{
		MaxMetrics: 2,
		MaxLogs:    2,
	})

	base := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	store.AddMetricPoints(
		cloudwatch.MetricPoint{ResourceID: "i-1", MetricName: "CPUUtilization", Timestamp: base.Add(-2 * time.Minute), Value: 10},
		cloudwatch.MetricPoint{ResourceID: "i-1", MetricName: "CPUUtilization", Timestamp: base.Add(-1 * time.Minute), Value: 20},
		cloudwatch.MetricPoint{ResourceID: "i-1", MetricName: "CPUUtilization", Timestamp: base, Value: 30},
	)

	points := store.QueryMetricPoints(Query{
		Start: base.Add(-90 * time.Second),
		End:   base,
	})
	if len(points) != 2 {
		t.Fatalf("expected 2 retained points, got %d", len(points))
	}
	if points[0].Value != 20 || points[1].Value != 30 {
		t.Fatalf("unexpected retained metric values: %#v", points)
	}
}

func TestInMemoryStoreLogRetentionAndDefensiveCopy(t *testing.T) {
	store := NewInMemoryStore(Config{
		MaxMetrics: 2,
		MaxLogs:    2,
	})

	base := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	fields := map[string]string{
		"resource_id": "i-1",
	}

	store.AddLogRecords(
		cloudwatchlogs.LogRecord{EventID: "a", Timestamp: base.Add(-2 * time.Minute), Fields: fields},
		cloudwatchlogs.LogRecord{EventID: "b", Timestamp: base.Add(-1 * time.Minute), Fields: fields},
		cloudwatchlogs.LogRecord{EventID: "c", Timestamp: base, Fields: fields},
	)

	records := store.QueryLogRecords(Query{
		Start: base.Add(-90 * time.Second),
		End:   base,
	})
	if len(records) != 2 {
		t.Fatalf("expected 2 retained records, got %d", len(records))
	}
	if records[0].EventID != "b" || records[1].EventID != "c" {
		t.Fatalf("unexpected retained event ids: %#v", records)
	}

	records[0].Fields["resource_id"] = "mutated"
	recordsAgain := store.QueryLogRecords(Query{})
	if recordsAgain[0].Fields["resource_id"] == "mutated" {
		t.Fatal("expected defensive copy of log fields")
	}
}
