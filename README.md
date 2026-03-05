# Flow

Live infrastructure telemetry and analysis platform for cloud and platform teams.

## Features
- live infrastructure telemetry
- CloudWatch metrics and logs correlation
- infrastructure flow visualization
- incident debugging and health monitoring

## Quick Start

```bash
go run ./cmd/flow
curl -s localhost:8080/healthz
curl -s localhost:8080/readyz
curl -s localhost:8080/metrics
```

Set a custom listen address with `FLOW_HTTP_ADDR`:

```bash
FLOW_HTTP_ADDR=:9090 go run ./cmd/flow
```

Use STS AssumeRole-based AWS auth (optional runtime validation):

```bash
FLOW_AWS_REGION=us-east-1 \
FLOW_AWS_ROLE_ARN=arn:aws:iam::123456789012:role/flow-observer \
FLOW_AWS_SESSION_NAME=flow-session \
FLOW_AWS_VALIDATE_ON_START=true \
go run ./cmd/flow
```

Run the ingestion integration harness:

```bash
make test-integration
```

## Repository Layout

- `cmd/flow`: CLI entrypoint.
- `internal/`: internal application logic.
- `pkg/`: reusable packages.
- `docs/`: architecture, roadmap, and RFCs.

## Security Model

- AWS STS AssumeRole (no permanent access keys)
- least-privilege IAM roles
- tag-based ownership and safe destructive operations

## Required Resource Tags

```text
gocools:stack-id
gocools:environment
gocools:owner
```

## Documentation

- [Architecture](docs/architecture.md)
- [Telemetry Correlation Model](docs/telemetry-correlation-model.md)
- [Telemetry Storage](docs/telemetry-storage.md)
- [Incident Timeline API](docs/api/incident-timeline.md)
- [Service Observability](docs/observability.md)
- [Release Checklist](docs/release-checklist.md)
- [Release Notes v0.1.0](docs/releases/v0.1.0.md)
- [Roadmap](docs/roadmap.md)
- [RFC-0001](docs/rfc/rfc-0001-platform.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
