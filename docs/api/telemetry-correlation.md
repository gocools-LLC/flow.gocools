# Telemetry Correlation API

## Endpoint

`GET /api/v1/telemetry/correlation`

## Query Parameters

- `start` (optional, RFC3339 UTC): inclusive start timestamp
- `end` (optional, RFC3339 UTC): inclusive end timestamp
- `max_skew_seconds` (optional, int >= 0): correlation tolerance in seconds (default graph behavior applies when omitted)
- `resource_id` (optional): restrict graph to a single resolved resource ID
- `limit_nodes` (optional, int >= 0): cap returned node count
- `limit_edges` (optional, int >= 0): cap returned edge count

## Response

Status `200 OK`:

```json
{
  "nodes": [
    {
      "id": "resource:i-123",
      "kind": "resource",
      "attributes": {
        "resource_id": "i-123"
      }
    }
  ],
  "edges": [
    {
      "from": "resource:i-123",
      "to": "metric:i-123:CPUUtilization:1741178400000:0",
      "kind": "emits_metric"
    }
  ],
  "metric_count": 10,
  "log_count": 24
}
```

Status `400 Bad Request`:

```json
{
  "error": "invalid max_skew_seconds query parameter"
}
```

Possible validation errors:

- `invalid limit_nodes query parameter`
- `invalid limit_edges query parameter`

## Runtime Input

This endpoint reads from the in-memory telemetry signal buffer that is populated by
runtime ingestion (`FLOW_INGEST_MODE=cloudwatch_logs`, `cloudwatch_metrics`, or `cloudwatch_all`).

For metric+log correlations, use `FLOW_INGEST_MODE=cloudwatch_all`.
Buffer size can be tuned with `FLOW_SIGNAL_MAX_METRICS` and `FLOW_SIGNAL_MAX_LOGS`.
