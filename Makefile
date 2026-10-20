.PHONY: build test lint

build:
	CGO_ENABLED=0 go build -o docxforge$(EXE) ./cmd/docxforge

test:
	go test -race ./...

lint:
	@echo "golangci-lint not wired yet; run: golangci-lint run"
