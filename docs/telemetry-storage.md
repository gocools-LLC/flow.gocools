# Telemetry Storage Abstraction

Flow telemetry storage is modeled behind a backend-agnostic interface:

- `Put(key, data, metadata)`
- `Get(key)`
- `List(prefix)`
- `Delete(key)`
- `ApplyRetention(policy)`

## Backends

- `LocalStore`: filesystem-backed storage for local development and tests
- `S3Store`: S3-compatible object storage backend (AWS S3 and compatible APIs like R2)

## Retention Hooks

`ApplyRetention` supports:

- `MaxAge`: age-based candidate selection
- `Prefix`: scope retention by key namespace
- `DryRun`: report candidates without deletion
- `OnDeleteCandidate`: callback hook for audit/reporting pipelines

This allows safe preview of retention impact before destructive cleanup.

