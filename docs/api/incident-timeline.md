# Incident Timeline API

## Endpoint

`GET /api/v1/incidents/timeline`

## Query Parameters

- `start` (optional, RFC3339 UTC): inclusive start timestamp
- `end` (optional, RFC3339 UTC): inclusive end timestamp
- `page` (optional, int >= 1, default `1`)
- `page_size` (optional, int >= 1, default `50`, max `200`)

## Response

Status `200 OK`:

```json
{
  "events": [
    {
      "id": "event-a",
      "timestamp": "2026-03-05T10:00:00Z",
      "severity": "error",
      "source": "ecs/service-a",
      "message": "request failed",
      "correlation_id": "req-123"
    }
  ],
  "page": 1,
  "page_size": 50,
  "total": 1
}
```

Status `400 Bad Request`:

```json
{
  "error": "invalid start query parameter"
}
```

## Ordering Rules

Returned events are deterministically ordered:

1. `timestamp` descending
2. `id` ascending when timestamps are equal

