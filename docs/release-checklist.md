# Flow v0.1.0 Release Checklist

## Versioning Policy

Flow follows Semantic Versioning:

- `MAJOR`: incompatible API changes
- `MINOR`: backward-compatible features
- `PATCH`: backward-compatible fixes

Initial milestone release target: `v0.1.0`.

## Pre-Release Validation

- [ ] `go test ./...`
- [ ] `make test-integration`
- [ ] `go build ./...`
- [ ] verify `/healthz` and `/readyz` on local run
- [ ] verify `/api/v1/incidents/timeline` response contract

## Release Artifacts

- Linux amd64 binary
- Darwin arm64 binary
- Windows amd64 binary

GitHub Actions workflow: `.github/workflows/release.yml`.

## Tagging Procedure

1. Ensure branch `main` is green.
2. Update changelog/release notes.
3. Tag release:

   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

4. Verify workflow artifact upload.

## Post-Release

- [ ] publish release notes
- [ ] verify downloadable artifacts
- [ ] create `v0.1.1` patch milestone for follow-ups

