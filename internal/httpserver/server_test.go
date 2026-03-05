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
