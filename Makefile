.PHONY: run build test tidy db-up db-down docker-up docker-down build-lambda package-lambda

# Run locally
run:
	go run ./cmd/api

# Build server binary
build:
	mkdir -p bin
	go build -o bin/server ./cmd/api

# Build AWS Lambda bootstrap binary (arm64 by default for AWS Graviton)
build-lambda:
	mkdir -p bin
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/lambda

# Package AWS Lambda zip for deployment
package-lambda: build-lambda
	cd bin && zip -j deployment.zip bootstrap

# Run tests
test:
	go test -v ./...

# Tidy dependencies
tidy:
	go mod tidy

# Start only PostgreSQL container in background
db-up:
	docker compose up -d postgres

# Stop PostgreSQL container
db-down:
	docker compose stop postgres

# Run entire stack in Docker
docker-up:
	docker compose up --build -d

# Stop entire stack
docker-down:
	docker compose down
