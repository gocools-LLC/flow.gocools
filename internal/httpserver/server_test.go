package httpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/analyzer/timeline"
	"github.com/gocools-LLC/flow.gocools/internal/correlation"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestHealthzPayload(t *testing.T) {
	handler := New(Config{
		Version: "test-version",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	if got := res.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content type application/json, got %q", got)
	}

	expected := "{\"service\":\"flow\",\"status\":\"ok\",\"version\":\"test-version\"}\n"
	if got := res.Body.String(); got != expected {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestReadyzPayload(t *testing.T) {
	handler := New(Config{
		Version: "test-version",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	expected := "{\"service\":\"flow\",\"status\":\"ready\",\"version\":\"test-version\"}\n"
	if got := res.Body.String(); got != expected {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestHealthzMethodNotAllowed(t *testing.T) {
	handler := New(Config{
		Version: "test-version",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler

	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, res.Code)
	}

	expected := "{\"error\":\"method not allowed\"}\n"
	if got := res.Body.String(); got != expected {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestIncidentTimelineEndpointSupportsTimeRangeAndDeterministicOrder(t *testing.T) {
	timelineService := timeline.NewInMemoryService([]timeline.Event{
		{
			ID:        "event-b",
			Timestamp: time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
			Severity:  timeline.SeverityError,
			Message:   "error b",
		},
		{
			ID:        "event-a",
			Timestamp: time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
			Severity:  timeline.SeverityError,
			Message:   "error a",
		},
		{
			ID:        "event-c",
			Timestamp: time.Date(2026, 3, 5, 9, 50, 0, 0, time.UTC),
			Severity:  timeline.SeverityInfo,
			Message:   "outside range",
		},
	})

	handler := New(Config{
		Version:         "test-version",
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		TimelineService: timelineService,
	}).Handler

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/incidents/timeline?start=2026-03-05T09:59:00Z&end=2026-03-05T10:01:00Z&page=1&page_size=10",
		nil,
	)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload timeline.Page
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if payload.Total != 2 {
		t.Fatalf("expected total 2, got %d", payload.Total)
	}
	if len(payload.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(payload.Events))
	}
	if payload.Events[0].ID != "event-a" || payload.Events[1].ID != "event-b" {
		t.Fatalf("unexpected event ordering: [%s %s]", payload.Events[0].ID, payload.Events[1].ID)
	}
}

func TestIncidentTimelineEndpointBadStartReturns400(t *testing.T) {
	handler := New(Config{
		Version: "test-version",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/timeline?start=bad-time", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
}

type fakeCorrelationService struct {
	result correlation.Result
	err    error
	query  correlation.Query
	calls  int
}

func (f *fakeCorrelationService) QueryGraph(query correlation.Query) (correlation.Result, error) {
	f.calls++
	f.query = query
	if f.err != nil {
		return correlation.Result{}, f.err
	}
	return f.result, nil
}

func TestTelemetryCorrelationEndpointReturnsGraphPayload(t *testing.T) {
	correlationService := &fakeCorrelationService{
		result: correlation.Result{
			Nodes: []correlation.Node{
				{ID: "resource:i-123", Kind: "resource"},
				{ID: "log:event-1", Kind: "log"},
			},
			Edges: []correlation.Edge{
				{From: "resource:i-123", To: "log:event-1", Kind: "emits_log"},
			},
			MetricCount: 3,
			LogCount:    2,
		},
	}

	handler := New(Config{
		Version:            "test-version",
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		CorrelationService: correlationService,
	}).Handler

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/telemetry/correlation?start=2026-03-05T10:00:00Z&end=2026-03-05T10:05:00Z&max_skew_seconds=120&resource_id=i-123&limit_nodes=10&limit_edges=20",
		nil,
	)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if correlationService.calls != 1 {
		t.Fatalf("expected one service call, got %d", correlationService.calls)
	}
	if correlationService.query.MaxSkew != 120*time.Second {
		t.Fatalf("expected max skew 120s, got %s", correlationService.query.MaxSkew)
	}
	if correlationService.query.ResourceID != "i-123" {
		t.Fatalf("expected resource filter i-123, got %q", correlationService.query.ResourceID)
	}
	if correlationService.query.LimitNodes != 10 || correlationService.query.LimitEdges != 20 {
		t.Fatalf("unexpected limit params: nodes=%d edges=%d", correlationService.query.LimitNodes, correlationService.query.LimitEdges)
	}

	var payload correlation.Result
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if payload.MetricCount != 3 || payload.LogCount != 2 {
		t.Fatalf("unexpected counts in payload: %+v", payload)
	}
	if len(payload.Nodes) != 2 || len(payload.Edges) != 1 {
		t.Fatalf("unexpected graph payload size: nodes=%d edges=%d", len(payload.Nodes), len(payload.Edges))
	}
}

func TestTelemetryCorrelationEndpointInvalidMaxSkewReturns400(t *testing.T) {
	handler := New(Config{
		Version: "test-version",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/correlation?max_skew_seconds=bad", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
}

func TestTelemetryCorrelationEndpointInvalidLimitsReturn400(t *testing.T) {
	handler := New(Config{
		Version: "test-version",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/correlation?limit_nodes=bad", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/correlation?limit_edges=bad", nil)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
}

func TestMetricsEndpointExposesHTTPMetrics(t *testing.T) {
	handler := New(Config{
		Version: "test-version",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRes := httptest.NewRecorder()
	handler.ServeHTTP(healthRes, healthReq)

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRes := httptest.NewRecorder()
	handler.ServeHTTP(metricsRes, metricsReq)

	if metricsRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, metricsRes.Code)
	}

	body := metricsRes.Body.String()
	if !strings.Contains(body, "flow_http_requests_total") {
		t.Fatalf("expected metrics output to contain flow_http_requests_total, got: %s", body)
	}
}

func TestMetricsEndpointNormalizesPathCardinality(t *testing.T) {
	handler := New(Config{
		Version: "test-version",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler

	requests := []string{
		"/api/v1/resources/550e8400-e29b-41d4-a716-446655440000",
		"/api/v1/resources/550e8400-e29b-41d4-a716-446655440001",
	}
	for _, path := range requests {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRes := httptest.NewRecorder()
	handler.ServeHTTP(metricsRes, metricsReq)

	body := metricsRes.Body.String()
	if !strings.Contains(body, `route="/api/v1/resources/:id"`) {
		t.Fatalf("expected normalized route label in metrics output, got: %s", body)
	}
	if strings.Contains(body, "550e8400-e29b-41d4-a716-446655440000") || strings.Contains(body, "550e8400-e29b-41d4-a716-446655440001") {
		t.Fatalf("expected raw IDs to be absent from metrics output, got: %s", body)
	}
}

func TestRequestTracingUsesNormalizedRouteAttribute(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
	)

	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(previous)
	defer provider.Shutdown(t.Context())

	handler := New(Config{
		Version: "test-version",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}).Handler

	req := httptest.NewRequest(http.MethodGet, "/api/v1/resources/550e8400-e29b-41d4-a716-446655440000", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	spans := recorder.Ended()
	if len(spans) == 0 {
		t.Fatal("expected at least one ended span")
	}

	attrs := spans[len(spans)-1].Attributes()
	if route, ok := attributeValue(attrs, "http.route"); !ok || route != "/api/v1/resources/:id" {
		t.Fatalf("expected normalized http.route attribute, got %q (exists=%v)", route, ok)
	}
	if _, ok := attributeValue(attrs, "http.path"); ok {
		t.Fatal("expected http.path attribute to be omitted by default")
	}
}

func attributeValue(attrs []attribute.KeyValue, key string) (string, bool) {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsString(), true
		}
	}
	return "", false
}
