BINARY := flow

.PHONY: build test run fmt lint

build:
	go build ./...

test:
	go test ./...

run:
	go run ./cmd/$(BINARY)

fmt:
	gofmt -w ./cmd ./internal ./pkg

lint:
	go vet ./...
