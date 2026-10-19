BIN_DIR ?= bin
GO ?= go

.PHONY: all build test test-race fmt fmt-check vet check clean

all: check build

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -o $(BIN_DIR)/scaleblade ./cmd/scaleblade
	$(GO) build -trimpath -o $(BIN_DIR)/scaleblade-mcp ./cmd/scaleblade-mcp

test:
	$(GO) test ./...

test-race:
	$(GO) test -race -shuffle=on ./...

fmt:
	gofmt -w .

fmt-check:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt needed for:"; echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...

check: fmt-check vet test-race build

clean:
	rm -rf $(BIN_DIR)
