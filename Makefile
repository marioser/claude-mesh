DIST := dist

# Host platform binaries.
BRIDGE := $(DIST)/claude-mesh-bridge
MCP    := $(DIST)/claude-mesh-mcp

# Cross-build targets: darwin first (primary target), then linux for future cross-platform.
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64

.PHONY: test test-race test-integration test-e2e build crossbuild clean

## test: run all unit tests.
test:
	go test ./...

## test-race: run all tests with the race detector.
test-race:
	go test -race ./...

## test-integration: run integration tests (requires mosquitto + redis containers).
test-integration:
	go test -tags integration ./internal/bridge/...

## test-e2e: run end-to-end shell tests.
test-e2e:
	bash tests/e2e/three-sessions.sh

## build: compile both binaries for the host platform into dist/.
build:
	mkdir -p $(DIST)
	go build -o $(BRIDGE) .
	go build -o $(MCP) ./cmd/claude-mesh-mcp

## crossbuild: compile both binaries for all supported platforms into dist/<os>-<arch>/.
crossbuild:
	$(foreach PLATFORM,$(PLATFORMS), \
		$(eval GOOS   := $(word 1,$(subst /, ,$(PLATFORM)))) \
		$(eval GOARCH := $(word 2,$(subst /, ,$(PLATFORM)))) \
		$(eval OUTDIR := $(DIST)/$(GOOS)-$(GOARCH)) \
		mkdir -p $(OUTDIR) && \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(OUTDIR)/claude-mesh-bridge . && \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(OUTDIR)/claude-mesh-mcp ./cmd/claude-mesh-mcp && \
	) true

## clean: remove the dist/ directory.
clean:
	rm -rf $(DIST)
