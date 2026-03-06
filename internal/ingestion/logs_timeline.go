package ingestion

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/analyzer/timeline"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
)

const minSeenRetention = 30 * time.Minute

type logCollector interface {
	Collect(ctx context.Context, req cloudwatchlogs.CollectRequest) ([]cloudwatchlogs.LogRecord, error)
}

type timelineAppender interface {
	AddEvents(events ...timeline.Event)
}

type logRecordAppender interface {
	AddLogRecords(records ...cloudwatchlogs.LogRecord)
}

type LogsTimelineIngestor struct {
	logger    *slog.Logger
	cfg       RuntimeConfig
	collector logCollector
	timeline  timelineAppender
	logSink   logRecordAppender

	now     func() time.Time
	lastEnd time.Time
	seenIDs map[string]time.Time
}

func NewLogsTimelineIngestor(logger *slog.Logger, cfg RuntimeConfig, collector logCollector, timeline timelineAppender) *LogsTimelineIngestor {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &LogsTimelineIngestor{
		logger:    logger,
		cfg:       cfg,
		collector: collector,
		timeline:  timeline,
		now:       time.Now,
		seenIDs:   map[string]time.Time{},
	}
}

func (i *LogsTimelineIngestor) WithLogRecordSink(sink logRecordAppender) *LogsTimelineIngestor {
	i.logSink = sink
	return i
}

func (i *LogsTimelineIngestor) Run(ctx context.Context) {
	i.runTick(ctx)

	ticker := time.NewTicker(i.cfg.PollEvery())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			i.runTick(ctx)
		}
	}
}

func (i *LogsTimelineIngestor) runTick(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, i.cfg.Timeout())
	defer cancel()

	if err := i.ingestOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		i.logger.Error("cloudwatch_logs_ingestion_failed", "error", err)
	}
}

func (i *LogsTimelineIngestor) ingestOnce(ctx context.Context) error {
	end := i.now().UTC()
	start := i.lastEnd.UTC()

	if start.IsZero() || !start.Before(end) {
		start = end.Add(-i.cfg.Window())
	}

	records, err := i.collector.Collect(ctx, cloudwatchlogs.CollectRequest{
		LogGroupName:  i.cfg.LogGroupName,
		FilterPattern: i.cfg.FilterPattern,
		StartTime:     start,
		EndTime:       end,
		Limit:         i.cfg.RecordLimit(),
	})
	if err != nil {
		return err
	}

	i.lastEnd = end
	i.pruneSeen(end)

	if len(records) == 0 {
		return nil
	}

	events := make([]timeline.Event, 0, len(records))
	addedRecords := make([]cloudwatchlogs.LogRecord, 0, len(records))
	for _, record := range records {
		id := eventID(record)
		if _, exists := i.seenIDs[id]; exists {
			continue
		}
		i.seenIDs[id] = end
		addedRecords = append(addedRecords, record)

		events = append(events, timeline.Event{
			ID:            id,
			Timestamp:     eventTimestamp(record, end),
			Severity:      eventSeverity(record),
			Source:        eventSource(record),
			Message:       eventMessage(record),
			CorrelationID: strings.TrimSpace(record.CorrelationID),
		})
	}

	if len(events) == 0 {
		return nil
	}

	i.timeline.AddEvents(events...)
	if i.logSink != nil {
		i.logSink.AddLogRecords(addedRecords...)
	}
	i.logger.Info(
		"cloudwatch_logs_ingested",
		"log_group", i.cfg.LogGroupName,
		"records_seen", len(records),
		"events_added", len(events),
		"start", start.Format(time.RFC3339),
		"end", end.Format(time.RFC3339),
	)
	return nil
}

func (i *LogsTimelineIngestor) pruneSeen(now time.Time) {
	retention := i.cfg.Window() * 6
	if retention < minSeenRetention {
		retention = minSeenRetention
	}

	cutoff := now.Add(-retention)
	for eventID, seenAt := range i.seenIDs {
		if seenAt.Before(cutoff) {
			delete(i.seenIDs, eventID)
		}
	}
}

func eventID(record cloudwatchlogs.LogRecord) string {
	if id := strings.TrimSpace(record.EventID); id != "" {
		return id
	}

	raw := strings.Join([]string{
		record.LogGroupName,
		record.LogStreamName,
		record.Timestamp.UTC().Format(time.RFC3339Nano),
		record.IngestionTime.UTC().Format(time.RFC3339Nano),
		record.Message,
	}, "|")
	sum := sha1.Sum([]byte(raw))
	return "cwlog-" + hex.EncodeToString(sum[:8])
}

func eventSource(record cloudwatchlogs.LogRecord) string {
	group := strings.TrimSpace(record.LogGroupName)
	stream := strings.TrimSpace(record.LogStreamName)

	if group == "" && stream == "" {
		return "cloudwatch_logs"
	}
	if stream == "" {
		return group
	}
	if group == "" {
		return stream
	}
	return group + "/" + stream
}

func eventMessage(record cloudwatchlogs.LogRecord) string {
	message := strings.TrimSpace(record.Message)
	if message == "" {
		return "cloudwatch log event"
	}
	return message
}

func eventTimestamp(record cloudwatchlogs.LogRecord, fallback time.Time) time.Time {
	if !record.Timestamp.IsZero() {
		return record.Timestamp.UTC()
	}
	if !record.IngestionTime.IsZero() {
		return record.IngestionTime.UTC()
	}
	return fallback.UTC()
}

func eventSeverity(record cloudwatchlogs.LogRecord) timeline.Severity {
	level := strings.ToLower(strings.TrimSpace(record.Level))
	switch level {
	case "critical", "fatal", "panic":
		return timeline.SeverityCritical
	case "error", "err":
		return timeline.SeverityError
	case "warn", "warning":
		return timeline.SeverityWarning
	case "info":
		return timeline.SeverityInfo
	}

	message := strings.ToLower(strings.TrimSpace(record.Message))
	switch {
	case strings.Contains(message, "panic"), strings.Contains(message, "fatal"), strings.Contains(message, "critical"):
		return timeline.SeverityCritical
	case strings.Contains(message, "error"), strings.Contains(message, "failed"), strings.Contains(message, "exception"):
		return timeline.SeverityError
	case strings.Contains(message, "warn"):
		return timeline.SeverityWarning
	default:
		return timeline.SeverityInfo
	}
}
