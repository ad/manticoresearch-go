#!/bin/bash

# AI Search Test Runner
# Comprehensive test suite for AI search functionality

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
VERBOSE=${VERBOSE:-false}
COVERAGE=${COVERAGE:-true}
BENCHMARK=${BENCHMARK:-false}
INTEGRATION=${INTEGRATION:-true}
FRONTEND=${FRONTEND:-false}

# Directories
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COVERAGE_DIR="${PROJECT_ROOT}/coverage"
TEST_RESULTS_DIR="${PROJECT_ROOT}/test-results"

echo -e "${BLUE}=== AI Search Comprehensive Test Suite ===${NC}"
echo "Project root: ${PROJECT_ROOT}"
echo "Coverage: ${COVERAGE}"
echo "Benchmarks: ${BENCHMARK}"
echo "Integration tests: ${INTEGRATION}"
echo "Frontend tests: ${FRONTEND}"
echo ""

# Create directories
mkdir -p "${COVERAGE_DIR}"
mkdir -p "${TEST_RESULTS_DIR}"

# Function to run tests with coverage
run_tests_with_coverage() {
    local test_pattern="$1"
    local test_name="$2"
    local output_file="${COVERAGE_DIR}/${test_name}.out"
    
    echo -e "${YELLOW}Running ${test_name} tests...${NC}"
    
    if [ "$VERBOSE" = "true" ]; then
        go test -v -race -coverprofile="${output_file}" "${test_pattern}" | tee "${TEST_RESULTS_DIR}/${test_name}.log"
    else
        go test -race -coverprofile="${output_file}" "${test_pattern}" > "${TEST_RESULTS_DIR}/${test_name}.log" 2>&1
    fi
    
    local exit_code=$?
    
    if [ $exit_code -eq 0 ]; then
        echo -e "${GREEN}✅ ${test_name} tests passed${NC}"
        if [ "$COVERAGE" = "true" ] && [ -f "${output_file}" ]; then
            local coverage=$(go tool cover -func="${output_file}" | grep total | awk '{print $3}')
            echo -e "${BLUE}   Coverage: ${coverage}${NC}"
        fi
    else
        echo -e "${RED}❌ ${test_name} tests failed${NC}"
        if [ "$VERBOSE" = "false" ]; then
            echo "Last 10 lines of output:"
            tail -n 10 "${TEST_RESULTS_DIR}/${test_name}.log"
        fi
        return $exit_code
    fi
}

# Function to run benchmarks
run_benchmarks() {
    local test_pattern="$1"
    local test_name="$2"
    
    echo -e "${YELLOW}Running ${test_name} benchmarks...${NC}"
    
    local benchmark_file="${TEST_RESULTS_DIR}/${test_name}_benchmark.txt"
    go test -bench=. -benchmem "${test_pattern}" > "${benchmark_file}" 2>&1
    
    local exit_code=$?
    
    if [ $exit_code -eq 0 ]; then
        echo -e "${GREEN}✅ ${test_name} benchmarks completed${NC}"
        echo "Results saved to: ${benchmark_file}"
        
        # Show summary of benchmark results
        if command -v benchstat >/dev/null 2>&1; then
            echo -e "${BLUE}Benchmark summary:${NC}"
            benchstat "${benchmark_file}" | head -n 20
        fi
    else
        echo -e "${RED}❌ ${test_name} benchmarks failed${NC}"
        return $exit_code
    fi
}

# Function to check test prerequisites
check_prerequisites() {
    echo -e "${YELLOW}Checking prerequisites...${NC}"
    
    # Check Go version
    if ! command -v go >/dev/null 2>&1; then
        echo -e "${RED}❌ Go is not installed${NC}"
        exit 1
    fi
    
    local go_version=$(go version | awk '{print $3}' | sed 's/go//')
    echo -e "${GREEN}✅ Go version: ${go_version}${NC}"
    
    # Check if we're in the right directory
    if [ ! -f "${PROJECT_ROOT}/go.mod" ]; then
        echo -e "${RED}❌ Not in a Go module directory${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✅ Go module found${NC}"
    
    # Check for required test files
    local required_files=(
        "internal/models/ai_config_comprehensive_test.go"
        "internal/search/engine_ai_comprehensive_test.go"
        "internal/handlers/ai_error_comprehensive_test.go"
        "internal/integration/ai_search_integration_test.go"
    )
    
    for file in "${required_files[@]}"; do
        if [ ! -f "${PROJECT_ROOT}/${file}" ]; then
            echo -e "${RED}❌ Required test file not found: ${file}${NC}"
            exit 1
        fi
    done
    
    echo -e "${GREEN}✅ All required test files found${NC}"
    echo ""
}

