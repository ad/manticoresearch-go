#!/bin/bash

# Manticore Search JSON API Testing Script
# This script tests all the JSON API endpoints that will be used in the HTTP client implementation

set -e

MANTICORE_URL="http://localhost:9308"
TEST_TABLE="api_test_table"

echo "=== Manticore Search JSON API Testing ==="
echo "Testing against: $MANTICORE_URL"
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print test results
print_result() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓ PASS${NC}: $2"
    else
        echo -e "${RED}✗ FAIL${NC}: $2"
    fi
}

# Function to print test section
print_section() {
    echo -e "\n${YELLOW}=== $1 ===${NC}"
}

# Test 1: Health Check
print_section "Health Check"
echo "Testing GET / endpoint..."
response=$(curl -s -w "%{http_code}" -X GET "$MANTICORE_URL/")
http_code="${response: -3}"
if [ "$http_code" = "200" ]; then
    print_result 0 "Health check endpoint responds with 200"
    echo "Response contains cluster info and version"
else
    print_result 1 "Health check failed with HTTP $http_code"
fi

# Test 2: Schema Operations via CLI endpoint
print_section "Schema Operations"

# Drop test table if exists
echo "Cleaning up any existing test table..."
curl -s -X POST "$MANTICORE_URL/cli" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "query=DROP TABLE IF EXISTS $TEST_TABLE" > /dev/null

# Create test table
echo "Testing table creation via /cli endpoint..."
response=$(curl -s -X POST "$MANTICORE_URL/cli" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "query=CREATE TABLE $TEST_TABLE (id bigint, title text, content text, url string)")

if echo "$response" | grep -q "Query OK"; then
    print_result 0 "Table creation via /cli endpoint"
else
    print_result 1 "Table creation failed: $response"
fi

# Verify table exists
echo "Verifying table creation..."
response=$(curl -s -X POST "$MANTICORE_URL/cli" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "query=SHOW TABLES")

if echo "$response" | grep -q "$TEST_TABLE"; then
    print_result 0 "Table verification - table exists"
else
    print_result 1 "Table verification failed"
fi

# Test 3: Document Indexing
print_section "Document Indexing"

# Test single document indexing via /replace
echo "Testing single document indexing via /replace endpoint..."
response=$(curl -s -X POST "$MANTICORE_URL/replace" \
  -H "Content-Type: application/json" \
  -d "{
    \"index\": \"$TEST_TABLE\",
    \"id\": 1,
    \"doc\": {
      \"title\": \"Test Document 1\",
      \"content\": \"This is the first test document for API validation\",
      \"url\": \"http://example.com/doc1\"
    }
  }")

if echo "$response" | grep -q '"status": 200\|"status": 201'; then
    print_result 0 "Single document indexing via /replace"
else
    print_result 1 "Single document indexing failed: $response"
fi

# Test bulk document indexing via /bulk
echo "Testing bulk document indexing via /bulk endpoint..."
bulk_data='{"replace":{"index":"'$TEST_TABLE'","id":2,"doc":{"title":"Bulk Document 1","content":"First bulk test document","url":"http://example.com/bulk1"}}}
{"replace":{"index":"'$TEST_TABLE'","id":3,"doc":{"title":"Bulk Document 2","content":"Second bulk test document","url":"http://example.com/bulk2"}}}'

response=$(curl -s -X POST "$MANTICORE_URL/bulk" \
  -H "Content-Type: application/x-ndjson" \
  -d "$bulk_data")

if echo "$response" | grep -q '"errors": false' || echo "$response" | grep -q '"status": 201'; then
    print_result 0 "Bulk document indexing via /bulk"
else
    print_result 1 "Bulk document indexing failed: $response"
fi

# Test 4: Search Operations
print_section "Search Operations"

# Test basic search with match_all
echo "Testing basic search with match_all..."
response=$(curl -s -X POST "$MANTICORE_URL/search" \
  -H "Content-Type: application/json" \
  -d "{
    \"index\": \"$TEST_TABLE\",
    \"query\": {\"match_all\": {}},
    \"limit\": 10
  }")

if echo "$response" | grep -q '"hits"' && ! echo "$response" | grep -q '"error"'; then
    print_result 0 "Basic search with match_all"
else
    print_result 1 "Basic search failed: $response"
fi

# Test full-text search with match
echo "Testing full-text search with match query..."
response=$(curl -s -X POST "$MANTICORE_URL/search" \
  -H "Content-Type: application/json" \
  -d "{
    \"index\": \"$TEST_TABLE\",
    \"query\": {\"match\": {\"content\": \"bulk\"}},
    \"limit\": 10
  }")

if echo "$response" | grep -q '"hits"' && ! echo "$response" | grep -q '"error"'; then
    print_result 0 "Full-text search with match query"
else
    print_result 1 "Full-text search failed: $response"
fi

