.PHONY: build test lint release clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
EXE :=
ifeq ($(OS),Windows_NT)
EXE := .exe
endif

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o docxcraft$(EXE) ./cmd/docxcraft

test:
	go test ./...

lint:
	golangci-lint run ./...

# Cross-compile release archives into dist/ for GitHub Releases.
# Usage: make release VERSION=v0.0.1
release:
	@mkdir -p dist
	@set -e; \
	for target in \
		linux/amd64 \
		linux/arm64 \
		darwin/amd64 \
		darwin/arm64 \
		windows/amd64 \
		windows/arm64; do \
		os=$${target%/*}; \
		arch=$${target#*/}; \
		ext=""; \
		archive_ext="tar.gz"; \
		if [ "$$os" = "windows" ]; then ext=".exe"; archive_ext="zip"; fi; \
		out="dist/docxcraft-$(VERSION)-$$os-$$arch$$ext"; \
		echo "Building $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o "$$out" ./cmd/docxcraft; \
		if [ "$$archive_ext" = "zip" ]; then \
			( cd dist && zip -q "docxcraft-$(VERSION)-$$os-$$arch.zip" "docxcraft-$(VERSION)-$$os-$$arch$$ext" ); \
		else \
			tar -C dist -czf "dist/docxcraft-$(VERSION)-$$os-$$arch.tar.gz" "docxcraft-$(VERSION)-$$os-$$arch$$ext"; \
		fi; \
		rm -f "$$out"; \
	done
	@ls -la dist/

clean:
	rm -rf dist docxcraft docxcraft.exe