# Function to run frontend tests
run_frontend_tests() {
    echo -e "${YELLOW}Running frontend AI search tests...${NC}"
    
    local frontend_test_file="${PROJECT_ROOT}/static/test/ai-search-ui-test.html"
    
    if [ ! -f "${frontend_test_file}" ]; then
        echo -e "${RED}❌ Frontend test file not found: ${frontend_test_file}${NC}"
        return 1
    fi
    
    echo -e "${GREEN}✅ Frontend test file found${NC}"
    echo -e "${BLUE}   Open ${frontend_test_file} in a browser to run frontend tests${NC}"
    
    # If we have a headless browser available, we could run the tests automatically
    if command -v chromium-browser >/dev/null 2>&1 || command -v google-chrome >/dev/null 2>&1; then
        echo -e "${BLUE}   Detected Chrome/Chromium - frontend tests could be automated${NC}"
    fi
    
    return 0
}

# Function to generate coverage report
generate_coverage_report() {
    echo -e "${YELLOW}Generating coverage report...${NC}"
    
    local coverage_files=($(find "${COVERAGE_DIR}" -name "*.out" -type f))
    
    if [ ${#coverage_files[@]} -eq 0 ]; then
        echo -e "${YELLOW}⚠️  No coverage files found${NC}"
        return 0
    fi
    
    # Merge coverage files
    local merged_coverage="${COVERAGE_DIR}/merged.out"
    echo "mode: set" > "${merged_coverage}"
    
    for file in "${coverage_files[@]}"; do
        if [ -f "${file}" ]; then
            tail -n +2 "${file}" >> "${merged_coverage}"
        fi
    done
    
    # Generate HTML report
    local html_report="${COVERAGE_DIR}/coverage.html"
    go tool cover -html="${merged_coverage}" -o "${html_report}"
    
    # Generate text summary
    local total_coverage=$(go tool cover -func="${merged_coverage}" | grep total | awk '{print $3}')
    
    echo -e "${GREEN}✅ Coverage report generated${NC}"
    echo -e "${BLUE}   Total coverage: ${total_coverage}${NC}"
    echo -e "${BLUE}   HTML report: ${html_report}${NC}"
    echo -e "${BLUE}   Merged coverage: ${merged_coverage}${NC}"
}

# Function to display test summary
display_summary() {
    echo ""
    echo -e "${BLUE}=== Test Summary ===${NC}"
    
    local total_tests=0
    local passed_tests=0
    local failed_tests=0
    
    # Count test results
    for log_file in "${TEST_RESULTS_DIR}"/*.log; do
        if [ -f "${log_file}" ]; then
            total_tests=$((total_tests + 1))
            if grep -q "PASS" "${log_file}"; then
                passed_tests=$((passed_tests + 1))
            else
                failed_tests=$((failed_tests + 1))
            fi
        fi
    done
    
    echo "Total test suites: ${total_tests}"
    echo -e "Passed: ${GREEN}${passed_tests}${NC}"
    echo -e "Failed: ${RED}${failed_tests}${NC}"
    
    if [ $failed_tests -eq 0 ]; then
        echo -e "${GREEN}🎉 All tests passed!${NC}"
    else
        echo -e "${RED}💥 Some tests failed${NC}"
    fi
    
    echo ""
    echo "Test artifacts:"
    echo "  - Test logs: ${TEST_RESULTS_DIR}/"
    echo "  - Coverage reports: ${COVERAGE_DIR}/"
    
    if [ "$FRONTEND" = "true" ]; then
        echo "  - Frontend tests: ${PROJECT_ROOT}/static/test/ai-search-ui-test.html"
    fi
}

# Main execution
main() {
    cd "${PROJECT_ROOT}"
    
    # Check prerequisites
    check_prerequisites
    
    local exit_code=0
    
    # 1. AI Configuration Tests
    echo -e "${BLUE}=== 1. AI Configuration Management Tests ===${NC}"
    if ! run_tests_with_coverage "./internal/models" "ai-config"; then
        exit_code=1
    fi
    echo ""
    
    # 2. AI Search Engine Tests
    echo -e "${BLUE}=== 2. AI Search Engine Tests ===${NC}"
    if ! run_tests_with_coverage "./internal/search" "ai-search-engine"; then
        exit_code=1
    fi
    echo ""
    
    # 3. AI Error Handling Tests
    echo -e "${BLUE}=== 3. AI Error Handling Tests ===${NC}"
    if ! run_tests_with_coverage "./internal/handlers" "ai-error-handling"; then
        exit_code=1
    fi
    echo ""
    
    # 4. AI Manticore Client Tests
    echo -e "${BLUE}=== 4. AI Manticore Client Tests ===${NC}"
    if ! run_tests_with_coverage "./internal/manticore" "ai-manticore-client"; then
        exit_code=1
    fi
    echo ""
    
    # 5. Integration Tests
    if [ "$INTEGRATION" = "true" ]; then
        echo -e "${BLUE}=== 5. AI Search Integration Tests ===${NC}"
        if ! run_tests_with_coverage "./internal/integration" "ai-integration"; then
            exit_code=1
        fi
        echo ""
    fi
    
    # 6. Benchmarks
    if [ "$BENCHMARK" = "true" ]; then
        echo -e "${BLUE}=== 6. AI Search Benchmarks ===${NC}"
        run_benchmarks "./internal/models" "ai-config-benchmark"
        run_benchmarks "./internal/search" "ai-search-engine-benchmark"
        run_benchmarks "./internal/manticore" "ai-manticore-client-benchmark"
        if [ "$INTEGRATION" = "true" ]; then
            run_benchmarks "./internal/integration" "ai-integration-benchmark"
        fi
        echo ""
    fi
    
    # 7. Frontend Tests
    if [ "$FRONTEND" = "true" ]; then
        echo -e "${BLUE}=== 7. AI Search Frontend Tests ===${NC}"
        if ! run_frontend_tests; then
            exit_code=1
        fi
        echo ""
    fi
    
    # 8. Generate Coverage Report
    if [ "$COVERAGE" = "true" ]; then
        generate_coverage_report
        echo ""
    fi
    
    # Display summary
    display_summary
    
    return $exit_code
}

# Handle command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        --no-coverage)
            COVERAGE=false
            shift
            ;;
        -b|--benchmark)
            BENCHMARK=true
            shift
            ;;
        --no-integration)
            INTEGRATION=false
            shift
            ;;
        -f|--frontend)
            FRONTEND=true
            shift
            ;;
        -h|--help)
            echo "AI Search Test Runner"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  -v, --verbose       Enable verbose output"
            echo "  --no-coverage       Disable coverage reporting"
            echo "  -b, --benchmark     Run benchmarks"
            echo "  --no-integration    Skip integration tests"
            echo "  -f, --frontend      Run frontend tests"
            echo "  -h, --help          Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  VERBOSE=true        Enable verbose output"
            echo "  COVERAGE=false      Disable coverage reporting"
            echo "  BENCHMARK=true      Run benchmarks"
            echo "  INTEGRATION=false   Skip integration tests"
            echo "  FRONTEND=true       Run frontend tests"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use -h or --help for usage information"
            exit 1
            ;;
    esac
done

# Run main function
if main; then
    echo -e "${GREEN}🎉 AI Search test suite completed successfully!${NC}"
    exit 0
else
    echo -e "${RED}💥 AI Search test suite failed!${NC}"
    exit 1
fi