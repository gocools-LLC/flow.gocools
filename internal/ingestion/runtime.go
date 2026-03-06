package ingestion

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/telemetry/cloudwatch"
)

const (
	ModeDisabled         = "disabled"
	ModeCloudWatchLog    = "cloudwatch_logs"
	ModeCloudWatchMetric = "cloudwatch_metrics"
	ModeCloudWatchAll    = "cloudwatch_all"

	defaultPollInterval        = 30 * time.Second
	defaultCollectWindow       = 5 * time.Minute
	defaultRequestTimeout      = 20 * time.Second
	defaultRecordLimit         = 500
	defaultUtilizationWarnPct  = 70.0
	defaultUtilizationErrorPct = 90.0
)

type RuntimeConfig struct {
	Mode           string
	PollInterval   time.Duration
	CollectWindow  time.Duration
	RequestTimeout time.Duration

	LogGroupName     string
	FilterPattern    string
	Limit            int32
	MetricTargetsRaw string
	UtilizationWarn  float64
	UtilizationError float64
}

func RuntimeConfigFromEnv() RuntimeConfig {
	return RuntimeConfig{
		Mode:             envOrDefault("FLOW_INGEST_MODE", ModeDisabled),
		PollInterval:     envDurationOrDefault("FLOW_INGEST_INTERVAL", defaultPollInterval),
		CollectWindow:    envDurationOrDefault("FLOW_INGEST_WINDOW", defaultCollectWindow),
		RequestTimeout:   envDurationOrDefault("FLOW_INGEST_TIMEOUT", defaultRequestTimeout),
		LogGroupName:     strings.TrimSpace(os.Getenv("FLOW_CW_LOG_GROUP")),
		FilterPattern:    strings.TrimSpace(os.Getenv("FLOW_CW_FILTER_PATTERN")),
		Limit:            int32(envIntOrDefault("FLOW_CW_LIMIT", defaultRecordLimit)),
		MetricTargetsRaw: strings.TrimSpace(os.Getenv("FLOW_CW_METRIC_TARGETS")),
		UtilizationWarn:  envFloatOrDefault("FLOW_CW_METRIC_UTIL_WARN", defaultUtilizationWarnPct),
		UtilizationError: envFloatOrDefault("FLOW_CW_METRIC_UTIL_ERROR", defaultUtilizationErrorPct),
	}
}

func (c RuntimeConfig) NormalizedMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		return ModeDisabled
	}
	return mode
}

func (c RuntimeConfig) PollEvery() time.Duration {
	if c.PollInterval <= 0 {
		return defaultPollInterval
	}
	return c.PollInterval
}

func (c RuntimeConfig) Window() time.Duration {
	if c.CollectWindow <= 0 {
		return defaultCollectWindow
	}
	return c.CollectWindow
}

func (c RuntimeConfig) Timeout() time.Duration {
	if c.RequestTimeout <= 0 {
		return defaultRequestTimeout
	}
	return c.RequestTimeout
}

func (c RuntimeConfig) RecordLimit() int32 {
	if c.Limit <= 0 {
		return defaultRecordLimit
	}
	return c.Limit
}

func (c RuntimeConfig) MetricWarnThreshold() float64 {
	if c.UtilizationWarn <= 0 {
		return defaultUtilizationWarnPct
	}
	return c.UtilizationWarn
}

func (c RuntimeConfig) MetricErrorThreshold() float64 {
	if c.UtilizationError <= 0 {
		return defaultUtilizationErrorPct
	}
	return c.UtilizationError
}

func (c RuntimeConfig) MetricTargets() ([]cloudwatch.ResourceTarget, error) {
	return parseMetricTargets(c.MetricTargetsRaw)
}

func (c RuntimeConfig) Validate() error {
	switch c.NormalizedMode() {
	case ModeDisabled:
		return nil
	case ModeCloudWatchLog:
		if strings.TrimSpace(c.LogGroupName) == "" {
			return fmt.Errorf("flow cloudwatch logs ingestion requires FLOW_CW_LOG_GROUP")
		}
		if c.MetricWarnThreshold() >= c.MetricErrorThreshold() {
			return fmt.Errorf("FLOW_CW_METRIC_UTIL_WARN must be lower than FLOW_CW_METRIC_UTIL_ERROR")
		}
		return nil
	case ModeCloudWatchMetric:
		targets, err := c.MetricTargets()
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			return fmt.Errorf("flow cloudwatch metrics ingestion requires FLOW_CW_METRIC_TARGETS")
		}
		if c.MetricWarnThreshold() >= c.MetricErrorThreshold() {
			return fmt.Errorf("FLOW_CW_METRIC_UTIL_WARN must be lower than FLOW_CW_METRIC_UTIL_ERROR")
		}
		return nil
	case ModeCloudWatchAll:
		if strings.TrimSpace(c.LogGroupName) == "" {
			return fmt.Errorf("flow cloudwatch all ingestion requires FLOW_CW_LOG_GROUP")
		}
		targets, err := c.MetricTargets()
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			return fmt.Errorf("flow cloudwatch all ingestion requires FLOW_CW_METRIC_TARGETS")
		}
		if c.MetricWarnThreshold() >= c.MetricErrorThreshold() {
			return fmt.Errorf("FLOW_CW_METRIC_UTIL_WARN must be lower than FLOW_CW_METRIC_UTIL_ERROR")
		}
		return nil
	default:
		return fmt.Errorf("unsupported FLOW_INGEST_MODE: %q", c.Mode)
	}
}

func parseMetricTargets(raw string) ([]cloudwatch.ResourceTarget, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	items := strings.Split(trimmed, ",")
	targets := make([]cloudwatch.ResourceTarget, 0, len(items))
	for _, item := range items {
		token := strings.TrimSpace(item)
		if token == "" {
			continue
		}

		parts := strings.SplitN(token, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid FLOW_CW_METRIC_TARGETS token %q; expected <type>:<id>", token)
		}

		targetType := strings.ToLower(strings.TrimSpace(parts[0]))
		targetValue := strings.TrimSpace(parts[1])
		if targetValue == "" {
			return nil, fmt.Errorf("invalid FLOW_CW_METRIC_TARGETS token %q; target value is empty", token)
		}

		switch targetType {
		case "ec2":
			targets = append(targets, cloudwatch.ResourceTarget{
				Kind:       cloudwatch.ResourceKindEC2,
				InstanceID: targetValue,
			})
		case "ecs", "ecs_service":
			clusterService := strings.SplitN(targetValue, "/", 2)
			if len(clusterService) != 2 || strings.TrimSpace(clusterService[0]) == "" || strings.TrimSpace(clusterService[1]) == "" {
				return nil, fmt.Errorf("invalid FLOW_CW_METRIC_TARGETS token %q; ecs value must be <cluster>/<service>", token)
			}
			targets = append(targets, cloudwatch.ResourceTarget{
				Kind:        cloudwatch.ResourceKindECSService,
				ClusterName: strings.TrimSpace(clusterService[0]),
				ServiceName: strings.TrimSpace(clusterService[1]),
			})
		default:
			return nil, fmt.Errorf("invalid FLOW_CW_METRIC_TARGETS token %q; unsupported type %q", token, targetType)
		}
	}

	slices.SortStableFunc(targets, func(a, b cloudwatch.ResourceTarget) int {
		left := string(a.Kind) + ":" + a.InstanceID + ":" + a.ClusterName + ":" + a.ServiceName
		right := string(b.Kind) + ":" + b.InstanceID + ":" + b.ClusterName + ":" + b.ServiceName
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
		return 0
	})

	return targets, nil
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envIntOrDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloatOrDefault(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
