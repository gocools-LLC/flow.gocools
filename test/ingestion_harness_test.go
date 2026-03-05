package integration

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
	fixture := newHarnessFixture(42, time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC))

	metricClient := &fakeMetricClient{
		output: fixture.metricOutput,
	}
	metricCollector := cloudwatch.NewCollector(metricClient, cloudwatch.CollectorConfig{
		Now:   func() time.Time { return fixture.metricWindowEnd },
		Sleep: func(time.Duration) {},
	})

	logClient := &fakeLogClient{
		output: fixture.logOutput,
	}
	logCollector := cloudwatchlogs.NewCollector(logClient, cloudwatchlogs.CollectorConfig{
		Hooks: []cloudwatchlogs.ParseHook{cloudwatchlogs.CorrelationIDRegexHook()},
		Now:   func() time.Time { return fixture.logWindowEnd },
		Sleep: func(time.Duration) {},
	})

	metrics, err := metricCollector.Collect(ctx, []cloudwatch.ResourceTarget{
		{
			Kind:       cloudwatch.ResourceKindEC2,
			InstanceID: fixture.resourceID,
		},
	}, fixture.metricWindow)
	if err != nil {
		t.Fatalf("metrics collect failed: %v\n%s", err, harnessDiagnostics(nil, nil, nil, metricClient, logClient))
	}
	if len(metrics) == 0 {
		t.Fatalf("expected metrics from harness\n%s", harnessDiagnostics(metrics, nil, nil, metricClient, logClient))
	}

	if metricClient.lastInput == nil {
		t.Fatalf("expected metric collector request input\n%s", harnessDiagnostics(metrics, nil, nil, metricClient, logClient))
	}
	expectedMetricStart := fixture.metricWindowEnd.Add(-fixture.metricWindow)
	if metricClient.lastInput.StartTime == nil || !metricClient.lastInput.StartTime.UTC().Equal(expectedMetricStart) {
		t.Fatalf("unexpected metric start window: got=%v expected=%s\n%s", metricClient.lastInput.StartTime, expectedMetricStart.Format(time.RFC3339), harnessDiagnostics(metrics, nil, nil, metricClient, logClient))
	}
	if metricClient.lastInput.EndTime == nil || !metricClient.lastInput.EndTime.UTC().Equal(fixture.metricWindowEnd) {
		t.Fatalf("unexpected metric end window: got=%v expected=%s\n%s", metricClient.lastInput.EndTime, fixture.metricWindowEnd.Format(time.RFC3339), harnessDiagnostics(metrics, nil, nil, metricClient, logClient))
	}

	logs, err := logCollector.Collect(ctx, cloudwatchlogs.CollectRequest{
		LogGroupName: fixture.logGroupName,
		StartTime:    fixture.logWindowStart,
		EndTime:      fixture.logWindowEnd,
	})
	if err != nil {
		t.Fatalf("logs collect failed: %v\n%s", err, harnessDiagnostics(metrics, nil, nil, metricClient, logClient))
	}
	if len(logs) == 0 {
		t.Fatalf("expected logs from harness\n%s", harnessDiagnostics(metrics, logs, nil, metricClient, logClient))
	}

	if logClient.lastInput == nil {
		t.Fatalf("expected log collector request input\n%s", harnessDiagnostics(metrics, logs, nil, metricClient, logClient))
	}
	if logClient.lastInput.StartTime == nil || *logClient.lastInput.StartTime != fixture.logWindowStart.UnixMilli() {
		t.Fatalf("unexpected log start window: got=%v expected=%d\n%s", logClient.lastInput.StartTime, fixture.logWindowStart.UnixMilli(), harnessDiagnostics(metrics, logs, nil, metricClient, logClient))
	}
	if logClient.lastInput.EndTime == nil || *logClient.lastInput.EndTime != fixture.logWindowEnd.UnixMilli() {
		t.Fatalf("unexpected log end window: got=%v expected=%d\n%s", logClient.lastInput.EndTime, fixture.logWindowEnd.UnixMilli(), harnessDiagnostics(metrics, logs, nil, metricClient, logClient))
	}

	stableSortMetricPoints(metrics)
	stableSortLogRecords(logs)

	correlationGraph := graph.BuildCorrelationGraph(metrics, logs, graph.CorrelationBuildConfig{
		MaxSkew: 2 * time.Minute,
	})
	edges := correlationGraph.Edges()
	stableSortEdges(edges)

	correlatedEdgeCount := 0
	for _, edge := range edges {
		if edge.Kind == graph.EdgeKindCorrelatedWith {
			correlatedEdgeCount++
		}
	}

	if correlatedEdgeCount == 0 {
		t.Fatalf("expected at least one correlated edge from ingestion harness\n%s", harnessDiagnostics(metrics, logs, edges, metricClient, logClient))
	}
}