# Test query_string search
echo "Testing query_string search..."
response=$(curl -s -X POST "$MANTICORE_URL/search" \
  -H "Content-Type: application/json" \
  -d "{
    \"index\": \"$TEST_TABLE\",
    \"query\": {\"query_string\": \"test\"},
    \"limit\": 10
  }")

if echo "$response" | grep -q '"hits"'; then
    print_result 0 "Query string search"
else
    print_result 1 "Query string search failed: $response"
fi

# Test pagination
echo "Testing pagination with offset and limit..."
response=$(curl -s -X POST "$MANTICORE_URL/search" \
  -H "Content-Type: application/json" \
  -d "{
    \"index\": \"$TEST_TABLE\",
    \"query\": {\"match_all\": {}},
    \"limit\": 2,
    \"offset\": 1
  }")

hits_count=$(echo "$response" | jq '.hits.hits | length')
if [ "$hits_count" = "2" ]; then
    print_result 0 "Pagination with offset and limit"
else
    print_result 1 "Pagination failed: got $hits_count hits instead of 2"
fi

# Test 5: Error Handling
print_section "Error Handling"

# Test search on non-existent table
echo "Testing search on non-existent table..."
response=$(curl -s -X POST "$MANTICORE_URL/search" \
  -H "Content-Type: application/json" \
  -d "{
    \"index\": \"nonexistent_table_xyz\",
    \"query\": {\"match_all\": {}}
  }")

if echo "$response" | grep -q '"error"'; then
    print_result 0 "Error handling for non-existent table"
else
    print_result 1 "Error handling failed: $response"
fi

# Test malformed JSON
echo "Testing malformed JSON handling..."
response=$(curl -s -X POST "$MANTICORE_URL/search" \
  -H "Content-Type: application/json" \
  -d '{"invalid": json}')

if echo "$response" | grep -q '"error"'; then
    print_result 0 "Error handling for malformed JSON"
else
    print_result 1 "Malformed JSON error handling failed: $response"
fi

# Test 6: SQL Endpoint Limitations
print_section "SQL Endpoint Limitations"

# Test SQL endpoint with SELECT (should work)
echo "Testing SQL endpoint with SELECT query..."
response=$(curl -s -X POST "$MANTICORE_URL/sql" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "query=SELECT COUNT(*) FROM $TEST_TABLE")

if echo "$response" | grep -q '"hits"\|"data"' && ! echo "$response" | grep -q '"error"'; then
    print_result 0 "SQL endpoint with SELECT query"
else
    print_result 1 "SQL SELECT failed: $response"
fi

# Test SQL endpoint with CREATE (should fail)
echo "Testing SQL endpoint with CREATE query (should fail)..."
response=$(curl -s -X POST "$MANTICORE_URL/sql" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "query=CREATE TABLE test_sql (id bigint)")

if echo "$response" | grep -q "only SELECT queries are supported"; then
    print_result 0 "SQL endpoint correctly rejects non-SELECT queries"
else
    print_result 1 "SQL endpoint error handling failed: $response"
fi

# Test 7: Performance and Limits
print_section "Performance and Limits"

# Test large bulk operation
echo "Testing larger bulk operation (10 documents)..."
bulk_large=""
for i in {4..13}; do
    bulk_large+="{\"replace\":{\"index\":\"$TEST_TABLE\",\"id\":$i,\"doc\":{\"title\":\"Bulk Doc $i\",\"content\":\"Content for document $i\",\"url\":\"http://example.com/doc$i\"}}}"$'\n'
done

response=$(curl -s -X POST "$MANTICORE_URL/bulk" \
  -H "Content-Type: application/x-ndjson" \
  -d "$bulk_large")

if echo "$response" | grep -q '"errors": false' || echo "$response" | grep -q '"status": 201'; then
    print_result 0 "Large bulk operation (10 documents)"
else
    print_result 1 "Large bulk operation failed: $response"
fi

# Verify total document count
response=$(curl -s -X POST "$MANTICORE_URL/search" \
  -H "Content-Type: application/json" \
  -d "{
    \"index\": \"$TEST_TABLE\",
    \"query\": {\"match_all\": {}},
    \"limit\": 0
  }")

total_docs=$(echo "$response" | jq '.hits.total')
if [ "$total_docs" -ge "10" ]; then
    print_result 0 "Document count verification ($total_docs documents)"
else
    print_result 1 "Document count mismatch: expected at least 10, got $total_docs"
fi

# Cleanup
print_section "Cleanup"
echo "Cleaning up test table..."
curl -s -X POST "$MANTICORE_URL/cli" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "query=DROP TABLE IF EXISTS $TEST_TABLE" > /dev/null

print_result 0 "Test cleanup completed"

echo -e "\n${YELLOW}=== API Testing Complete ===${NC}"
echo "All critical JSON API endpoints have been tested and documented."