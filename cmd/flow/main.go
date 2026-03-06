package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/analyzer/timeline"
	internalaws "github.com/gocools-LLC/flow.gocools/internal/aws"
	"github.com/gocools-LLC/flow.gocools/internal/httpserver"
	"github.com/gocools-LLC/flow.gocools/internal/ingestion"
	"github.com/gocools-LLC/flow.gocools/internal/observability"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatchlogs"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("flow exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	addr := envOrDefault("FLOW_HTTP_ADDR", ":8080")
	awsRuntimeConfig := internalaws.RuntimeConfigFromEnv()
	ingestionRuntimeConfig := ingestion.RuntimeConfigFromEnv()
	ingestionMode := ingestionRuntimeConfig.NormalizedMode()
	metricTargets, _ := ingestionRuntimeConfig.MetricTargets()

	logger.Info(
		"timeline_ingestion_configuration",
		"mode", ingestionMode,
		"poll_interval", ingestionRuntimeConfig.PollEvery(),
		"collect_window", ingestionRuntimeConfig.Window(),
		"request_timeout", ingestionRuntimeConfig.Timeout(),
		"log_group", ingestionRuntimeConfig.LogGroupName,
		"filter_pattern_set", ingestionRuntimeConfig.FilterPattern != "",
		"limit", ingestionRuntimeConfig.RecordLimit(),
		"metric_target_count", len(metricTargets),
		"metric_warn_threshold", ingestionRuntimeConfig.MetricWarnThreshold(),
		"metric_error_threshold", ingestionRuntimeConfig.MetricErrorThreshold(),
	)

	if err := ingestionRuntimeConfig.Validate(); err != nil {
		return err
	}

	logger.Info(
		"aws_auth_configuration",
		"region", awsRuntimeConfig.Session.Region,
		"role_arn_set", awsRuntimeConfig.Session.RoleARN != "",
		"session_name", awsRuntimeConfig.Session.SessionName,
		"external_id_set", awsRuntimeConfig.Session.ExternalID != "",
		"validate_on_start", awsRuntimeConfig.ValidateOnStart,
	)

	if awsRuntimeConfig.ValidateOnStart {
		validateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := internalaws.ValidateCredentials(validateCtx, awsRuntimeConfig.Session); err != nil {
			return err
		}
		logger.Info("aws_credentials_validation_succeeded")
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	timelineService := timeline.NewInMemoryService(nil)
	if err := startTimelineIngestion(runCtx, logger, awsRuntimeConfig, ingestionRuntimeConfig, timelineService); err != nil {
		return err
	}

	tracingShutdown, err := observability.InitTracing(context.Background(), "flow", logger)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := tracingShutdown(shutdownCtx); shutdownErr != nil {
			logger.Error("tracing shutdown failed", "error", shutdownErr)
		}
	}()

	srv := httpserver.New(httpserver.Config{
		Addr:            addr,
		Version:         version,
		Logger:          logger,
		TimelineService: timelineService,
	})

	serverErrCh := make(chan error, 1)
	go func() {
		logger.Info("starting flow service", "addr", addr, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrCh:
		runCancel()
		return err
	case <-sigCtx.Done():
		logger.Info("shutdown signal received")
		runCancel()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	logger.Info("flow service stopped")
	return nil
}

func startTimelineIngestion(
	ctx context.Context,
	logger *slog.Logger,
	awsConfig internalaws.RuntimeConfig,
	runtimeConfig ingestion.RuntimeConfig,
	timelineService *timeline.InMemoryService,
) error {
	mode := runtimeConfig.NormalizedMode()
	switch mode {
	case ingestion.ModeDisabled:
		logger.Info("timeline_ingestion_disabled")
		return nil
	case ingestion.ModeCloudWatchLog:
		if strings.TrimSpace(awsConfig.Session.Region) == "" {
			return fmt.Errorf("flow cloudwatch logs ingestion requires FLOW_AWS_REGION or AWS_REGION")
		}

		client, err := cloudwatchlogs.NewAWSClient(ctx, cloudwatchlogs.ClientConfig{
			Region:      awsConfig.Session.Region,
			RoleARN:     awsConfig.Session.RoleARN,
			SessionName: awsConfig.Session.SessionName,
			ExternalID:  awsConfig.Session.ExternalID,
		})
		if err != nil {
			return fmt.Errorf("create cloudwatch logs client: %w", err)
		}

		collector := cloudwatchlogs.NewCollector(client, cloudwatchlogs.CollectorConfig{
			Hooks: []cloudwatchlogs.ParseHook{
				cloudwatchlogs.CorrelationIDRegexHook(),
			},
		})
		ingestor := ingestion.NewLogsTimelineIngestor(logger, runtimeConfig, collector, timelineService)
		go ingestor.Run(ctx)

		logger.Info(
			"timeline_ingestion_enabled",
			"mode", mode,
			"log_group", runtimeConfig.LogGroupName,
			"poll_interval", runtimeConfig.PollEvery(),
			"collect_window", runtimeConfig.Window(),
			"request_timeout", runtimeConfig.Timeout(),
			"limit", runtimeConfig.RecordLimit(),
		)
		return nil
	case ingestion.ModeCloudWatchMetric:
		if strings.TrimSpace(awsConfig.Session.Region) == "" {
			return fmt.Errorf("flow cloudwatch metrics ingestion requires FLOW_AWS_REGION or AWS_REGION")
		}

		targets, err := runtimeConfig.MetricTargets()
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			return fmt.Errorf("flow cloudwatch metrics ingestion requires FLOW_CW_METRIC_TARGETS")
		}

		client, err := cloudwatch.NewAWSClient(ctx, cloudwatch.ClientConfig{
			Region:      awsConfig.Session.Region,
			RoleARN:     awsConfig.Session.RoleARN,
			SessionName: awsConfig.Session.SessionName,
			ExternalID:  awsConfig.Session.ExternalID,
		})
		if err != nil {
			return fmt.Errorf("create cloudwatch metrics client: %w", err)
		}

		collector := cloudwatch.NewCollector(client, cloudwatch.CollectorConfig{})
		ingestor := ingestion.NewMetricsTimelineIngestor(logger, runtimeConfig, targets, collector, timelineService)
		go ingestor.Run(ctx)

		logger.Info(
			"timeline_ingestion_enabled",
			"mode", mode,
			"target_count", len(targets),
			"poll_interval", runtimeConfig.PollEvery(),
			"collect_window", runtimeConfig.Window(),
			"request_timeout", runtimeConfig.Timeout(),
			"warn_threshold", runtimeConfig.MetricWarnThreshold(),
			"error_threshold", runtimeConfig.MetricErrorThreshold(),
		)
		return nil
	default:
		return fmt.Errorf("unsupported FLOW_INGEST_MODE: %q", runtimeConfig.Mode)
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
