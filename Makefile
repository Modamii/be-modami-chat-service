.PHONY: build run test lint clean dev infra infra-down proto

# Build the chat service binary
build:
	go build -o bin/chat-service ./cmd/

# Run the chat service locally
run: build
	./bin/chat-service

# Run with hot reload using air (install: go install github.com/air-verse/air@latest)
dev:
	air -c .air.toml

# Run all tests
test:
	go test -v -race -count=1 ./...

# Run tests with coverage
test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Lint using golangci-lint
lint:
	golangci-lint run ./...

# Start infrastructure services
infra:
	docker compose -f deployments/docker/docker-compose.yaml up -d mongodb redis kafka centrifugo minio

# Stop infrastructure services
infra-down:
	docker compose -f deployments/docker/docker-compose.yaml down

# Start everything including the chat service
up:
	docker compose -f deployments/docker/docker-compose.yaml up -d --build

# Stop everything
down:
	docker compose -f deployments/docker/docker-compose.yaml down -v

# Generate protobuf code
proto:
	protoc --go_out=. --go_opt=paths=source_relative api/proto/*.proto

# Tidy dependencies
tidy:
	go mod tidy

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html
