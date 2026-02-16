APP      := ai-hub
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_AT := $(shell date '+%Y-%m-%d %H:%M:%S')

# Default: current platform
GOOS     ?= $(shell go env GOOS)
GOARCH   ?= $(shell go env GOARCH)
OUTPUT   ?= $(APP)-$(GOOS)-$(GOARCH)

LDFLAGS  := -s -w -X 'main.Version=$(VERSION)' -X 'main.BuildAt=$(BUILD_AT)'

# CGO is required for SQLite
export CGO_ENABLED := 1

# Cross-compile C compilers
CC_darwin_arm64   := cc
CC_linux_amd64    := x86_64-linux-musl-gcc
CC_windows_amd64  := x86_64-w64-mingw32-gcc

# Pick the right CC for target
export CC := $(or $(CC_$(GOOS)_$(GOARCH)),cc)

.PHONY: all frontend build clean release help

## Build for current platform (default)
all: frontend build

## Build frontend
frontend:
	@echo "==> Building frontend..."
	@cd web && npm run build
	@rm -f web/dist/.gitkeep

## Build Go binary (assumes frontend already built)
build:
	@echo "==> Building $(OUTPUT) ($(GOOS)/$(GOARCH))..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o dist/$(OUTPUT)$(if $(filter windows,$(GOOS)),.exe) .
	@echo "==> dist/$(OUTPUT)$(if $(filter windows,$(GOOS)),.exe)"

## Cross-compile for all platforms
release: frontend
	@mkdir -p dist
	@echo "==> Building darwin/arm64..."
	@GOOS=darwin GOARCH=arm64 CC=$(CC_darwin_arm64) go build -ldflags "$(LDFLAGS)" -o dist/$(APP)-darwin-arm64 .
	@echo "==> Building linux/amd64..."
	@GOOS=linux GOARCH=amd64 CC=$(CC_linux_amd64) go build -ldflags "$(LDFLAGS)" -o dist/$(APP)-linux-amd64 .
	@echo "==> Building windows/amd64..."
	@GOOS=windows GOARCH=amd64 CC=$(CC_windows_amd64) go build -ldflags "$(LDFLAGS)" -o dist/$(APP)-windows-amd64.exe .
	@echo ""
	@echo "==> Release binaries:"
	@ls -lh dist/

## Clean build artifacts
clean:
	@rm -rf dist web/dist

## Show help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all       Build frontend + binary for current platform (default)"
	@echo "  frontend  Build frontend only"
	@echo "  build     Build Go binary only (frontend must exist)"
	@echo "  release   Build frontend + all platform binaries (macOS/Linux/Windows)"
	@echo "  clean     Remove build artifacts"
	@echo ""
	@echo "Cross-compile single target:"
	@echo "  make build GOOS=linux GOARCH=amd64"
	@echo ""
	@echo "Cross-compile dependencies:"
	@echo "  Linux:   brew install FiloSottile/musl-cross/musl-cross"
	@echo "  Windows: brew install mingw-w64"
