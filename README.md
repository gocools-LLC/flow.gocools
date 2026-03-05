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
```

Set a custom listen address with `FLOW_HTTP_ADDR`:

```bash
FLOW_HTTP_ADDR=:9090 go run ./cmd/flow
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
- [Roadmap](docs/roadmap.md)
- [RFC-0001](docs/rfc/rfc-0001-platform.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
