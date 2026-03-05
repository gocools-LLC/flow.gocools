package cloudwatchlogs

import (
	"context"
	"errors"
	"strings"
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
	Parser         Parser
	Hooks          []ParseHook
}

type Client interface {
	FilterLogEvents(ctx context.Context, params *cloudwatchlogs.FilterLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error)
}

type Collector struct {
	client         Client
	maxAttempts    int
	initialBackoff time.Duration
	parser         Parser
	hooks          []ParseHook
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

	parser := cfg.Parser
	if parser == nil {
		parser = NewJSONParser()
	}

	hooks := append([]ParseHook{}, cfg.Hooks...)

	return &Collector{
		client:         client,
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		parser:         parser,
		hooks:          hooks,
		now:            time.Now,
		sleep:          time.Sleep,
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

		output, err := c.filterWithRetry(ctx, input)
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

func (c *Collector) filterWithRetry(ctx context.Context, input *cloudwatchlogs.FilterLogEventsInput) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	backoff := c.initialBackoff

	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		output, err := c.client.FilterLogEvents(ctx, input)
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
