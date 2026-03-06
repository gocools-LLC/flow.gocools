package httpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gocools-LLC/flow.gocools/internal/analyzer/timeline"
	"github.com/gocools-LLC/flow.gocools/internal/correlation"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type Config struct {
	Addr               string
	Version            string
	Logger             *slog.Logger
	TimelineService    timeline.Service
	CorrelationService correlationQueryService
}

type correlationQueryService interface {
	QueryGraph(query correlation.Query) (correlation.Result, error)
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
	metricsRegistry := prometheus.NewRegistry()
	serverMetrics := newServerMetrics(metricsRegistry)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", statusHandler(version, "ok"))
	mux.HandleFunc("/readyz", statusHandler(version, "ready"))
	mux.HandleFunc("/api/v1/incidents/timeline", incidentTimelineHandler(timelineService))
	mux.HandleFunc("/api/v1/telemetry/correlation", telemetryCorrelationHandler(cfg.CorrelationService))
	mux.Handle("/metrics", promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}))

	return &http.Server{
		Addr:              addr,
		Handler:           requestLogMiddleware(logger, mux, serverMetrics),
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

func telemetryCorrelationHandler(service correlationQueryService) http.HandlerFunc {
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
		maxSkewSeconds, err := parseIntQuery(r, "max_skew_seconds")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid max_skew_seconds query parameter"})
			return
		}
		limitNodes, err := parseIntQuery(r, "limit_nodes")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid limit_nodes query parameter"})
			return
		}
		limitEdges, err := parseIntQuery(r, "limit_edges")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid limit_edges query parameter"})
			return
		}
		resourceID := r.URL.Query().Get("resource_id")

		maxSkew := time.Duration(0)
		if maxSkewSeconds > 0 {
			maxSkew = time.Duration(maxSkewSeconds) * time.Second
		}

		if service == nil {
			writeJSON(w, http.StatusOK, correlation.Result{
				Nodes: []correlation.Node{},
				Edges: []correlation.Edge{},
			})
			return
		}

		result, err := service.QueryGraph(correlation.Query{
			Start:      start,
			End:        end,
			MaxSkew:    maxSkew,
			ResourceID: resourceID,
			LimitNodes: limitNodes,
			LimitEdges: limitEdges,
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

func requestLogMiddleware(logger *slog.Logger, next http.Handler, metrics *serverMetrics) http.Handler {
	tracer := otel.Tracer("flow.http")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx, span := tracer.Start(r.Context(), r.Method+" "+r.URL.Path)
		route := normalizePathLabel(r.URL.Path)

		rec := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(ctx))

		status := rec.statusCode
		if status == 0 {
			status = http.StatusOK
		}
		duration := time.Since(start)

		metrics.observe(r.Method, route, status, duration)
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.route", route),
			attribute.Int("http.status_code", status),
			attribute.Int64("http.duration_ms", duration.Milliseconds()),
		)
		if status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
		span.End()

		logger.Info(
			"http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", duration.Milliseconds(),
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

func (r *responseRecorder) Write(p []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	return r.ResponseWriter.Write(p)
}