func TestHarnessFixtureDeterministicOrder(t *testing.T) {
	baseTime := time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
	fixtureA := newHarnessFixture(42, baseTime)
	fixtureB := newHarnessFixture(42, baseTime)

	metricIDsA := collectMetricIDs(fixtureA.metricOutput.MetricDataResults)
	metricIDsB := collectMetricIDs(fixtureB.metricOutput.MetricDataResults)
	if strings.Join(metricIDsA, ",") != strings.Join(metricIDsB, ",") {
		t.Fatalf("expected stable metric fixture ordering, got A=%v B=%v", metricIDsA, metricIDsB)
	}

	eventIDsA := collectLogEventIDs(fixtureA.logOutput.Events)
	eventIDsB := collectLogEventIDs(fixtureB.logOutput.Events)
	if strings.Join(eventIDsA, ",") != strings.Join(eventIDsB, ",") {
		t.Fatalf("expected stable log fixture ordering, got A=%v B=%v", eventIDsA, eventIDsB)
	}
}

type harnessFixture struct {
	seed            int64
	resourceID      string
	logGroupName    string
	metricWindow    time.Duration
	metricWindowEnd time.Time
	logWindowStart  time.Time
	logWindowEnd    time.Time
	metricOutput    *cwapi.GetMetricDataOutput
	logOutput       *cwlogsapi.FilterLogEventsOutput
}

func newHarnessFixture(seed int64, baseTime time.Time) harnessFixture {
	metricResults := []cwtypes.MetricDataResult{
		{
			Id:         awsString("m2"),
			Timestamps: []time.Time{baseTime.Add(30 * time.Second)},
			Values:     []float64{87.3},
		},
		{
			Id:         awsString("m1"),
			Timestamps: []time.Time{baseTime},
			Values:     []float64{92.1},
		},
	}
	sort.Slice(metricResults, func(i, j int) bool {
		return derefString(metricResults[i].Id) < derefString(metricResults[j].Id)
	})

	logEvents := []cwlogstypes.FilteredLogEvent{
		{
			EventId:       awsString("event-b"),
			LogStreamName: awsString("service-stream"),
			Message:       awsString(`{"level":"error","resource_id":"i-123","correlation_id":"req-1","msg":"timeout"}`),
			Timestamp:     awsInt64(baseTime.Add(30 * time.Second).UnixMilli()),
		},
		{
			EventId:       awsString("event-a"),
			LogStreamName: awsString("service-stream"),
			Message:       awsString(`{"level":"warn","resource_id":"i-123","correlation_id":"req-2","msg":"slow"}`),
			Timestamp:     awsInt64(baseTime.Add(45 * time.Second).UnixMilli()),
		},
	}
	sort.Slice(logEvents, func(i, j int) bool {
		return derefString(logEvents[i].EventId) < derefString(logEvents[j].EventId)
	})

	return harnessFixture{
		seed:            seed,
		resourceID:      "i-123",
		logGroupName:    "/ecs/service-a",
		metricWindow:    5 * time.Minute,
		metricWindowEnd: baseTime.Add(5 * time.Minute),
		logWindowStart:  baseTime.Add(-5 * time.Minute),
		logWindowEnd:    baseTime.Add(5 * time.Minute),
		metricOutput: &cwapi.GetMetricDataOutput{
			MetricDataResults: metricResults,
		},
		logOutput: &cwlogsapi.FilterLogEventsOutput{
			Events: logEvents,
		},
	}
}

