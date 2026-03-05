package integration

import (
	"context"
	"testing"
	"time"

	cwapi "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	cwlogsapi "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlogstypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/gocools-LLC/flow.gocools/internal/graph"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
)

func TestTelemetryIngestionHarness(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)

	metricCollector := cloudwatch.NewCollector(&fakeMetricClient{
		output: &cwapi.GetMetricDataOutput{
			MetricDataResults: []cwtypes.MetricDataResult{
				{
					Id:         awsString("m1"),
					Timestamps: []time.Time{baseTime},
					Values:     []float64{92.1},
				},
			},
		},
	}, cloudwatch.CollectorConfig{})

	logCollector := cloudwatchlogs.NewCollector(&fakeLogClient{
		output: &cwlogsapi.FilterLogEventsOutput{
			Events: []cwlogstypes.FilteredLogEvent{
				{
					EventId:       awsString("event-1"),
					LogStreamName: awsString("service-stream"),
					Message:       awsString(`{"level":"error","resource_id":"i-123","correlation_id":"req-1","msg":"timeout"}`),
					Timestamp:     awsInt64(baseTime.Add(30 * time.Second).UnixMilli()),
				},
			},
		},
	}, cloudwatchlogs.CollectorConfig{
		Hooks: []cloudwatchlogs.ParseHook{cloudwatchlogs.CorrelationIDRegexHook()},
	})

	metrics, err := metricCollector.Collect(ctx, []cloudwatch.ResourceTarget{
		{
			Kind:       cloudwatch.ResourceKindEC2,
			InstanceID: "i-123",
		},
	}, 5*time.Minute)
	if err != nil {
		t.Fatalf("metrics collect failed: %v", err)
	}
	if len(metrics) == 0 {
		t.Fatalf("expected metrics from harness")
	}

	logs, err := logCollector.Collect(ctx, cloudwatchlogs.CollectRequest{
		LogGroupName: "/ecs/service-a",
		StartTime:    baseTime.Add(-5 * time.Minute),
		EndTime:      baseTime.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("logs collect failed: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected logs from harness")
	}

	correlationGraph := graph.BuildCorrelationGraph(metrics, logs, graph.CorrelationBuildConfig{
		MaxSkew: 2 * time.Minute,
	})

	edges := correlationGraph.Edges()
	t.Logf("harness produced %d metrics, %d logs, %d edges", len(metrics), len(logs), len(edges))

	correlatedEdgeCount := 0
	for _, edge := range edges {
		if edge.Kind == graph.EdgeKindCorrelatedWith {
			correlatedEdgeCount++
		}
	}

	if correlatedEdgeCount == 0 {
		t.Logf("metrics: %+v", metrics)
		t.Logf("logs: %+v", logs)
		t.Logf("edges: %+v", edges)
		t.Fatal("expected at least one correlated edge from ingestion harness")
	}
}

type fakeMetricClient struct {
	output *cwapi.GetMetricDataOutput
}

func (f *fakeMetricClient) GetMetricData(_ context.Context, _ *cwapi.GetMetricDataInput, _ ...func(*cwapi.Options)) (*cwapi.GetMetricDataOutput, error) {
	return f.output, nil
}

type fakeLogClient struct {
	output *cwlogsapi.FilterLogEventsOutput
}

func (f *fakeLogClient) FilterLogEvents(_ context.Context, _ *cwlogsapi.FilterLogEventsInput, _ ...func(*cwlogsapi.Options)) (*cwlogsapi.FilterLogEventsOutput, error) {
	return f.output, nil
}

func awsString(v string) *string {
	return &v
}

func awsInt64(v int64) *int64 {
	return &v
}
