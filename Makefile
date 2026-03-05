BINARY := flow

.PHONY: build test test-integration run fmt lint

build:
	go build ./...

test:
	go test ./...

test-integration:
	go test -v ./test/...

run:
	go run ./cmd/$(BINARY)

fmt:
	gofmt -w ./cmd ./internal ./pkg

lint:
	go vet ./...
