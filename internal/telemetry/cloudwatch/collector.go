package cloudwatch

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/smithy-go"
)

type ResourceKind string

const (
	ResourceKindEC2        ResourceKind = "ec2"
	ResourceKindECSService ResourceKind = "ecs_service"
)

type ResourceTarget struct {
	Kind ResourceKind

	InstanceID  string
	ClusterName string
	ServiceName string
}

type MetricPoint struct {
	ResourceID string
	Namespace  string
	MetricName string
	Timestamp  time.Time
	Value      float64
}

type CollectorConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	MaxRetryBudget int
	JitterFraction float64
	Now            func() time.Time
	Sleep          func(time.Duration)
	RandFloat64    func() float64
}

type Client interface {
	GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

type Collector struct {
	client         Client
	maxAttempts    int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	maxRetryBudget int
	jitterFraction float64
	now            func() time.Time
	sleep          func(time.Duration)
	randFloat64    func() float64

	metrics struct {
		throttledResponses  atomic.Uint64
		retryAttempts       atomic.Uint64
		retryBudgetExceeded atomic.Uint64
		throttleDrops       atomic.Uint64
	}
}

type CollectorMetrics struct {
	ThrottledResponses  uint64 `json:"throttled_responses"`
	RetryAttempts       uint64 `json:"retry_attempts"`
	RetryBudgetExceeded uint64 `json:"retry_budget_exceeded"`
	ThrottleDrops       uint64 `json:"throttle_drops"`
}

type retryState struct {
	remainingBudget int
}

var ErrRetryBudgetExhausted = errors.New("cloudwatch retry budget exhausted")

func NewCollector(client Client, cfg CollectorConfig) *Collector {
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	initialBackoff := cfg.InitialBackoff
	if initialBackoff <= 0 {
		initialBackoff = 250 * time.Millisecond
	}

	maxBackoff := cfg.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 5 * time.Second
	}
	if maxBackoff < initialBackoff {
		maxBackoff = initialBackoff
	}

	maxRetryBudget := cfg.MaxRetryBudget
	if maxRetryBudget <= 0 {
		maxRetryBudget = maxAttempts - 1
	}
	if maxRetryBudget < 0 {
		maxRetryBudget = 0
	}

	jitterFraction := cfg.JitterFraction
	if jitterFraction <= 0 {
		jitterFraction = 0.2
	}
	if jitterFraction > 1 {
		jitterFraction = 1
	}

	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	sleep := cfg.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	randFloat64 := cfg.RandFloat64
	if randFloat64 == nil {
		randFloat64 = rand.Float64
	}

	return &Collector{
		client:         client,
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
		maxRetryBudget: maxRetryBudget,
		jitterFraction: jitterFraction,
		now:            now,
		sleep:          sleep,
		randFloat64:    randFloat64,
	}
}

func (c *Collector) Collect(ctx context.Context, targets []ResourceTarget, window time.Duration) ([]MetricPoint, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	if window <= 0 {
		window = 5 * time.Minute
	}

	queries, descriptors := buildMetricQueries(targets)
	if len(queries) == 0 {
		return nil, nil
	}

	endTime := c.now().UTC()
	startTime := endTime.Add(-window)

	input := &cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: queries,
	}

	retryState := retryState{
		remainingBudget: c.maxRetryBudget,
	}
	output, err := c.getMetricDataWithRetry(ctx, input, &retryState)
	if err != nil {
		return nil, err
	}

	return metricPointsFromOutput(output, descriptors), nil
}

func (c *Collector) getMetricDataWithRetry(ctx context.Context, input *cloudwatch.GetMetricDataInput, state *retryState) (*cloudwatch.GetMetricDataOutput, error) {
	backoff := c.initialBackoff

	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		output, err := c.client.GetMetricData(ctx, input)
		if err == nil {
			return output, nil
		}

		if !isThrottlingError(err) {
			return nil, err
		}

		c.metrics.throttledResponses.Add(1)
		if attempt == c.maxAttempts {
			c.metrics.throttleDrops.Add(1)
			return nil, err
		}

		if state.remainingBudget <= 0 {
			c.metrics.retryBudgetExceeded.Add(1)
			c.metrics.throttleDrops.Add(1)
			return nil, fmt.Errorf("%w: %v", ErrRetryBudgetExhausted, err)
		}

		state.remainingBudget--
		c.metrics.retryAttempts.Add(1)

		delay := c.jitteredBackoff(backoff)
		if err := c.waitBackoff(ctx, delay); err != nil {
			return nil, err
		}
		backoff = c.nextBackoff(backoff)
	}

	return nil, errors.New("unreachable retry state")
}

