SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  = -s -w -X github.com/pdparchitect/rook/internal/version.Version=$(VERSION)

CMD       = rook
PLATFORMS = linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

GOOS   ?= $(shell go env GOHOSTOS)
GOARCH ?= $(shell go env GOHOSTARCH)

.PHONY: help build dev run test race vet fmt lint clean dist cross

# Listing the targets rather than assuming one: rook has two build variants that
# differ in what the binary may read from disk, and picking the wrong one
# silently is exactly the confusion this avoids.
help:
	@echo "rook - an AI bug-hunting harness"
	@echo
	@echo "  make build      Build ./rook for release ($(GOOS)/$(GOARCH))"
	@echo "  make dev        Build ./rook for development - see below"
	@echo "  make test       Run the test suite"
	@echo "  make race       Run the test suite under the race detector"
	@echo "  make vet        Run go vet over both build variants"
	@echo "  make fmt        Format the tree"
	@echo "  make lint       Alias for vet"
	@echo "  make cross      Cross-compile: make cross GOOS=darwin GOARCH=arm64"
	@echo "  make dist       Build release archives for every platform under dist/"
	@echo "  make clean      Remove built binaries and dist/"
	@echo
	@echo "build vs dev: a release binary does NOT read a .env from its working"
	@echo "  directory; a developer build does. rook runs shell commands against"
	@echo "  targets with a provider key in the process, so a released binary must"
	@echo "  not take credentials from whatever directory it was pointed at. Both"
	@echo "  write to ./rook - run './rook --version' to see which one you have."
	@echo
	@echo "Overrides: VERSION=$(VERSION)"
	@echo "           GOOS=$(GOOS) GOARCH=$(GOARCH)"

build:
	@echo "Building $(CMD) ($(VERSION), release)..."
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(CMD) ./cmd/$(CMD)

# A developer build. The only difference is that it reads a .env from the
# working directory, which a released binary must never do - pointing rook at a
# target's checkout would otherwise be enough to load whatever credentials are
# lying around in it.
dev:
	@echo "Building $(CMD) ($(VERSION), dev - reads .env)..."
	CGO_ENABLED=0 go build -trimpath -tags dev -ldflags "$(LDFLAGS)" -o $(CMD) ./cmd/$(CMD)

run: build
	./$(CMD) $(ARGS)

fmt:
	go fmt ./...

test:
	go test ./... -count=1

race:
	go test -race ./... -count=1

# Both variants, because a build tag can break a compile the default never
# reaches - and the developer build is the one no pipeline exercises.
vet:
	go vet ./...
	go vet -tags dev ./...

lint: vet
	@echo "lint ok"

clean:
	rm -f $(CMD)
	rm -rf dist

# Cross-compile a single platform: make cross GOOS=darwin GOARCH=arm64
cross:
	@echo "Building $(CMD) ($(VERSION)) for $(GOOS)/$(GOARCH)..."
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o $(CMD) ./cmd/$(CMD)

# Build release archives for every target platform under dist/.
dist: clean
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		out="dist/$(CMD)-$(VERSION)-$$os-$$arch"; \
		echo "Building $$out..."; \
		mkdir -p "$$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o "$$out/$(CMD)$$ext" ./cmd/$(CMD); \
		cp README.md LICENSE "$$out/"; \
		tar -czf "dist/$(CMD)-$(VERSION)-$$os-$$arch.tar.gz" -C dist "$(CMD)-$(VERSION)-$$os-$$arch"; \
	done
	@cd dist && sha256sum *.tar.gz > checksums.txt
	@echo "Release archives written to dist/"
