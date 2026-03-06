package signals

import (
	"slices"
	"sync"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
)

const (
	defaultMaxMetrics = 20000
	defaultMaxLogs    = 20000
)

type Config struct {
	MaxMetrics int
	MaxLogs    int
}

type Query struct {
	Start time.Time
	End   time.Time
}

type InMemoryStore struct {
	mu sync.RWMutex

	maxMetrics int
	maxLogs    int
	metrics    []cloudwatch.MetricPoint
	logs       []cloudwatchlogs.LogRecord
}

func NewInMemoryStore(cfg Config) *InMemoryStore {
	maxMetrics := cfg.MaxMetrics
	if maxMetrics <= 0 {
		maxMetrics = defaultMaxMetrics
	}
	maxLogs := cfg.MaxLogs
	if maxLogs <= 0 {
		maxLogs = defaultMaxLogs
	}

	return &InMemoryStore{
		maxMetrics: maxMetrics,
		maxLogs:    maxLogs,
		metrics:    make([]cloudwatch.MetricPoint, 0, maxMetrics),
		logs:       make([]cloudwatchlogs.LogRecord, 0, maxLogs),
	}
}

func (s *InMemoryStore) AddMetricPoints(points ...cloudwatch.MetricPoint) {
	if len(points) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics = append(s.metrics, points...)
	if len(s.metrics) > s.maxMetrics {
		s.metrics = append([]cloudwatch.MetricPoint(nil), s.metrics[len(s.metrics)-s.maxMetrics:]...)
	}
}

func (s *InMemoryStore) AddLogRecords(records ...cloudwatchlogs.LogRecord) {
	if len(records) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, record := range records {
		record.Fields = copyFields(record.Fields)
		s.logs = append(s.logs, record)
	}
	if len(s.logs) > s.maxLogs {
		s.logs = append([]cloudwatchlogs.LogRecord(nil), s.logs[len(s.logs)-s.maxLogs:]...)
	}
}

func (s *InMemoryStore) QueryMetricPoints(query Query) []cloudwatch.MetricPoint {
	start, end := queryBounds(query)

	s.mu.RLock()
	defer s.mu.RUnlock()

	filtered := make([]cloudwatch.MetricPoint, 0, len(s.metrics))
	for _, point := range s.metrics {
		ts := point.Timestamp.UTC()
		if !start.IsZero() && ts.Before(start) {
			continue
		}
		if !end.IsZero() && ts.After(end) {
			continue
		}
		filtered = append(filtered, point)
	}

	slices.SortFunc(filtered, func(a, b cloudwatch.MetricPoint) int {
		if a.Timestamp.Before(b.Timestamp) {
			return -1
		}
		if a.Timestamp.After(b.Timestamp) {
			return 1
		}
		if a.ResourceID < b.ResourceID {
			return -1
		}
		if a.ResourceID > b.ResourceID {
			return 1
		}
		if a.MetricName < b.MetricName {
			return -1
		}
		if a.MetricName > b.MetricName {
			return 1
		}
		return 0
	})

	return filtered
}

func (s *InMemoryStore) QueryLogRecords(query Query) []cloudwatchlogs.LogRecord {
	start, end := queryBounds(query)

	s.mu.RLock()
	defer s.mu.RUnlock()

	filtered := make([]cloudwatchlogs.LogRecord, 0, len(s.logs))
	for _, record := range s.logs {
		ts := record.Timestamp.UTC()
		if ts.IsZero() {
			ts = record.IngestionTime.UTC()
		}

		if !start.IsZero() && ts.Before(start) {
			continue
		}
		if !end.IsZero() && ts.After(end) {
			continue
		}

		recordCopy := record
		recordCopy.Fields = copyFields(record.Fields)
		filtered = append(filtered, recordCopy)
	}

	slices.SortFunc(filtered, func(a, b cloudwatchlogs.LogRecord) int {
		aTS := a.Timestamp.UTC()
		bTS := b.Timestamp.UTC()
		if aTS.Before(bTS) {
			return -1
		}
		if aTS.After(bTS) {
			return 1
		}
		if a.EventID < b.EventID {
			return -1
		}
		if a.EventID > b.EventID {
			return 1
		}
		return 0
	})

	return filtered
}

func queryBounds(query Query) (time.Time, time.Time) {
	return query.Start.UTC(), query.End.UTC()
}

func copyFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}

	out := make(map[string]string, len(fields))
	for key, value := range fields {
		out[key] = value
	}
	return out
}
