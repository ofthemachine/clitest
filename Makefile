VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test test-integration test-session lint fmt

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o clitest ./cmd/clitest/

test:
	go test ./...

test-integration:
	go test -tags=integration -v ./tests/...

test-session: build
	@chmod +x bin/test-session.sh
	@./bin/test-session.sh $(DIR)

lint: fmt
	go vet ./...

fmt:
	gofmt -w .

install:
	CGO_ENABLED=0 go install -trimpath -ldflags="$(LDFLAGS)" ./cmd/clitest/
