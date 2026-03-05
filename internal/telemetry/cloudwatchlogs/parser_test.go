package cloudwatchlogs

import "testing"

func TestJSONParserExtractsCorrelationAndLevel(t *testing.T) {
	parser := NewJSONParser()
	parsed := parser.Parse(`{"level":"error","correlation_id":"req-123","message":"boom","status":500}`)

	if parsed.ParseError != "" {
		t.Fatalf("unexpected parse error: %s", parsed.ParseError)
	}
	if parsed.Level != "error" {
		t.Fatalf("expected level error, got %q", parsed.Level)
	}
	if parsed.CorrelationID != "req-123" {
		t.Fatalf("expected correlation id req-123, got %q", parsed.CorrelationID)
	}
	if parsed.Fields["status"] != "500" {
		t.Fatalf("expected status field to be 500, got %q", parsed.Fields["status"])
	}
}

func TestJSONParserMalformedInputDoesNotPanic(t *testing.T) {
	parser := NewJSONParser()
	parsed := parser.Parse(`{"level":"info"`)

	if parsed.ParseError == "" {
		t.Fatal("expected parse error for malformed json")
	}
}

func TestCorrelationIDRegexHook(t *testing.T) {
	parsed := ParsedRecord{}
	hook := CorrelationIDRegexHook()

	hook("request failed request_id=req-abc-9 timeout", &parsed)
	if parsed.CorrelationID != "req-abc-9" {
		t.Fatalf("expected regex correlation id req-abc-9, got %q", parsed.CorrelationID)
	}
}
