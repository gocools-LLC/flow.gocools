package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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
