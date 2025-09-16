# Makefile for Manticore Search Tester

# Variables
BINARY_NAME=manticore-search-tester
BINARY_PATH=./bin/$(BINARY_NAME)
MAIN_PATH=./cmd/server
DOCKER_COMPOSE_FILE=docker-compose.yml

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
BUILD_FLAGS=-ldflags="-s -w"

.PHONY: all build clean test run dev docker-up docker-down docker-logs help

# Default target
all: clean build

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_PATH) $(MAIN_PATH)
	@echo "Build complete: $(BINARY_PATH)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf bin/
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run the application
run: build
	@echo "Starting $(BINARY_NAME)..."
	$(BINARY_PATH)

# Run in development mode (with auto-restart on file changes)
dev:
	@echo "Starting development server..."
	@if command -v air > /dev/null; then \
		air -c .air.toml; \
	else \
		echo "Air not found. Install with: go install github.com/cosmtrek/air@latest"; \
		echo "Running without auto-restart..."; \
		$(MAKE) run; \
	fi

# Test API endpoints
test-api: build
	@echo "Testing API endpoints..."
	$(BINARY_PATH) test-api

# Docker commands
docker-up:
	@echo "Starting Docker services..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) up -d
	@echo "Docker services started"

docker-down:
	@echo "Stopping Docker services..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) down
	@echo "Docker services stopped"

docker-logs:
	@echo "Showing Docker logs..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f

docker-logs-app:
	@echo "Showing application logs..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f web-service

docker-logs-manticore:
	@echo "Showing Manticore logs..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f manticore

# Build and run in Docker
docker-build:
	@echo "Building Docker image..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) build
	@echo "Docker image built"

docker-rebuild:
	@echo "Rebuilding Docker services..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) up -d --build
	@echo "Docker services rebuilt and started"

# Test in Docker
docker-test:
	@echo "Testing application in Docker..."
	@echo "Waiting for services to be ready..."
	@sleep 10
	@echo "Testing status endpoint..."
	curl -f http://localhost:8080/api/status || (echo "Status check failed" && exit 1)
	@echo "Testing search endpoint..."
	curl -f "http://localhost:8080/api/search?query=сайт&mode=basic" || (echo "Search test failed" && exit 1)
	@echo "Docker tests passed! ✅"

# Start full development environment
dev-full: docker-up
	@echo "Waiting for Manticore to start..."
	@sleep 5
	@$(MAKE) dev

# Stop full development environment
dev-stop: docker-down

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies installed"

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...
	@echo "Code formatted"

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Build for multiple platforms
build-all: clean
	@echo "Building for multiple platforms..."
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o bin/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "Multi-platform build complete"

# Create release archive
release: build-all
	@echo "Creating release archives..."
	@mkdir -p releases
	@cd bin && tar -czf ../releases/$(BINARY_NAME)-linux-amd64.tar.gz $(BINARY_NAME)-linux-amd64
	@cd bin && tar -czf ../releases/$(BINARY_NAME)-darwin-amd64.tar.gz $(BINARY_NAME)-darwin-amd64
	@cd bin && tar -czf ../releases/$(BINARY_NAME)-darwin-arm64.tar.gz $(BINARY_NAME)-darwin-arm64
	@cd bin && zip ../releases/$(BINARY_NAME)-windows-amd64.zip $(BINARY_NAME)-windows-amd64.exe
	@echo "Release archives created in releases/"

# Install development tools
install-tools:
	@echo "Installing development tools..."
	$(GOCMD) install github.com/cosmtrek/air@latest
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Development tools installed"

# Show help
help:
	@echo "Available commands:"
	@echo ""
	@echo "Local Development:"
	@echo "  build       - Build the application"
	@echo "  clean       - Clean build artifacts"
	@echo "  test        - Run tests"
	@echo "  run         - Build and run the application"
	@echo "  dev         - Run in development mode with auto-restart"
	@echo "  test-api    - Test API endpoints"
	@echo ""
	@echo "Docker Commands:"
	@echo "  docker-up   - Start Docker services (Manticore + App)"
	@echo "  docker-down - Stop Docker services"
	@echo "  docker-logs - Show all Docker logs"
	@echo "  docker-logs-app - Show application logs"
	@echo "  docker-logs-manticore - Show Manticore logs"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-rebuild - Rebuild and restart Docker services"
	@echo "  docker-test - Test application in Docker"
	@echo ""
	@echo "Development Environment:"
	@echo "  dev-full    - Start full development environment (Docker + dev server)"
	@echo "  dev-stop    - Stop full development environment"
	@echo ""
	@echo "Code Quality:"
	@echo "  deps        - Install dependencies"
	@echo "  fmt         - Format code"
	@echo "  lint        - Lint code"
	@echo ""
	@echo "Release:"
	@echo "  build-all   - Build for multiple platforms"
	@echo "  release     - Create release archives"
	@echo ""
	@echo "Tools:"
	@echo "  install-tools - Install development tools"
	@echo "  help        - Show this help message"