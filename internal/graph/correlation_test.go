package graph

import (
	"testing"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
)

func TestBuildCorrelationGraphRules(t *testing.T) {
	metrics := []cloudwatch.MetricPoint{
		{
			ResourceID: "i-123",
			Namespace:  "AWS/EC2",
			MetricName: "CPUUtilization",
			Timestamp:  time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
			Value:      80.2,
		},
	}

	logs := []cloudwatchlogs.LogRecord{
		{
			LogGroupName:  "app-group",
			LogStreamName: "stream-a",
			EventID:       "event-near",
			Timestamp:     time.Date(2026, 3, 5, 0, 1, 0, 0, time.UTC),
			Fields: map[string]string{
				"resource_id": "i-123",
			},
		},
		{
			LogGroupName:  "app-group",
			LogStreamName: "stream-a",
			EventID:       "event-far",
			Timestamp:     time.Date(2026, 3, 5, 0, 10, 0, 0, time.UTC),
			Fields: map[string]string{
				"resource_id": "i-123",
			},
		},
	}

	graph := BuildCorrelationGraph(metrics, logs, CorrelationBuildConfig{
		MaxSkew: 2 * time.Minute,
	})

	resourceNodeID := "resource:i-123"
	if _, ok := graph.Node(resourceNodeID); !ok {
		t.Fatalf("expected resource node %q", resourceNodeID)
	}

	correlatedEdges := 0
	for _, edge := range graph.Edges() {
		if edge.Kind == EdgeKindCorrelatedWith {
			correlatedEdges++
		}
	}
	if correlatedEdges != 1 {
		t.Fatalf("expected 1 correlated edge, got %d", correlatedEdges)
	}

	hasNearCorrelation := false
	hasFarCorrelation := false
	for _, edge := range graph.Edges() {
		if edge.Kind == EdgeKindCorrelatedWith && edge.To == "log:event-near" {
			hasNearCorrelation = true
		}
		if edge.Kind == EdgeKindCorrelatedWith && edge.To == "log:event-far" {
			hasFarCorrelation = true
		}
	}
	if !hasNearCorrelation {
		t.Fatal("expected metric to correlate with near log")
	}
	if hasFarCorrelation {
		t.Fatal("did not expect metric to correlate with far log")
	}
}

func TestGraphRelationQueries(t *testing.T) {
	graph := NewGraph()
	graph.AddNode(Node{ID: "resource:i-1", Kind: NodeKindResource})
	graph.AddNode(Node{ID: "metric:i-1:cpu:1:0", Kind: NodeKindMetric})
	graph.AddNode(Node{ID: "log:event-1", Kind: NodeKindLog})

	graph.AddEdge(Edge{
		From: "resource:i-1",
		To:   "metric:i-1:cpu:1:0",
		Kind: EdgeKindEmitsMetric,
	})
	graph.AddEdge(Edge{
		From: "resource:i-1",
		To:   "log:event-1",
		Kind: EdgeKindEmitsLog,
	})

	neighbors := graph.Neighbors("resource:i-1")
	if len(neighbors) != 2 {
		t.Fatalf("expected 2 neighbors, got %d", len(neighbors))
	}

	metricNeighbors := graph.RelatedByEdgeKind("resource:i-1", EdgeKindEmitsMetric)
	if len(metricNeighbors) != 1 {
		t.Fatalf("expected 1 metric-related node, got %d", len(metricNeighbors))
	}
	if metricNeighbors[0].Kind != NodeKindMetric {
		t.Fatalf("expected metric node kind, got %s", metricNeighbors[0].Kind)
	}

	edges := graph.EdgesForNode("resource:i-1")
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges for node, got %d", len(edges))
	}
}
