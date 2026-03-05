package cloudwatch

import (
	"context"
	"testing"
	"time"

	cw "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/smithy-go"
)

func TestBuildMetricQueries_EC2AndECS(t *testing.T) {
	targets := []ResourceTarget{
		{
			Kind:       ResourceKindEC2,
			InstanceID: "i-123",
		},
		{
			Kind:        ResourceKindECSService,
			ClusterName: "cluster-a",
			ServiceName: "svc-a",
		},
	}

	queries, descriptors := buildMetricQueries(targets)

	if len(queries) != 5 {
		t.Fatalf("expected 5 queries, got %d", len(queries))
	}

	if len(descriptors) != 5 {
		t.Fatalf("expected 5 descriptors, got %d", len(descriptors))
	}

	first := queries[0]
	if first.MetricStat == nil || first.MetricStat.Metric == nil || first.MetricStat.Metric.Namespace == nil {
		t.Fatal("first query metric is nil")
	}
	if got := *first.MetricStat.Metric.Namespace; got != "AWS/EC2" {
		t.Fatalf("expected AWS/EC2 namespace, got %q", got)
	}
	if got := *first.MetricStat.Metric.MetricName; got != "CPUUtilization" {
		t.Fatalf("expected CPUUtilization metric, got %q", got)
	}
}

func TestCollect_RetriesOnThrottling(t *testing.T) {
	client := &fakeClient{
		output: &cw.GetMetricDataOutput{
			MetricDataResults: []types.MetricDataResult{
				{
					Id:         awsString("m1"),
					Values:     []float64{42.5},
					Timestamps: []time.Time{time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)},
				},
			},
		},
		failuresBeforeSuccess: 1,
	}

	collector := NewCollector(client, CollectorConfig{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
	})
	collector.sleep = func(time.Duration) {}
	collector.now = func() time.Time { return time.Date(2026, 3, 5, 0, 5, 0, 0, time.UTC) }

	points, err := collector.Collect(context.Background(), []ResourceTarget{
		{
			Kind:       ResourceKindEC2,
			InstanceID: "i-123",
		},
	}, 5*time.Minute)
	if err != nil {
		t.Fatalf("collect returned error: %v", err)
	}

	if client.calls != 2 {
		t.Fatalf("expected 2 API calls due to retry, got %d", client.calls)
	}

	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	if points[0].ResourceID != "i-123" {
		t.Fatalf("expected resource i-123, got %q", points[0].ResourceID)
	}
}

func TestMetricPointsFromOutput(t *testing.T) {
	output := &cw.GetMetricDataOutput{
		MetricDataResults: []types.MetricDataResult{
			{
				Id:         awsString("m1"),
				Values:     []float64{1.5, 2.5},
				Timestamps: []time.Time{time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 5, 0, 1, 0, 0, time.UTC)},
			},
		},
	}

	descriptors := map[string]queryDescriptor{
		"m1": {
			ResourceID: "cluster-a/svc-a",
			Namespace:  "AWS/ECS",
			MetricName: "CPUUtilization",
		},
	}

	points := metricPointsFromOutput(output, descriptors)
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}

	if points[0].Namespace != "AWS/ECS" {
		t.Fatalf("expected namespace AWS/ECS, got %q", points[0].Namespace)
	}
	if points[0].MetricName != "CPUUtilization" {
		t.Fatalf("expected metric CPUUtilization, got %q", points[0].MetricName)
	}
}

type fakeClient struct {
	output                *cw.GetMetricDataOutput
	calls                 int
	failuresBeforeSuccess int
}

func (f *fakeClient) GetMetricData(_ context.Context, _ *cw.GetMetricDataInput, _ ...func(*cw.Options)) (*cw.GetMetricDataOutput, error) {
	f.calls++
	if f.calls <= f.failuresBeforeSuccess {
		return nil, &smithy.GenericAPIError{
			Code:    "ThrottlingException",
			Message: "rate exceeded",
		}
	}
	return f.output, nil
}
