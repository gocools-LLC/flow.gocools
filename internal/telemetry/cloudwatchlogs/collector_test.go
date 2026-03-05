package cloudwatchlogs

import (
	"context"
	"testing"
	"time"

	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/smithy-go"
)

func TestCollectHandlesPaginationAndTimeWindow(t *testing.T) {
	client := &fakeClient{
		pages: []*cwl.FilterLogEventsOutput{
			{
				Events: []types.FilteredLogEvent{
					{
						EventId:       awsString("event-1"),
						LogStreamName: awsString("stream-a"),
						Message:       awsString(`{"level":"info","correlation_id":"cid-1","msg":"ok"}`),
						Timestamp:     awsInt64(time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC).UnixMilli()),
					},
				},
				NextToken: awsString("page-2"),
			},
			{
				Events: []types.FilteredLogEvent{
					{
						EventId:       awsString("event-2"),
						LogStreamName: awsString("stream-a"),
						Message:       awsString("error request_id=req-2 timeout"),
						Timestamp:     awsInt64(time.Date(2026, 3, 5, 0, 1, 0, 0, time.UTC).UnixMilli()),
					},
				},
			},
		},
	}

	collector := NewCollector(client, CollectorConfig{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		Hooks:          []ParseHook{CorrelationIDRegexHook()},
	})
	collector.sleep = func(time.Duration) {}
	collector.now = func() time.Time { return time.Date(2026, 3, 5, 0, 5, 0, 0, time.UTC) }

	records, err := collector.Collect(context.Background(), CollectRequest{
		LogGroupName:  "group-a",
		StartTime:     time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
		EndTime:       time.Date(2026, 3, 5, 0, 5, 0, 0, time.UTC),
		FilterPattern: "ERROR",
	})
	if err != nil {
		t.Fatalf("collect returned error: %v", err)
	}

	if len(client.inputs) != 2 {
		t.Fatalf("expected 2 paginated calls, got %d", len(client.inputs))
	}

	firstInput := client.inputs[0]
	if firstInput.StartTime == nil || *firstInput.StartTime != time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("unexpected start time in request: %v", firstInput.StartTime)
	}
	if firstInput.EndTime == nil || *firstInput.EndTime != time.Date(2026, 3, 5, 0, 5, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("unexpected end time in request: %v", firstInput.EndTime)
	}
	if firstInput.FilterPattern == nil || *firstInput.FilterPattern != "ERROR" {
		t.Fatalf("expected filter pattern ERROR, got %v", firstInput.FilterPattern)
	}

	secondInput := client.inputs[1]
	if secondInput.NextToken == nil || *secondInput.NextToken != "page-2" {
		t.Fatalf("expected second request token page-2, got %v", secondInput.NextToken)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].CorrelationID != "cid-1" {
		t.Fatalf("expected first correlation id cid-1, got %q", records[0].CorrelationID)
	}
	if records[1].CorrelationID != "req-2" {
		t.Fatalf("expected regex-derived correlation id req-2, got %q", records[1].CorrelationID)
	}
}

func TestCollectMalformedLogsDoNotCrash(t *testing.T) {
	client := &fakeClient{
		pages: []*cwl.FilterLogEventsOutput{
			{
				Events: []types.FilteredLogEvent{
					{
						EventId:       awsString("event-1"),
						LogStreamName: awsString("stream-a"),
						Message:       awsString(`{"level":"info"`),
					},
				},
			},
		},
	}

	collector := NewCollector(client, CollectorConfig{})
	collector.sleep = func(time.Duration) {}
	collector.now = func() time.Time { return time.Date(2026, 3, 5, 0, 5, 0, 0, time.UTC) }

	records, err := collector.Collect(context.Background(), CollectRequest{
		LogGroupName: "group-a",
	})
	if err != nil {
		t.Fatalf("collect returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].ParserError == "" {
		t.Fatal("expected parser error for malformed log message")
	}
}

func TestCollectRetriesOnThrottling(t *testing.T) {
	client := &fakeClient{
		pages: []*cwl.FilterLogEventsOutput{
			{Events: []types.FilteredLogEvent{}},
		},
		failuresBeforeSuccess: 1,
	}

	collector := NewCollector(client, CollectorConfig{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
	})
	collector.sleep = func(time.Duration) {}
	collector.now = func() time.Time { return time.Date(2026, 3, 5, 0, 5, 0, 0, time.UTC) }

	_, err := collector.Collect(context.Background(), CollectRequest{
		LogGroupName: "group-a",
	})
	if err != nil {
		t.Fatalf("collect returned error: %v", err)
	}

	if client.calls != 2 {
		t.Fatalf("expected 2 calls due to retry, got %d", client.calls)
	}
}

type fakeClient struct {
	pages                 []*cwl.FilterLogEventsOutput
	inputs                []*cwl.FilterLogEventsInput
	calls                 int
	failuresBeforeSuccess int
}

func (f *fakeClient) FilterLogEvents(_ context.Context, input *cwl.FilterLogEventsInput, _ ...func(*cwl.Options)) (*cwl.FilterLogEventsOutput, error) {
	f.calls++
	f.inputs = append(f.inputs, input)

	if f.calls <= f.failuresBeforeSuccess {
		return nil, &smithy.GenericAPIError{
			Code:    "ThrottlingException",
			Message: "rate exceeded",
		}
	}

	pageIndex := f.calls - f.failuresBeforeSuccess - 1
	if pageIndex < 0 || pageIndex >= len(f.pages) {
		return &cwl.FilterLogEventsOutput{}, nil
	}
	return f.pages[pageIndex], nil
}
