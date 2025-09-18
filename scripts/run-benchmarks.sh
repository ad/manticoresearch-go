#!/bin/bash

# Script to run performance benchmarks with Manticore Search using docker-compose

set -e

echo "Starting Manticore Search for benchmarking..."

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

echo "Running performance benchmarks..."

# Set environment variables
export MANTICORE_BENCHMARK_TESTS=1
export MANTICORE_URL=http://localhost:9308

# Create benchmark results directory
mkdir -p benchmark-results
timestamp=$(date +"%Y%m%d_%H%M%S")
benchmark_file="benchmark-results/benchmark_${timestamp}.txt"

echo "Benchmark Results - $(date)" > "$benchmark_file"
echo "=================================" >> "$benchmark_file"
echo "" >> "$benchmark_file"

# Run different benchmark categories
echo "Running indexing benchmarks..."
go test -v ./internal/manticore -bench=BenchmarkIndex -benchmem -benchtime=10s -timeout=10m | tee -a "$benchmark_file"

echo "" >> "$benchmark_file"
echo "Running search benchmarks..."
go test -v ./internal/manticore -bench=BenchmarkSearch -benchmem -benchtime=10s -timeout=10m | tee -a "$benchmark_file"

echo "" >> "$benchmark_file"
echo "Running schema operation benchmarks..."
go test -v ./internal/manticore -bench=BenchmarkCreateSchema -benchmem -benchtime=5s -timeout=5m | tee -a "$benchmark_file"
go test -v ./internal/manticore -bench=BenchmarkTruncateTables -benchmem -benchtime=5s -timeout=5m | tee -a "$benchmark_file"

echo "" >> "$benchmark_file"
echo "Running concurrent operation benchmarks..."
go test -v ./internal/manticore -bench=BenchmarkConcurrent -benchmem -benchtime=10s -timeout=10m | tee -a "$benchmark_file"

echo "" >> "$benchmark_file"
echo "Running memory usage benchmarks..."
go test -v ./internal/manticore -bench=BenchmarkMemoryUsage -benchmem -benchtime=5s -timeout=5m | tee -a "$benchmark_file"

echo "" >> "$benchmark_file"
echo "Running latency benchmarks..."
go test -v ./internal/manticore -bench=BenchmarkLatency -benchmem -benchtime=10s -timeout=5m | tee -a "$benchmark_file"

benchmark_result=$?

echo "Stopping Manticore Search..."
docker-compose down

echo ""
echo "Benchmark results saved to: $benchmark_file"
echo ""

if [ $benchmark_result -eq 0 ]; then
    echo "All benchmarks completed successfully!"
    
    # Show summary
    echo "=== BENCHMARK SUMMARY ==="
    echo "Indexing Performance:"
    grep -E "BenchmarkIndex.*docs/sec" "$benchmark_file" | head -5 || echo "No indexing benchmarks found"
    
    echo ""
    echo "Search Performance:"
    grep -E "BenchmarkSearch.*ns/op" "$benchmark_file" | head -5 || echo "No search benchmarks found"
    
    echo ""
    echo "Memory Usage:"
    grep -E "BenchmarkMemoryUsage.*allocs/op" "$benchmark_file" | head -3 || echo "No memory benchmarks found"
    
else
    echo "Some benchmarks failed"
    exit $benchmark_result
fi