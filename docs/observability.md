# Flow Service Observability

## Internal Metrics

Flow exposes Prometheus metrics at:

- `GET /metrics`

Current key metrics:

- `flow_http_requests_total{method,route,status}`
- `flow_http_request_duration_seconds{method,route,status}`

CloudWatch ingestion collectors also expose in-process retry metrics via `Collector.Metrics()`:

- `throttled_responses`
- `retry_attempts`
- `retry_budget_exceeded`
- `throttle_drops`

Timeline ingestion runtime toggles:

- `FLOW_INGEST_MODE=disabled` (default)
- `FLOW_INGEST_MODE=cloudwatch_logs`
- `FLOW_INGEST_MODE=cloudwatch_metrics`
- `FLOW_INGEST_MODE=cloudwatch_all` (logs + metrics in parallel)
- `FLOW_CW_LOG_GROUP=/aws/...` (required in `cloudwatch_logs` mode)
- `FLOW_CW_METRIC_TARGETS=ec2:i-...,ecs:<cluster>/<service>` (required in `cloudwatch_metrics` mode)
- `FLOW_CW_METRIC_UTIL_WARN` and `FLOW_CW_METRIC_UTIL_ERROR` for utilization thresholds
- `FLOW_INGEST_INTERVAL`, `FLOW_INGEST_WINDOW`, `FLOW_INGEST_TIMEOUT`
- `FLOW_SIGNAL_MAX_METRICS`, `FLOW_SIGNAL_MAX_LOGS` for in-memory correlation buffer capacity

## Tracing

Flow emits OpenTelemetry spans for inbound HTTP requests when configured:

```bash
FLOW_OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4318
FLOW_OTEL_INSECURE=true
```

Spans include attributes:

- `http.method`
- `http.route`
- `http.status_code`
- `http.duration_ms`

## Suggested Dashboard Panels

1. Request rate by route:
   `sum(rate(flow_http_requests_total[5m])) by (route)`
2. Error rate:
   `sum(rate(flow_http_requests_total{status=~"5.."}[5m])) / sum(rate(flow_http_requests_total[5m]))`
3. P95 latency:
   `histogram_quantile(0.95, sum(rate(flow_http_request_duration_seconds_bucket[5m])) by (le,route))`

## Trace Queries

- Find slow requests by route and filter spans where `http.duration_ms > 500`.
- Group failures by `http.path` with `http.status_code >= 500`.
