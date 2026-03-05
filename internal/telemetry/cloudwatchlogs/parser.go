package cloudwatchlogs

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type ParsedRecord struct {
	Level         string
	CorrelationID string
	Fields        map[string]string
	ParseError    string
}

type Parser interface {
	Parse(message string) ParsedRecord
}

type ParseHook func(rawMessage string, parsed *ParsedRecord)

type JSONParser struct{}

func NewJSONParser() JSONParser {
	return JSONParser{}
}

func (JSONParser) Parse(message string) ParsedRecord {
	parsed := ParsedRecord{
		Fields: map[string]string{},
	}

	raw := map[string]any{}
	if err := json.Unmarshal([]byte(message), &raw); err != nil {
		parsed.ParseError = err.Error()
		return parsed
	}

	for key, value := range raw {
		lowerKey := strings.ToLower(key)
		stringValue, ok := stringifyValue(value)
		if !ok {
			continue
		}

		parsed.Fields[lowerKey] = stringValue

		if parsed.Level == "" && isLevelKey(lowerKey) {
			parsed.Level = stringValue
		}
		if parsed.CorrelationID == "" && isCorrelationKey(lowerKey) {
			parsed.CorrelationID = stringValue
		}
	}

	return parsed
}

func CorrelationIDRegexHook() ParseHook {
	pattern := regexp.MustCompile(`(?i)(?:correlation[_-]?id|request[_-]?id|trace[_-]?id)[=: ]+([A-Za-z0-9._:/-]+)`)

	return func(rawMessage string, parsed *ParsedRecord) {
		if parsed.CorrelationID != "" {
			return
		}
		matches := pattern.FindStringSubmatch(rawMessage)
		if len(matches) < 2 {
			return
		}
		parsed.CorrelationID = matches[1]
	}
}

func isLevelKey(key string) bool {
	return key == "level" || key == "severity" || key == "lvl"
}

func isCorrelationKey(key string) bool {
	return key == "correlation_id" || key == "correlationid" || key == "request_id" || key == "requestid" || key == "trace_id" || key == "traceid"
}

func stringifyValue(v any) (string, bool) {
	switch value := v.(type) {
	case string:
		return value, true
	case bool:
		if value {
			return "true", true
		}
		return "false", true
	case float64:
		return fmt.Sprintf("%v", value), true
	default:
		return "", false
	}
}
