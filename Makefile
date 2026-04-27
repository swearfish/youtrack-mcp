APP := youtrack-mcp
DIST := dist
MAIN := ./cmd/youtrack-mcp
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X youtrack-mcp/internal/version.Value=$(VERSION)
GO_BUILD_FLAGS := -trimpath
TARGETS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	windows/amd64 \
	windows/arm64

.PHONY: fmt test build cross-build clean

fmt:
	go fmt ./...

test:
	go test ./...

build:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(DIST)/$(APP) $(MAIN)

cross-build:
	@mkdir -p $(DIST)
	@for target in $(TARGETS); do \
		os=$${target%/*}; \
		arch=$${target#*/}; \
		output="$(DIST)/$(APP)-$${os}-$${arch}"; \
		if [ "$$os" = "windows" ]; then output="$$output.exe"; fi; \
		echo "Building $$output"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o "$$output" $(MAIN) || exit 1; \
	done

clean:
	rm -rf $(DIST)
