package ingestion

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/analyzer/timeline"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
)

type metricCollector interface {
	Collect(ctx context.Context, targets []cloudwatch.ResourceTarget, window time.Duration) ([]cloudwatch.MetricPoint, error)
}

type metricPointAppender interface {
	AddMetricPoints(points ...cloudwatch.MetricPoint)
}

type MetricsTimelineIngestor struct {
	logger     *slog.Logger
	cfg        RuntimeConfig
	targets    []cloudwatch.ResourceTarget
	collector  metricCollector
	timeline   timelineAppender
	metricSink metricPointAppender

	now     func() time.Time
	lastEnd time.Time
	seenIDs map[string]time.Time
}

func NewMetricsTimelineIngestor(
	logger *slog.Logger,
	cfg RuntimeConfig,
	targets []cloudwatch.ResourceTarget,
	collector metricCollector,
	timeline timelineAppender,
) *MetricsTimelineIngestor {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	copyTargets := make([]cloudwatch.ResourceTarget, len(targets))
	copy(copyTargets, targets)

	return &MetricsTimelineIngestor{
		logger:    logger,
		cfg:       cfg,
		targets:   copyTargets,
		collector: collector,
		timeline:  timeline,
		now:       time.Now,
		seenIDs:   map[string]time.Time{},
	}
}

func (i *MetricsTimelineIngestor) WithMetricPointSink(sink metricPointAppender) *MetricsTimelineIngestor {
	i.metricSink = sink
	return i
}

func (i *MetricsTimelineIngestor) Run(ctx context.Context) {
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

func (i *MetricsTimelineIngestor) runTick(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, i.cfg.Timeout())
	defer cancel()

	if err := i.ingestOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		i.logger.Error("cloudwatch_metrics_ingestion_failed", "error", err)
	}
}

func (i *MetricsTimelineIngestor) ingestOnce(ctx context.Context) error {
	points, err := i.collector.Collect(ctx, i.targets, i.cfg.Window())
	if err != nil {
		return err
	}

	now := i.now().UTC()
	i.pruneSeen(now)

	if len(points) == 0 {
		return nil
	}

	events := make([]timeline.Event, 0, len(points))
	addedPoints := make([]cloudwatch.MetricPoint, 0, len(points))
	for _, point := range points {
		severity, emit := metricSeverity(point, i.cfg.MetricWarnThreshold(), i.cfg.MetricErrorThreshold())
		if !emit {
			continue
		}

		id := metricEventID(point)
		if _, exists := i.seenIDs[id]; exists {
			continue
		}
		i.seenIDs[id] = now
		addedPoints = append(addedPoints, point)

		timestamp := point.Timestamp.UTC()
		if timestamp.IsZero() {
			timestamp = now
		}

		events = append(events, timeline.Event{
			ID:        id,
			Timestamp: timestamp,
			Severity:  severity,
			Source:    metricSource(point),
			Message:   metricMessage(point, severity),
		})
	}

	if len(events) == 0 {
		return nil
	}

	i.timeline.AddEvents(events...)
	if i.metricSink != nil {
		i.metricSink.AddMetricPoints(addedPoints...)
	}
	i.lastEnd = now
	i.logger.Info(
		"cloudwatch_metrics_ingested",
		"targets", len(i.targets),
		"points_seen", len(points),
		"events_added", len(events),
		"warn_threshold", i.cfg.MetricWarnThreshold(),
		"error_threshold", i.cfg.MetricErrorThreshold(),
	)
	return nil
}

func (i *MetricsTimelineIngestor) pruneSeen(now time.Time) {
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

func metricSeverity(point cloudwatch.MetricPoint, warnThreshold, errorThreshold float64) (timeline.Severity, bool) {
	metric := strings.ToLower(strings.TrimSpace(point.MetricName))
	switch metric {
	case "cpuutilization", "memoryutilization":
		if point.Value >= errorThreshold {
			return timeline.SeverityError, true
		}
		if point.Value >= warnThreshold {
			return timeline.SeverityWarning, true
		}
		return timeline.SeverityInfo, false
	default:
		return timeline.SeverityInfo, false
	}
}

func metricEventID(point cloudwatch.MetricPoint) string {
	key := strings.Join([]string{
		point.ResourceID,
		point.Namespace,
		point.MetricName,
		point.Timestamp.UTC().Format(time.RFC3339Nano),
		fmt.Sprintf("%.6f", point.Value),
	}, "|")
	sum := sha1.Sum([]byte(key))
	return "cwmetric-" + hex.EncodeToString(sum[:8])
}

func metricSource(point cloudwatch.MetricPoint) string {
	resource := strings.TrimSpace(point.ResourceID)
	if resource == "" {
		resource = "unknown"
	}
	return "cloudwatch_metrics/" + resource
}

func metricMessage(point cloudwatch.MetricPoint, severity timeline.Severity) string {
	return fmt.Sprintf(
		"%s %s at %.2f on %s (%s)",
		point.Namespace,
		point.MetricName,
		point.Value,
		point.ResourceID,
		severity,
	)
}
