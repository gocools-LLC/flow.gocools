package cloudwatchlogs

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/smithy-go"
)

type CollectRequest struct {
	LogGroupName  string
	FilterPattern string
	StartTime     time.Time
	EndTime       time.Time
	Limit         int32
}

type LogRecord struct {
	LogGroupName  string
	LogStreamName string
	EventID       string
	Timestamp     time.Time
	IngestionTime time.Time
	Message       string
	Level         string
	CorrelationID string
	Fields        map[string]string
	ParserError   string
}

type CollectorConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	MaxRetryBudget int
	JitterFraction float64
	Parser         Parser
	Hooks          []ParseHook
	Now            func() time.Time
	Sleep          func(time.Duration)
	RandFloat64    func() float64
}

type Client interface {
	FilterLogEvents(ctx context.Context, params *cloudwatchlogs.FilterLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error)
}

type Collector struct {
	client         Client
	maxAttempts    int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	maxRetryBudget int
	jitterFraction float64
	parser         Parser
	hooks          []ParseHook
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

var ErrRetryBudgetExhausted = errors.New("cloudwatch logs retry budget exhausted")

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

	parser := cfg.Parser
	if parser == nil {
		parser = NewJSONParser()
	}

	hooks := append([]ParseHook{}, cfg.Hooks...)

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
		parser:         parser,
		hooks:          hooks,
		now:            now,
		sleep:          sleep,
		randFloat64:    randFloat64,
	}
}

func (c *Collector) Collect(ctx context.Context, req CollectRequest) ([]LogRecord, error) {
	if req.LogGroupName == "" {
		return nil, errors.New("log group name is required")
	}

	endTime := req.EndTime.UTC()
	if endTime.IsZero() {
		endTime = c.now().UTC()
	}

	startTime := req.StartTime.UTC()
	if startTime.IsZero() {
		startTime = endTime.Add(-5 * time.Minute)
	}

	if startTime.After(endTime) {
		return nil, errors.New("start time must be before end time")
	}

	records := make([]LogRecord, 0)
	var nextToken string
	seenToken := map[string]struct{}{}
	retryState := retryState{
		remainingBudget: c.maxRetryBudget,
	}

	for {
		input := &cloudwatchlogs.FilterLogEventsInput{
			LogGroupName: awsString(req.LogGroupName),
			StartTime:    awsInt64(startTime.UnixMilli()),
			EndTime:      awsInt64(endTime.UnixMilli()),
		}
		if req.FilterPattern != "" {
			input.FilterPattern = awsString(req.FilterPattern)
		}
		if req.Limit > 0 {
			input.Limit = awsInt32(req.Limit)
		}
		if nextToken != "" {
			input.NextToken = awsString(nextToken)
		}

		output, err := c.filterWithRetry(ctx, input, &retryState)
		if err != nil {
			return nil, err
		}

		records = append(records, c.toLogRecords(req.LogGroupName, output.Events)...)

		if output.NextToken == nil || *output.NextToken == "" {
			break
		}

		nextToken = *output.NextToken
		if _, exists := seenToken[nextToken]; exists {
			break
		}
		seenToken[nextToken] = struct{}{}
	}

	return records, nil
}

func (c *Collector) toLogRecords(logGroupName string, events []types.FilteredLogEvent) []LogRecord {
	records := make([]LogRecord, 0, len(events))

	for _, event := range events {
		message := derefString(event.Message)
		parsed := c.parser.Parse(message)
		for _, hook := range c.hooks {
			hook(message, &parsed)
		}

		record := LogRecord{
			LogGroupName:  logGroupName,
			LogStreamName: derefString(event.LogStreamName),
			EventID:       derefString(event.EventId),
			Message:       message,
			Level:         parsed.Level,
			CorrelationID: parsed.CorrelationID,
			Fields:        parsed.Fields,
			ParserError:   parsed.ParseError,
		}

		if event.Timestamp != nil {
			record.Timestamp = time.UnixMilli(*event.Timestamp).UTC()
		}
		if event.IngestionTime != nil {
			record.IngestionTime = time.UnixMilli(*event.IngestionTime).UTC()
		}

		records = append(records, record)
	}

	return records
}

func (c *Collector) filterWithRetry(ctx context.Context, input *cloudwatchlogs.FilterLogEventsInput, state *retryState) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	backoff := c.initialBackoff

	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		output, err := c.client.FilterLogEvents(ctx, input)
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

func isThrottlingError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	code := strings.ToLower(apiErr.ErrorCode())
	return strings.Contains(code, "throttl")
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func awsString(v string) *string {
	return &v
}

func awsInt64(v int64) *int64 {
	return &v
}

func awsInt32(v int32) *int32 {
	return &v
}