func stableSortMetricPoints(points []cloudwatch.MetricPoint) {
	sort.Slice(points, func(i, j int) bool {
		if points[i].Timestamp.Equal(points[j].Timestamp) {
			if points[i].ResourceID == points[j].ResourceID {
				return points[i].MetricName < points[j].MetricName
			}
			return points[i].ResourceID < points[j].ResourceID
		}
		return points[i].Timestamp.Before(points[j].Timestamp)
	})
}

func stableSortLogRecords(records []cloudwatchlogs.LogRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].Timestamp.Equal(records[j].Timestamp) {
			return records[i].EventID < records[j].EventID
		}
		return records[i].Timestamp.Before(records[j].Timestamp)
	})
}

func stableSortEdges(edges []graph.Edge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			if edges[i].To == edges[j].To {
				return edges[i].Kind < edges[j].Kind
			}
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})
}

func harnessDiagnostics(metrics []cloudwatch.MetricPoint, logs []cloudwatchlogs.LogRecord, edges []graph.Edge, metricClient *fakeMetricClient, logClient *fakeLogClient) string {
	builder := strings.Builder{}
	builder.WriteString("harness diagnostics:\n")
	builder.WriteString(fmt.Sprintf("metric_calls=%d log_calls=%d\n", metricClient.calls, logClient.calls))

	if metricClient.lastInput != nil && metricClient.lastInput.StartTime != nil && metricClient.lastInput.EndTime != nil {
		builder.WriteString(fmt.Sprintf("metric_window=%s..%s query_count=%d\n",
			metricClient.lastInput.StartTime.UTC().Format(time.RFC3339),
			metricClient.lastInput.EndTime.UTC().Format(time.RFC3339),
			len(metricClient.lastInput.MetricDataQueries),
		))
	}
	if logClient.lastInput != nil && logClient.lastInput.StartTime != nil && logClient.lastInput.EndTime != nil {
		builder.WriteString(fmt.Sprintf("log_window=%d..%d log_group=%s\n",
			*logClient.lastInput.StartTime,
			*logClient.lastInput.EndTime,
			derefString(logClient.lastInput.LogGroupName),
		))
	}

	builder.WriteString(fmt.Sprintf("metrics=%d logs=%d edges=%d\n", len(metrics), len(logs), len(edges)))
	if len(metrics) > 0 {
		builder.WriteString(fmt.Sprintf("first_metric=%s/%s@%s\n", metrics[0].ResourceID, metrics[0].MetricName, metrics[0].Timestamp.Format(time.RFC3339)))
	}
	if len(logs) > 0 {
		builder.WriteString(fmt.Sprintf("first_log=%s/%s@%s\n", logs[0].EventID, logs[0].Level, logs[0].Timestamp.Format(time.RFC3339)))
	}
	return builder.String()
}

func collectMetricIDs(results []cwtypes.MetricDataResult) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, derefString(result.Id))
	}
	return ids
}

func collectLogEventIDs(events []cwlogstypes.FilteredLogEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, derefString(event.EventId))
	}
	return ids
}

type fakeMetricClient struct {
	output    *cwapi.GetMetricDataOutput
	calls     int
	lastInput *cwapi.GetMetricDataInput
}

func (f *fakeMetricClient) GetMetricData(_ context.Context, input *cwapi.GetMetricDataInput, _ ...func(*cwapi.Options)) (*cwapi.GetMetricDataOutput, error) {
	f.calls++
	f.lastInput = input
	return f.output, nil
}

type fakeLogClient struct {
	output    *cwlogsapi.FilterLogEventsOutput
	calls     int
	lastInput *cwlogsapi.FilterLogEventsInput
}

func (f *fakeLogClient) FilterLogEvents(_ context.Context, input *cwlogsapi.FilterLogEventsInput, _ ...func(*cwlogsapi.Options)) (*cwlogsapi.FilterLogEventsOutput, error) {
	f.calls++
	f.lastInput = input
	return f.output, nil
}

func awsString(v string) *string {
	return &v
}

func awsInt64(v int64) *int64 {
	return &v
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
