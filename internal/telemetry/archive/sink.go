package archive

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/storage"
)

type Sink struct {
	logger *slog.Logger
	store  storage.Store
	now    func() time.Time
}

func NewSink(logger *slog.Logger, store storage.Store) *Sink {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Sink{
		logger: logger,
		store:  store,
		now:    time.Now,
	}
}

func (s *Sink) AddMetricPoints(points ...cloudwatch.MetricPoint) {
	if s == nil || s.store == nil || len(points) == 0 {
		return
	}

	for _, point := range points {
		payload, err := json.Marshal(point)
		if err != nil {
			s.logger.Error("telemetry_archive_metric_marshal_failed", "error", err)
			continue
		}

		key := metricKey(point, s.now())
		err = s.store.Put(context.Background(), key, payload, map[string]string{
			"type":        "metric",
			"resource_id": point.ResourceID,
			"namespace":   point.Namespace,
			"metric_name": point.MetricName,
		})
		if err != nil {
			s.logger.Error("telemetry_archive_metric_put_failed", "error", err, "key", key)
		}
	}
}

func (s *Sink) AddLogRecords(records ...cloudwatchlogs.LogRecord) {
	if s == nil || s.store == nil || len(records) == 0 {
		return
	}

	for _, record := range records {
		payload, err := json.Marshal(record)
		if err != nil {
			s.logger.Error("telemetry_archive_log_marshal_failed", "error", err)
			continue
		}

		key := logKey(record, s.now())
		err = s.store.Put(context.Background(), key, payload, map[string]string{
			"type":            "log",
			"log_group_name":  record.LogGroupName,
			"log_stream_name": record.LogStreamName,
			"event_id":        record.EventID,
		})
		if err != nil {
			s.logger.Error("telemetry_archive_log_put_failed", "error", err, "key", key)
		}
	}
}

func metricKey(point cloudwatch.MetricPoint, fallbackNow time.Time) string {
	ts := point.Timestamp.UTC()
	if ts.IsZero() {
		ts = fallbackNow.UTC()
	}

	keyMaterial := strings.Join([]string{
		point.ResourceID,
		point.Namespace,
		point.MetricName,
		ts.Format(time.RFC3339Nano),
		fmt.Sprintf("%.6f", point.Value),
	}, "|")

	return fmt.Sprintf(
		"metrics/%s/%s-%s-%s.json",
		ts.Format("2006/01/02/15"),
		sanitizeToken(point.ResourceID),
		sanitizeToken(point.MetricName),
		shortHash(keyMaterial),
	)
}

func logKey(record cloudwatchlogs.LogRecord, fallbackNow time.Time) string {
	ts := record.Timestamp.UTC()
	if ts.IsZero() {
		ts = record.IngestionTime.UTC()
	}
	if ts.IsZero() {
		ts = fallbackNow.UTC()
	}

	eventID := strings.TrimSpace(record.EventID)
	if eventID == "" {
		eventID = shortHash(strings.Join([]string{
			record.LogGroupName,
			record.LogStreamName,
			record.Message,
			ts.Format(time.RFC3339Nano),
		}, "|"))
	}

	return fmt.Sprintf(
		"logs/%s/%s-%s.json",
		ts.Format("2006/01/02/15"),
		sanitizeToken(record.LogGroupName),
		sanitizeToken(eventID),
	)
}

func sanitizeToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}

	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	out := strings.Trim(builder.String(), "._-")
	if out == "" {
		return "unknown"
	}
	if len(out) > 80 {
		return out[:80]
	}
	return out
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:8])
}
