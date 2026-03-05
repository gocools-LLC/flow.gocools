package cloudwatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
}

type Client interface {
	GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

type Collector struct {
	client         Client
	maxAttempts    int
	initialBackoff time.Duration
	now            func() time.Time
	sleep          func(time.Duration)
}

func NewCollector(client Client, cfg CollectorConfig) *Collector {
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	initialBackoff := cfg.InitialBackoff
	if initialBackoff <= 0 {
		initialBackoff = 250 * time.Millisecond
	}

	return &Collector{
		client:         client,
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		now:            time.Now,
		sleep:          time.Sleep,
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

	output, err := c.getMetricDataWithRetry(ctx, input)
	if err != nil {
		return nil, err
	}

	return metricPointsFromOutput(output, descriptors), nil
}

func (c *Collector) getMetricDataWithRetry(ctx context.Context, input *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
	backoff := c.initialBackoff

	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		output, err := c.client.GetMetricData(ctx, input)
		if err == nil {
			return output, nil
		}

		if !isThrottlingError(err) || attempt == c.maxAttempts {
			return nil, err
		}

		c.sleep(backoff)
		backoff *= 2
	}

	return nil, errors.New("unreachable retry state")
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
