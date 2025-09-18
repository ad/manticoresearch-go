#!/bin/bash

# Script to compare benchmark results between different implementations or versions

set -e

if [ $# -lt 2 ]; then
    echo "Usage: $0 <benchmark_file_1> <benchmark_file_2>"
    echo "Example: $0 benchmark-results/old_implementation.txt benchmark-results/new_implementation.txt"
    exit 1
fi

file1="$1"
file2="$2"

if [ ! -f "$file1" ]; then
    echo "Error: Benchmark file '$file1' not found"
    exit 1
fi

if [ ! -f "$file2" ]; then
    echo "Error: Benchmark file '$file2' not found"
    exit 1
fi

echo "Comparing benchmark results:"
echo "File 1: $file1"
echo "File 2: $file2"
echo "================================="
echo ""

# Function to extract benchmark metrics
extract_metrics() {
    local file="$1"
    local benchmark_name="$2"
    
    grep -E "^$benchmark_name" "$file" | head -1 | awk '{
        for(i=1; i<=NF; i++) {
            if($i ~ /ns\/op/) {
                gsub(/ns\/op/, "", $(i-1))
                print $(i-1)
                break
            }
        }
    }'
}

# Compare key benchmarks
benchmarks=(
    "BenchmarkIndexSingleDocument"
    "BenchmarkIndexBulkDocuments100"
    "BenchmarkSearchBasic"
    "BenchmarkSearchMatchAll"
    "BenchmarkConcurrentSearch"
)

echo "Performance Comparison (ns/op):"
echo "Benchmark                     | File 1      | File 2      | Improvement"
echo "-------------------------------|-------------|-------------|------------"

for benchmark in "${benchmarks[@]}"; do
    metric1=$(extract_metrics "$file1" "$benchmark")
    metric2=$(extract_metrics "$file2" "$benchmark")
    
    if [ -n "$metric1" ] && [ -n "$metric2" ]; then
        # Calculate improvement percentage
        improvement=$(echo "scale=2; (($metric1 - $metric2) / $metric1) * 100" | bc -l 2>/dev/null || echo "N/A")
        
        printf "%-30s | %-11s | %-11s | %s%%\n" "$benchmark" "$metric1" "$metric2" "$improvement"
    else
        printf "%-30s | %-11s | %-11s | N/A\n" "$benchmark" "${metric1:-N/A}" "${metric2:-N/A}"
    fi
done

echo ""
echo "Memory Usage Comparison:"
echo "Benchmark                     | File 1 (B/op) | File 2 (B/op) | Improvement"
echo "-------------------------------|----------------|----------------|------------"

for benchmark in "${benchmarks[@]}"; do
    mem1=$(grep -E "^$benchmark" "$file1" | head -1 | awk '{
        for(i=1; i<=NF; i++) {
            if($i ~ /B\/op/) {
                gsub(/B\/op/, "", $(i-1))
                print $(i-1)
                break
            }
        }
    }')
    
    mem2=$(grep -E "^$benchmark" "$file2" | head -1 | awk '{
        for(i=1; i<=NF; i++) {
            if($i ~ /B\/op/) {
                gsub(/B\/op/, "", $(i-1))
                print $(i-1)
                break
            }
        }
    }')
    
    if [ -n "$mem1" ] && [ -n "$mem2" ]; then
        improvement=$(echo "scale=2; (($mem1 - $mem2) / $mem1) * 100" | bc -l 2>/dev/null || echo "N/A")
        printf "%-30s | %-14s | %-14s | %s%%\n" "$benchmark" "$mem1" "$mem2" "$improvement"
    else
        printf "%-30s | %-14s | %-14s | N/A\n" "$benchmark" "${mem1:-N/A}" "${mem2:-N/A}"
    fi
done

echo ""
echo "Custom Metrics Comparison:"

# Extract custom metrics like docs/sec
echo "Throughput (docs/sec):"
grep -E "docs/sec" "$file1" | while read -r line; do
    benchmark=$(echo "$line" | awk '{print $1}')
    value1=$(echo "$line" | grep -oE '[0-9]+\.[0-9]+.*docs/sec' | grep -oE '[0-9]+\.[0-9]+')
    
    value2=$(grep -E "^$benchmark" "$file2" | grep -oE '[0-9]+\.[0-9]+.*docs/sec' | grep -oE '[0-9]+\.[0-9]+' | head -1)
    
    if [ -n "$value1" ] && [ -n "$value2" ]; then
        improvement=$(echo "scale=2; (($value2 - $value1) / $value1) * 100" | bc -l 2>/dev/null || echo "N/A")
        printf "%-30s | %-11s | %-11s | %s%%\n" "$benchmark" "$value1" "$value2" "$improvement"
    fi
done

echo ""
echo "Summary:"
echo "- Positive improvement percentages indicate better performance in File 2"
echo "- Negative improvement percentages indicate worse performance in File 2"
echo "- For throughput metrics (docs/sec), higher values are better"
echo "- For latency metrics (ns/op), lower values are better"
echo "- For memory metrics (B/op), lower values are better"