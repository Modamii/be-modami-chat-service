.PHONY: build run test test-coverage lint clean dev infra infra-down up down proto tidy swagger swagger-check

BINARY   := bin/chat-service
CMD_DIR  := ./cmd/server
DOCS_DIR := docs
SWAG     := $(shell go env GOPATH)/bin/swag

# ── Build ────────────────────────────────────────────────────────────────────

build:
	go build -o $(BINARY) $(CMD_DIR)

run: build
	./$(BINARY)

# Run with hot reload (install: go install github.com/air-verse/air@latest)
dev:
	air -c .air.toml

# ── Swagger ──────────────────────────────────────────────────────────────────

# Generate / refresh Swagger docs from godoc annotations.
# Run this whenever you add or change API annotations.
swagger: swagger-check
	$(SWAG) init \
		--generalInfo cmd/server/docs.go \
		--output $(DOCS_DIR) \
		--parseDependency \
		--parseInternal

# Verify swag CLI is installed; install it if missing.
swagger-check:
	@if [ ! -f "$(SWAG)" ]; then \
		echo "swag not found — installing github.com/swaggo/swag/cmd/swag@latest ..."; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
	fi

# ── Test ─────────────────────────────────────────────────────────────────────

test:
	go test -v -race -count=1 ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# ── Code quality ─────────────────────────────────────────────────────────────

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

# ── Infrastructure ───────────────────────────────────────────────────────────

infra:
	docker compose -f deployments/docker/docker-compose.yaml up -d scylladb redis kafka centrifugo

infra-down:
	docker compose -f deployments/docker/docker-compose.yaml down

up:
	docker compose -f deployments/docker/docker-compose.yaml up -d --build

down:
	docker compose -f deployments/docker/docker-compose.yaml down -v

# ── Misc ─────────────────────────────────────────────────────────────────────

proto:
	protoc --go_out=. --go_opt=paths=source_relative api/proto/*.proto

clean:
	rm -rf bin/ coverage.out coverage.html
