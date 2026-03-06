package archive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/storage"
)

type capturedPut struct {
	key      string
	data     []byte
	metadata map[string]string
}

type fakeStore struct {
	puts []capturedPut
}

func (f *fakeStore) Put(_ context.Context, key string, data []byte, metadata map[string]string) error {
	clonedMeta := map[string]string{}
	for k, v := range metadata {
		clonedMeta[k] = v
	}
	f.puts = append(f.puts, capturedPut{
		key:      key,
		data:     append([]byte(nil), data...),
		metadata: clonedMeta,
	})
	return nil
}

func (f *fakeStore) Get(context.Context, string) (storage.Object, error) {
	return storage.Object{}, storage.ErrNotFound
}

func (f *fakeStore) List(context.Context, string) ([]storage.ObjectInfo, error) {
	return []storage.ObjectInfo{}, nil
}

func (f *fakeStore) Delete(context.Context, string) error {
	return nil
}

func (f *fakeStore) ApplyRetention(context.Context, storage.RetentionPolicy) ([]storage.ObjectInfo, error) {
	return []storage.ObjectInfo{}, nil
}

func TestSinkAddMetricPointsPersistsJSON(t *testing.T) {
	store := &fakeStore{}
	sink := NewSink(nil, store)
	now := time.Date(2026, 3, 6, 16, 0, 0, 0, time.UTC)
	sink.now = func() time.Time { return now }

	sink.AddMetricPoints(cloudwatch.MetricPoint{
		ResourceID: "i-123",
		Namespace:  "AWS/EC2",
		MetricName: "CPUUtilization",
		Timestamp:  now,
		Value:      88.5,
	})

	if len(store.puts) != 1 {
		t.Fatalf("expected one archived metric object, got %d", len(store.puts))
	}
	if !strings.HasPrefix(store.puts[0].key, "metrics/2026/03/06/16/") {
		t.Fatalf("unexpected metric key: %s", store.puts[0].key)
	}
	if got := store.puts[0].metadata["type"]; got != "metric" {
		t.Fatalf("expected metric metadata type, got %q", got)
	}

	var payload cloudwatch.MetricPoint
	if err := json.Unmarshal(store.puts[0].data, &payload); err != nil {
		t.Fatalf("failed to decode stored payload: %v", err)
	}
	if payload.ResourceID != "i-123" || payload.MetricName != "CPUUtilization" {
		t.Fatalf("unexpected metric payload: %+v", payload)
	}
}

func TestSinkAddLogRecordsPersistsJSON(t *testing.T) {
	store := &fakeStore{}
	sink := NewSink(nil, store)
	now := time.Date(2026, 3, 6, 16, 0, 0, 0, time.UTC)
	sink.now = func() time.Time { return now }

	sink.AddLogRecords(cloudwatchlogs.LogRecord{
		LogGroupName:  "/aws/ecs/dev",
		LogStreamName: "service-a/1",
		EventID:       "event-1",
		Timestamp:     now,
		Message:       "request failed",
		Level:         "error",
	})

	if len(store.puts) != 1 {
		t.Fatalf("expected one archived log object, got %d", len(store.puts))
	}
	if !strings.HasPrefix(store.puts[0].key, "logs/2026/03/06/16/") {
		t.Fatalf("unexpected log key: %s", store.puts[0].key)
	}
	if got := store.puts[0].metadata["type"]; got != "log" {
		t.Fatalf("expected log metadata type, got %q", got)
	}

	var payload cloudwatchlogs.LogRecord
	if err := json.Unmarshal(store.puts[0].data, &payload); err != nil {
		t.Fatalf("failed to decode stored log payload: %v", err)
	}
	if payload.EventID != "event-1" || payload.LogGroupName != "/aws/ecs/dev" {
		t.Fatalf("unexpected log payload: %+v", payload)
	}
}
