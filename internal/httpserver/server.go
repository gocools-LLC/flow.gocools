package httpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/analyzer/timeline"
)

type Config struct {
	Addr            string
	Version         string
	Logger          *slog.Logger
	TimelineService timeline.Service
}

type statusResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func New(cfg Config) *http.Server {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	addr := cfg.Addr
	if addr == "" {
		addr = ":8080"
	}

	version := cfg.Version
	if version == "" {
		version = "dev"
	}

	timelineService := cfg.TimelineService
	if timelineService == nil {
		timelineService = timeline.NewInMemoryService(nil)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", statusHandler(version, "ok"))
	mux.HandleFunc("/readyz", statusHandler(version, "ready"))
	mux.HandleFunc("/api/v1/incidents/timeline", incidentTimelineHandler(timelineService))

	return &http.Server{
		Addr:              addr,
		Handler:           requestLogMiddleware(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func incidentTimelineHandler(service timeline.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}

		start, err := parseRFC3339Query(r, "start")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid start query parameter"})
			return
		}
		end, err := parseRFC3339Query(r, "end")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid end query parameter"})
			return
		}

		page, err := parseIntQuery(r, "page")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid page query parameter"})
			return
		}
		pageSize, err := parseIntQuery(r, "page_size")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid page_size query parameter"})
			return
		}

		result, err := service.QueryTimeline(timeline.Query{
			Start:    start,
			End:      end,
			Page:     page,
			PageSize: pageSize,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

func parseRFC3339Query(r *http.Request, key string) (time.Time, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func parseIntQuery(r *http.Request, key string) (int, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return 0, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, strconv.ErrSyntax
	}
	return parsed, nil
}

func statusHandler(version string, status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}

		writeJSON(w, http.StatusOK, statusResponse{
			Service: "flow",
			Status:  status,
			Version: version,
		})
	}
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func requestLogMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		status := rec.statusCode
		if status == 0 {
			status = http.StatusOK
		}

		logger.Info(
			"http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