func (c *Collector) Metrics() CollectorMetrics {
	return CollectorMetrics{
		ThrottledResponses:  c.metrics.throttledResponses.Load(),
		RetryAttempts:       c.metrics.retryAttempts.Load(),
		RetryBudgetExceeded: c.metrics.retryBudgetExceeded.Load(),
		ThrottleDrops:       c.metrics.throttleDrops.Load(),
	}
}

func (c *Collector) jitteredBackoff(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	if c.jitterFraction <= 0 {
		return base
	}

	jitterScale := (c.randFloat64()*2 - 1) * c.jitterFraction
	delay := time.Duration(float64(base) * (1 + jitterScale))
	if delay < 0 {
		return 0
	}
	return delay
}

func (c *Collector) nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next <= 0 {
		return c.maxBackoff
	}
	if next > c.maxBackoff {
		return c.maxBackoff
	}
	return next
}

func (c *Collector) waitBackoff(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.sleep(delay)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

type queryDescriptor struct {
	ResourceID string
	Namespace  string
	MetricName string
}

func buildMetricQueries(targets []ResourceTarget) ([]types.MetricDataQuery, map[string]queryDescriptor) {
	queries := make([]types.MetricDataQuery, 0, len(targets)*3)
	descriptors := make(map[string]queryDescriptor, len(targets)*3)

	queryIndex := 0
	for _, target := range targets {
		metrics, resourceID := metricSpecsForTarget(target)
		for _, metric := range metrics {
			queryIndex++
			queryID := fmt.Sprintf("m%d", queryIndex)

			metricStat := &types.MetricStat{
				Metric: &types.Metric{
					Namespace:  &metric.namespace,
					MetricName: &metric.name,
					Dimensions: metric.dimensions,
				},
				Period: awsInt32(60),
				Stat:   awsString("Average"),
			}

			queries = append(queries, types.MetricDataQuery{
				Id:         &queryID,
				MetricStat: metricStat,
				ReturnData: awsBool(true),
			})

			descriptors[queryID] = queryDescriptor{
				ResourceID: resourceID,
				Namespace:  metric.namespace,
				MetricName: metric.name,
			}
		}
	}

	return queries, descriptors
}

type metricSpec struct {
	namespace  string
	name       string
	dimensions []types.Dimension
}

func metricSpecsForTarget(target ResourceTarget) ([]metricSpec, string) {
	switch target.Kind {
	case ResourceKindEC2:
		if target.InstanceID == "" {
			return nil, ""
		}
		dimensions := []types.Dimension{
			{
				Name:  awsString("InstanceId"),
				Value: &target.InstanceID,
			},
		}
		return []metricSpec{
			{namespace: "AWS/EC2", name: "CPUUtilization", dimensions: dimensions},
			{namespace: "AWS/EC2", name: "NetworkIn", dimensions: dimensions},
			{namespace: "AWS/EC2", name: "NetworkOut", dimensions: dimensions},
		}, target.InstanceID

	case ResourceKindECSService:
		if target.ClusterName == "" || target.ServiceName == "" {
			return nil, ""
		}
		dimensions := []types.Dimension{
			{
				Name:  awsString("ClusterName"),
				Value: &target.ClusterName,
			},
			{
				Name:  awsString("ServiceName"),
				Value: &target.ServiceName,
			},
		}
		resourceID := target.ClusterName + "/" + target.ServiceName
		return []metricSpec{
			{namespace: "AWS/ECS", name: "CPUUtilization", dimensions: dimensions},
			{namespace: "AWS/ECS", name: "MemoryUtilization", dimensions: dimensions},
		}, resourceID

	default:
		return nil, ""
	}
}

func metricPointsFromOutput(output *cloudwatch.GetMetricDataOutput, descriptors map[string]queryDescriptor) []MetricPoint {
	if output == nil {
		return nil
	}

	points := make([]MetricPoint, 0)
	for _, result := range output.MetricDataResults {
		if result.Id == nil {
			continue
		}
		descriptor, ok := descriptors[*result.Id]
		if !ok {
			continue
		}

		limit := len(result.Values)
		if len(result.Timestamps) < limit {
			limit = len(result.Timestamps)
		}

		for i := 0; i < limit; i++ {
			points = append(points, MetricPoint{
				ResourceID: descriptor.ResourceID,
				Namespace:  descriptor.Namespace,
				MetricName: descriptor.MetricName,
				Timestamp:  result.Timestamps[i].UTC(),
				Value:      result.Values[i],
			})
		}
	}

	return points
}

func isThrottlingError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	code := strings.ToLower(apiErr.ErrorCode())
	return strings.Contains(code, "throttl")
}

func awsString(v string) *string {
	return &v
}

func awsInt32(v int32) *int32 {
	return &v
}

func awsBool(v bool) *bool {
	return &v
}
