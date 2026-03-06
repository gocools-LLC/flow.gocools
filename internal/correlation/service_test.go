package correlation

import (
	"strings"
	"testing"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/signals"
)

type fakeStore struct {
	metrics []cloudwatch.MetricPoint
	logs    []cloudwatchlogs.LogRecord
}

func (f fakeStore) QueryMetricPoints(query signals.Query) []cloudwatch.MetricPoint {
	points := make([]cloudwatch.MetricPoint, 0, len(f.metrics))
	for _, point := range f.metrics {
		if !query.Start.IsZero() && point.Timestamp.Before(query.Start) {
			continue
		}
		if !query.End.IsZero() && point.Timestamp.After(query.End) {
			continue
		}
		points = append(points, point)
	}
	return points
}

func (f fakeStore) QueryLogRecords(query signals.Query) []cloudwatchlogs.LogRecord {
	records := make([]cloudwatchlogs.LogRecord, 0, len(f.logs))
	for _, record := range f.logs {
		if !query.Start.IsZero() && record.Timestamp.Before(query.Start) {
			continue
		}
		if !query.End.IsZero() && record.Timestamp.After(query.End) {
			continue
		}
		records = append(records, record)
	}
	return records
}

func TestQueryGraphBuildsCorrelatedEdges(t *testing.T) {
	now := time.Date(2026, 3, 6, 15, 0, 0, 0, time.UTC)
	service := NewService(fakeStore{
		metrics: []cloudwatch.MetricPoint{
			{
				ResourceID: "i-123",
				Namespace:  "AWS/EC2",
				MetricName: "CPUUtilization",
				Timestamp:  now,
				Value:      91,
			},
		},
		logs: []cloudwatchlogs.LogRecord{
			{
				EventID:   "evt-1",
				Timestamp: now.Add(30 * time.Second),
				Fields: map[string]string{
					"resource_id": "i-123",
				},
			},
		},
	})

	result, err := service.QueryGraph(Query{
		Start:   now.Add(-1 * time.Minute),
		End:     now.Add(1 * time.Minute),
		MaxSkew: 2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("query graph failed: %v", err)
	}
	if result.MetricCount != 1 || result.LogCount != 1 {
		t.Fatalf("unexpected signal counts: metrics=%d logs=%d", result.MetricCount, result.LogCount)
	}
	if len(result.Nodes) == 0 {
		t.Fatal("expected non-empty graph nodes")
	}

	correlated := 0
	for _, edge := range result.Edges {
		if string(edge.Kind) == "correlated_with" {
			correlated++
		}
	}
	if correlated == 0 {
		t.Fatalf("expected correlated edges, got %#v", result.Edges)
	}
}

func TestQueryGraphRejectsInvalidRange(t *testing.T) {
	service := NewService(fakeStore{})
	_, err := service.QueryGraph(Query{
		Start: time.Date(2026, 3, 6, 16, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 3, 6, 15, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected invalid range error")
	}
}

func TestQueryGraphRejectsNegativeLimits(t *testing.T) {
	service := NewService(fakeStore{})

	_, err := service.QueryGraph(Query{LimitNodes: -1})
	if err == nil || !strings.Contains(err.Error(), "limit_nodes") {
		t.Fatalf("expected limit_nodes validation error, got %v", err)
	}

	_, err = service.QueryGraph(Query{LimitEdges: -1})
	if err == nil || !strings.Contains(err.Error(), "limit_edges") {
		t.Fatalf("expected limit_edges validation error, got %v", err)
	}
}

func TestQueryGraphNilStoreReturnsEmpty(t *testing.T) {
	service := NewService(nil)
	result, err := service.QueryGraph(Query{})
	if err != nil {
		t.Fatalf("query graph failed: %v", err)
	}
	if len(result.Nodes) != 0 || len(result.Edges) != 0 {
		t.Fatalf("expected empty graph, got nodes=%d edges=%d", len(result.Nodes), len(result.Edges))
	}
}

func TestQueryGraphFiltersByResourceID(t *testing.T) {
	now := time.Date(2026, 3, 6, 15, 0, 0, 0, time.UTC)
	service := NewService(fakeStore{
		metrics: []cloudwatch.MetricPoint{
			{
				ResourceID: "i-123",
				Namespace:  "AWS/EC2",
				MetricName: "CPUUtilization",
				Timestamp:  now,
				Value:      80,
			},
			{
				ResourceID: "i-999",
				Namespace:  "AWS/EC2",
				MetricName: "CPUUtilization",
				Timestamp:  now,
				Value:      90,
			},
		},
		logs: []cloudwatchlogs.LogRecord{
			{
				EventID:   "evt-1",
				Timestamp: now,
				Fields: map[string]string{
					"resource_id": "i-123",
				},
			},
			{
				EventID:   "evt-2",
				Timestamp: now,
				Fields: map[string]string{
					"resource_id": "i-999",
				},
			},
		},
	})

	result, err := service.QueryGraph(Query{
		ResourceID: "i-123",
	})
	if err != nil {
		t.Fatalf("query graph failed: %v", err)
	}
	if len(result.Nodes) == 0 {
		t.Fatal("expected filtered graph nodes")
	}
	for _, node := range result.Nodes {
		if node.Attributes["resource_id"] != "i-123" {
			t.Fatalf("unexpected node outside filter: %#v", node)
		}
	}
	for _, edge := range result.Edges {
		if strings.Contains(edge.From, "i-999") || strings.Contains(edge.To, "i-999") {
			t.Fatalf("unexpected edge outside resource filter: %#v", edge)
		}
	}
}

func TestQueryGraphAppliesNodeAndEdgeLimits(t *testing.T) {
	now := time.Date(2026, 3, 6, 15, 0, 0, 0, time.UTC)
	service := NewService(fakeStore{
		metrics: []cloudwatch.MetricPoint{
			{
				ResourceID: "i-123",
				Namespace:  "AWS/EC2",
				MetricName: "CPUUtilization",
				Timestamp:  now,
				Value:      91,
			},
			{
				ResourceID: "i-123",
				Namespace:  "AWS/EC2",
				MetricName: "CPUUtilization",
				Timestamp:  now.Add(30 * time.Second),
				Value:      95,
			},
		},
		logs: []cloudwatchlogs.LogRecord{
			{
				EventID:   "evt-1",
				Timestamp: now,
				Fields: map[string]string{
					"resource_id": "i-123",
				},
			},
			{
				EventID:   "evt-2",
				Timestamp: now.Add(30 * time.Second),
				Fields: map[string]string{
					"resource_id": "i-123",
				},
			},
		},
	})

	result, err := service.QueryGraph(Query{
		LimitNodes: 2,
		LimitEdges: 1,
	})
	if err != nil {
		t.Fatalf("query graph failed: %v", err)
	}
	if len(result.Nodes) > 2 {
		t.Fatalf("expected at most 2 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) > 1 {
		t.Fatalf("expected at most 1 edge, got %d", len(result.Edges))
	}
}
