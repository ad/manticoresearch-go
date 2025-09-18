#!/bin/bash

# Script to run integration tests with Manticore Search using docker-compose

set -e

echo "Starting Manticore Search with docker-compose..."

# Check if docker-compose.yml exists
if [ ! -f "docker-compose.yml" ]; then
    echo "Error: docker-compose.yml not found in current directory"
    echo "Please run this script from the project root directory"
    exit 1
fi

# Start Manticore Search
docker-compose up -d manticore

echo "Waiting for Manticore Search to be ready..."
sleep 10

# Check if Manticore is responding
max_attempts=30
attempt=1
while [ $attempt -le $max_attempts ]; do
    if curl -s http://localhost:9308/ > /dev/null 2>&1; then
        echo "Manticore Search is ready!"
        break
    fi
    echo "Attempt $attempt/$max_attempts: Waiting for Manticore..."
    sleep 2
    attempt=$((attempt + 1))
done

if [ $attempt -gt $max_attempts ]; then
    echo "Error: Manticore Search failed to start within expected time"
    docker-compose logs manticore
    exit 1
fi

echo "Running integration tests..."

# Set environment variables and run tests
export MANTICORE_INTEGRATION_TESTS=1
export MANTICORE_URL=http://localhost:9308

# Run integration tests
go test -v ./internal/manticore -run TestIntegration -timeout 5m

test_result=$?

echo "Stopping Manticore Search..."
docker-compose down

if [ $test_result -eq 0 ]; then
    echo "All integration tests passed!"
else
    echo "Some integration tests failed"
    exit $test_result
fi