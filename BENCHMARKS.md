# Performance Benchmarks

This document describes the performance benchmarks for the Manticore HTTP client implementation.

## Overview

The benchmarks are designed to measure the performance characteristics of the new HTTP-based Manticore client implementation, including:

- **Indexing Performance**: Single document and bulk document indexing
- **Search Performance**: Various search operations with different query types and result sizes
- **Memory Usage**: Memory allocation patterns during operations
- **Latency**: Response time characteristics
- **Concurrency**: Performance under concurrent load
- **Schema Operations**: Database schema management performance

## Running Benchmarks

### Prerequisites

1. Docker and docker-compose installed
2. Go 1.23+ installed
3. Manticore Search running (handled automatically by scripts)

### Quick Start

```bash
# Run all benchmarks with automatic Manticore setup
./scripts/run-benchmarks.sh
```

### Manual Benchmark Execution

```bash
# Start Manticore Search
docker-compose up -d manticore

# Set environment variables
export MANTICORE_BENCHMARK_TESTS=1
export MANTICORE_URL=http://localhost:9308

# Run specific benchmark categories
go test -v ./internal/manticore -bench=BenchmarkIndex -benchmem -benchtime=10s
go test -v ./internal/manticore -bench=BenchmarkSearch -benchmem -benchtime=10s
go test -v ./internal/manticore -bench=BenchmarkConcurrent -benchmem -benchtime=10s

# Stop Manticore Search
docker-compose down
```

## Benchmark Categories

### 1. Indexing Benchmarks

- `BenchmarkIndexSingleDocument`: Single document indexing with vector data
- `BenchmarkIndexSingleDocumentNoVector`: Single document indexing without vector data
- `BenchmarkIndexBulkDocuments10/50/100/500`: Bulk indexing with different batch sizes

**Key Metrics:**
- Operations per second (ops/sec)
- Documents per second (docs/sec)
- Memory allocations per operation (allocs/op)
- Bytes allocated per operation (B/op)

### 2. Search Benchmarks

- `BenchmarkSearchBasic`: Basic match query search
- `BenchmarkSearchFullText`: Full-text search with query_string
- `BenchmarkSearchMatchAll`: Match all documents query
- `BenchmarkSearchLimit10/50/100`: Search with different result limits

**Key Metrics:**
- Nanoseconds per operation (ns/op)
- Results per operation (results/op)
- Memory usage patterns

### 3. Schema Operation Benchmarks

- `BenchmarkCreateSchema`: Database schema creation
- `BenchmarkTruncateTables`: Table truncation operations

**Key Metrics:**
- Operation latency
- Memory overhead

### 4. Concurrency Benchmarks

- `BenchmarkConcurrentSearch`: Concurrent search operations
- `BenchmarkConcurrentIndexing`: Concurrent document indexing

**Key Metrics:**
- Throughput under concurrent load
- Resource utilization
- Contention characteristics

### 5. Memory Usage Benchmarks

- `BenchmarkMemoryUsageIndexing`: Memory allocation patterns during indexing
- `BenchmarkMemoryUsageSearch`: Memory allocation patterns during search

**Key Metrics:**
- Allocations per operation (allocs/op)
- Bytes per operation (bytes/op)
- Memory efficiency

### 6. Latency Benchmarks

- `BenchmarkLatencyLocal`: Latency characteristics for local connections

**Key Metrics:**
- Average latency (avg_latency_ms)
- Minimum latency (min_latency_ms)
- Maximum latency (max_latency_ms)

## Interpreting Results

### Performance Metrics

```
BenchmarkIndexSingleDocument-8    1000    1234567 ns/op    2048 B/op    15 allocs/op    850.5 docs/sec
```

- `BenchmarkIndexSingleDocument-8`: Benchmark name with GOMAXPROCS value
- `1000`: Number of iterations run
- `1234567 ns/op`: Nanoseconds per operation (lower is better)
- `2048 B/op`: Bytes allocated per operation (lower is better)
- `15 allocs/op`: Number of allocations per operation (lower is better)
- `850.5 docs/sec`: Custom metric showing throughput (higher is better)

### Performance Targets

Based on typical Manticore Search performance characteristics:

**Indexing Performance:**
- Single document: < 10ms per operation
- Bulk operations: > 1000 docs/sec
- Memory usage: < 5KB per document

**Search Performance:**
- Basic search: < 50ms per operation
- Full-text search: < 100ms per operation
- Memory usage: < 10KB per search

**Concurrency:**
- Should scale linearly with available CPU cores
- No significant performance degradation under concurrent load

## Comparing Implementations

Use the comparison script to compare benchmark results:

```bash
./scripts/compare-benchmarks.sh benchmark-results/old_impl.txt benchmark-results/new_impl.txt
```

This will show:
- Performance improvements/regressions
- Memory usage changes
- Throughput comparisons

## Benchmark Environment

For consistent results, benchmarks should be run in a controlled environment:

- Dedicated machine or container
- Consistent system load
- Same Manticore Search version
- Same Go version
- Multiple runs for statistical significance

## Continuous Benchmarking

Consider integrating benchmarks into CI/CD pipeline:

1. Run benchmarks on performance-critical changes
2. Compare results against baseline
3. Alert on significant performance regressions
4. Track performance trends over time

## Troubleshooting

### Common Issues

1. **Benchmarks are skipped**: Set `MANTICORE_BENCHMARK_TESTS=1`
2. **Connection failures**: Ensure Manticore is running on correct port
3. **Inconsistent results**: Run multiple times and average results
4. **Memory issues**: Increase available memory for large benchmarks

### Debug Mode

For detailed benchmark debugging:

```bash
go test -v ./internal/manticore -bench=BenchmarkName -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof
```

Analyze profiles with:
```bash
go tool pprof cpu.prof
go tool pprof mem.prof
```

## Contributing

When adding new benchmarks:

1. Follow existing naming conventions
2. Include appropriate setup and teardown
3. Use realistic data sizes
4. Document expected performance characteristics
5. Test on multiple environments