# Mole Makefile: local builds only.
#
#   make build      # build the console SPA, then the mole binary
#   make build-go   # rebuild only the Go binary, reusing pkg/dashboard/dist
#   make help       # show targets and variables
#
# The binary is written to the repository root: ./mole (or ./mole.exe).

GO   ?= go
PNPM ?= pnpm

FRONTEND_DIR   := web/frontend
DASHBOARD_DIST := pkg/dashboard/dist
CMD_DIR        := cmd/mole

GOOS   := $(shell $(GO) env GOOS)
EXT    := $(if $(filter windows,$(GOOS)),.exe)
BINARY := mole$(EXT)

# Build metadata injected into pkg/config.
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_COMMIT ?= $(shell git rev-parse --short=8 HEAD 2>/dev/null || echo dev)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_VERSION ?= $(shell $(GO) env GOVERSION)

CONFIG_PKG := github.com/kiry163/mole/pkg/config
LDFLAGS := -X $(CONFIG_PKG).Version=$(VERSION) -X $(CONFIG_PKG).GitCommit=$(GIT_COMMIT) -X $(CONFIG_PKG).BuildTime=$(BUILD_TIME) -X $(CONFIG_PKG).GoVersion=$(GO_VERSION) -s -w
GO_BUILD = $(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) ./$(CMD_DIR)

.DEFAULT_GOAL := build
.PHONY: build build-go help

## build: Build the console SPA and the mole binary (default)
build:
	@echo "Building console..."
	@cd $(FRONTEND_DIR) && $(PNPM) install --frozen-lockfile
	@cd $(FRONTEND_DIR) && $(PNPM) run build:backend
	@echo "Building $(BINARY)..."
	@$(GO_BUILD)
	@echo "Built ./$(BINARY)"

## build-go: Build only the mole binary, reusing the existing console dist
build-go:
	@if [ ! -f $(DASHBOARD_DIST)/index.html ]; then \
		echo "warning: $(DASHBOARD_DIST)/index.html is missing; the binary will run without a console UI. Run 'make build' first."; \
	fi
	@echo "Building $(BINARY)..."
	@$(GO_BUILD)
	@echo "Built ./$(BINARY)"

## help: Show this help message
help:
	@echo "mole Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | awk -F': ' '{printf "  %-12s %s\n", substr($$1, 4), $$2}'
	@echo ""
	@echo "Variables:"
	@echo "  VERSION       Version string (default: git describe)"
	@echo "  PNPM          pnpm executable (default: pnpm)"
	@echo "  GO            Go executable (default: go)"
	@echo ""
	@echo "Current configuration:"
	@echo "  Binary: $(BINARY)"
	@echo "  Version: $(VERSION)"
	@echo "  Frontend: $(FRONTEND_DIR) -> $(DASHBOARD_DIST)"
