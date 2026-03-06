BINARY := flow

.PHONY: build test test-integration smoke-local run fmt lint

build:
	go build ./...

test:
	go test ./...

test-integration:
	go test -v ./test/...

smoke-local:
	./scripts/smoke-local.sh

run:
	go run ./cmd/$(BINARY)

fmt:
	gofmt -w ./cmd ./internal ./pkg

lint:
	go vet ./...
