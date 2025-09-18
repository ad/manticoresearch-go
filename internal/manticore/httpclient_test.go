package manticore

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ad/manticoresearch-go/internal/models"
)

// Mock HTTP server for testing
func createMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// Test basic client creation and configuration
func TestNewHTTPClient(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config)

	if client == nil {
		t.Fatal("NewHTTPClient returned nil")
	}

	// Test that client implements the interface
	var _ ClientInterface = client
}

func TestDefaultHTTPClientConfig(t *testing.T) {
	baseURL := "http://localhost:9308"
	config := DefaultHTTPClientConfig(baseURL)

	if config.BaseURL != baseURL {
		t.Errorf("Expected BaseURL %s, got %s", baseURL, config.BaseURL)
	}

	if config.Timeout != 60*time.Second {
		t.Errorf("Expected Timeout 60s, got %v", config.Timeout)
	}

	if config.RetryConfig.MaxAttempts != 5 {
		t.Errorf("Expected MaxAttempts 5, got %d", config.RetryConfig.MaxAttempts)
	}
}

func TestCircuitBreakerIntegration(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config).(*manticoreHTTPClient)

	// Test that circuit breaker is integrated
	if client.circuitBreakerWithRetry == nil {
		t.Error("Circuit breaker with retry should be initialized")
	}

	// Test getting stats
	stats := client.circuitBreakerWithRetry.GetCircuitBreakerStats()
	if stats.State != CircuitBreakerClosed {
		t.Error("Circuit breaker should initially be in CLOSED state")
	}
}

func TestIsConnected(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config).(*manticoreHTTPClient)

	// Initially should not be connected
	if client.IsConnected() {
		t.Error("Client should not be connected initially")
	}

	// Simulate connection
	client.isConnected = true
	if !client.IsConnected() {
		t.Error("Client should be connected after setting isConnected to true")
	}
}

func TestClose(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config)

	err := client.Close()
	if err != nil {
		t.Errorf("Close should not return error, got: %v", err)
	}

	if client.IsConnected() {
		t.Error("Client should not be connected after Close()")
	}
}

func TestBulkConfig(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")

	// Test default bulk configuration
	if config.BulkConfig.BatchSize != 100 {
		t.Errorf("Expected default BatchSize 100, got %d", config.BulkConfig.BatchSize)
	}

	if config.BulkConfig.MaxConcurrentBatch != 3 {
		t.Errorf("Expected default MaxConcurrentBatch 3, got %d", config.BulkConfig.MaxConcurrentBatch)
	}

	if config.BulkConfig.StreamingThreshold != 1000 {
		t.Errorf("Expected default StreamingThreshold 1000, got %d", config.BulkConfig.StreamingThreshold)
	}

	if config.BulkConfig.ProgressLogInterval != 500 {
		t.Errorf("Expected default ProgressLogInterval 500, got %d", config.BulkConfig.ProgressLogInterval)
	}

	if config.BulkConfig.BatchTimeout != 60*time.Second {
		t.Errorf("Expected default BatchTimeout 60s, got %v", config.BulkConfig.BatchTimeout)
	}
}

func TestIndexDocumentsValidation(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config)

	// Test empty documents
	err := client.IndexDocuments(nil, nil)
	if err != nil {
		t.Errorf("IndexDocuments with nil documents should not return error, got: %v", err)
	}

	// Test empty slice
	err = client.IndexDocuments([]*models.Document{}, [][]float64{})
	if err != nil {
		t.Errorf("IndexDocuments with empty documents should not return error, got: %v", err)
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"this is a long string", 10, "this is a ..."},
		{"exact", 5, "exact"},
		{"", 5, ""},
	}

	for _, test := range tests {
		result := truncateString(test.input, test.maxLen)
		if result != test.expected {
			t.Errorf("truncateString(%q, %d) = %q, expected %q", test.input, test.maxLen, result, test.expected)
		}
	}
}

// Test health check functionality
func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectedError bool
		errorContains string
	}{
		{
			name:          "successful health check",
			statusCode:    200,
			responseBody:  `{"hits":{"total":0,"hits":[]}}`,
			expectedError: false,
		},
		{
			name:          "table not found (acceptable for health check)",
			statusCode:    400,
			responseBody:  `{"error":"table 'test_health_check' doesn't exist"}`,
			expectedError: false,
		},
		{
			name:          "server error",
			statusCode:    500,
			responseBody:  `{"error":"internal server error"}`,
			expectedError: true,
			errorContains: "health check failed",
		},
		{
			name:          "service unavailable",
			statusCode:    503,
			responseBody:  `{"error":"service unavailable"}`,
			expectedError: true,
			errorContains: "health check failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/search" {
					t.Errorf("Expected path /search, got %s", r.URL.Path)
				}
				if r.Method != "POST" {
					t.Errorf("Expected method POST, got %s", r.Method)
				}

				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			})
			defer server.Close()

			config := DefaultHTTPClientConfig(server.URL)
			client := NewHTTPClient(config)

			err := client.HealthCheck()

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

// Test WaitForReady functionality
func TestWaitForReady(t *testing.T) {
	t.Run("successful wait", func(t *testing.T) {
		server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte(`{"hits":{"total":0,"hits":[]}}`))
		})
		defer server.Close()

		config := DefaultHTTPClientConfig(server.URL)
		client := NewHTTPClient(config)

		err := client.WaitForReady(5 * time.Second)
		if err != nil {
			t.Errorf("Expected no error but got: %v", err)
		}

		if !client.IsConnected() {
			t.Error("Client should be connected after successful WaitForReady")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"server error"}`))
		})
		defer server.Close()

		config := DefaultHTTPClientConfig(server.URL)
		client := NewHTTPClient(config)

		err := client.WaitForReady(1 * time.Second)
		if err == nil {
			t.Error("Expected timeout error but got none")
		}

		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Expected timeout error, got: %v", err)
		}
	})
}

// Test schema operations
func TestCreateSchema(t *testing.T) {
	requestCount := 0
	expectedQueries := []string{
		"DROP TABLE IF EXISTS documents",
		"DROP TABLE IF EXISTS documents_vector",
		"CREATE TABLE documents (id bigint, title text, content text, url string)",
		"CREATE TABLE documents_vector (id bigint, title string, url string, vector_data text)",
	}

	server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sql" {
			t.Errorf("Expected path /sql, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}

		var sqlRequest SQLRequest
		if err := json.Unmarshal(body, &sqlRequest); err != nil {
			t.Fatalf("Failed to unmarshal SQL request: %v", err)
		}

		// Verify the query matches expected sequence
		if requestCount < len(expectedQueries) {
			if !strings.Contains(sqlRequest.Query, expectedQueries[requestCount]) {
				t.Errorf("Request %d: expected query to contain %q, got %q", requestCount, expectedQueries[requestCount], sqlRequest.Query)
			}
		}

		requestCount++

		w.WriteHeader(200)
		w.Write([]byte(`{"data":[]}`))
	})
	defer server.Close()

	config := DefaultHTTPClientConfig(server.URL)
	client := NewHTTPClient(config)

	err := client.CreateSchema(nil)
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}

	if requestCount < 4 {
		t.Errorf("Expected at least 4 SQL requests, got %d", requestCount)
	}
}

func TestResetDatabase(t *testing.T) {
	requestCount := 0
	expectedQueries := []string{
		"DROP TABLE IF EXISTS documents",
		"DROP TABLE IF EXISTS documents_vector",
	}

	server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sql" {
			t.Errorf("Expected path /sql, got %s", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}

		var sqlRequest SQLRequest
		if err := json.Unmarshal(body, &sqlRequest); err != nil {
			t.Fatalf("Failed to unmarshal SQL request: %v", err)
		}

		if requestCount < len(expectedQueries) {
			if !strings.Contains(sqlRequest.Query, expectedQueries[requestCount]) {
				t.Errorf("Request %d: expected query to contain %q, got %q", requestCount, expectedQueries[requestCount], sqlRequest.Query)
			}
		}

		requestCount++

		w.WriteHeader(200)
		w.Write([]byte(`{"data":[]}`))
	})
	defer server.Close()

	config := DefaultHTTPClientConfig(server.URL)
	client := NewHTTPClient(config)

	err := client.ResetDatabase()
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}
}

func TestTruncateTables(t *testing.T) {
	requestCount := 0
	expectedQueries := []string{
		"TRUNCATE TABLE documents",
		"TRUNCATE TABLE documents_vector",
	}

	server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sql" {
			t.Errorf("Expected path /sql, got %s", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}

		var sqlRequest SQLRequest
		if err := json.Unmarshal(body, &sqlRequest); err != nil {
			t.Fatalf("Failed to unmarshal SQL request: %v", err)
		}

		if requestCount < len(expectedQueries) {
			if !strings.Contains(sqlRequest.Query, expectedQueries[requestCount]) {
				t.Errorf("Request %d: expected query to contain %q, got %q", requestCount, expectedQueries[requestCount], sqlRequest.Query)
			}
		}

		requestCount++

		w.WriteHeader(200)
		w.Write([]byte(`{"data":[]}`))
	})
	defer server.Close()

	config := DefaultHTTPClientConfig(server.URL)
	client := NewHTTPClient(config)

	err := client.TruncateTables()
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}
}

// Test single document indexing with comprehensive mocking
func TestIndexDocumentHTTP(t *testing.T) {
	requestCount := 0

	server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/replace" {
			t.Errorf("Expected path /replace, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}

		var replaceRequest ReplaceRequest
		if err := json.Unmarshal(body, &replaceRequest); err != nil {
			t.Fatalf("Failed to unmarshal replace request: %v", err)
		}

		// Verify request structure
		if replaceRequest.ID != 1 {
			t.Errorf("Expected ID 1, got %d", replaceRequest.ID)
		}

		if requestCount == 0 {
			// First request should be for documents table
			if replaceRequest.Index != "documents" {
				t.Errorf("Expected index 'documents', got %s", replaceRequest.Index)
			}
			if title, ok := replaceRequest.Doc["title"].(string); !ok || title != "Test Document" {
				t.Errorf("Expected title 'Test Document', got %v", replaceRequest.Doc["title"])
			}
		} else if requestCount == 1 {
			// Second request should be for documents_vector table
			if replaceRequest.Index != "documents_vector" {
				t.Errorf("Expected index 'documents_vector', got %s", replaceRequest.Index)
			}
			if vectorData, ok := replaceRequest.Doc["vector_data"].(string); !ok || vectorData == "" {
				t.Errorf("Expected vector_data to be present, got %v", replaceRequest.Doc["vector_data"])
			}
		}

		requestCount++

		w.WriteHeader(200)
		w.Write([]byte(`{"_index":"` + replaceRequest.Index + `","_id":1,"created":true,"result":"created","status":201}`))
	})
	defer server.Close()

	config := DefaultHTTPClientConfig(server.URL)
	client := NewHTTPClient(config)

	doc := &models.Document{
		ID:      1,
		Title:   "Test Document",
		Content: "This is test content",
		URL:     "http://example.com/test",
	}
	vector := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

	err := client.IndexDocument(doc, vector)
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 requests (full-text and vector), got %d", requestCount)
	}
}

func TestIndexDocumentHTTPWithoutVector(t *testing.T) {
	requestCount := 0

	server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(200)
		w.Write([]byte(`{"_index":"documents","_id":1,"created":true,"result":"created","status":201}`))
	})
	defer server.Close()

	config := DefaultHTTPClientConfig(server.URL)
	client := NewHTTPClient(config)

	doc := &models.Document{
		ID:      1,
		Title:   "Test Document",
		Content: "This is test content",
		URL:     "http://example.com/test",
	}

	err := client.IndexDocument(doc, nil)
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}

	// Should only make one request (full-text only)
	if requestCount != 1 {
		t.Errorf("Expected 1 request (full-text only), got %d", requestCount)
	}
}

// Test bulk document indexing with HTTP mocking
func TestIndexDocumentsBulkHTTP(t *testing.T) {
	requestCount := 0

	server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bulk" {
			t.Errorf("Expected path /bulk, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/x-ndjson" {
			t.Errorf("Expected Content-Type 'application/x-ndjson', got %s", contentType)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}

		// Verify NDJSON format
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		if len(lines) != 2 {
			t.Errorf("Expected 2 NDJSON lines, got %d", len(lines))
		}

		// Parse first line
		var bulkReq map[string]interface{}
		if err := json.Unmarshal([]byte(lines[0]), &bulkReq); err != nil {
			t.Fatalf("Failed to unmarshal first bulk request: %v", err)
		}

		if replace, ok := bulkReq["replace"].(map[string]interface{}); ok {
			if index, ok := replace["index"].(string); !ok || (index != "documents" && index != "documents_vector") {
				t.Errorf("Expected index 'documents' or 'documents_vector', got %s", index)
			}
		}

		requestCount++

		w.WriteHeader(200)
		w.Write([]byte(`{"items":[{"replace":{"_index":"documents","_id":1,"created":true,"result":"created","status":201}},{"replace":{"_index":"documents","_id":2,"created":true,"result":"created","status":201}}],"errors":false}`))
	})
	defer server.Close()

	config := DefaultHTTPClientConfig(server.URL)
	client := NewHTTPClient(config)

	documents := []*models.Document{
		{ID: 1, Title: "Doc 1", Content: "Content 1", URL: "http://example.com/1"},
		{ID: 2, Title: "Doc 2", Content: "Content 2", URL: "http://example.com/2"},
	}
	vectors := [][]float64{
		{0.1, 0.2, 0.3},
		{0.4, 0.5, 0.6},
	}

	err := client.IndexDocuments(documents, vectors)
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}

	// Should make 2 requests (full-text and vector bulk operations)
	if requestCount != 2 {
		t.Errorf("Expected 2 requests (full-text and vector bulk), got %d", requestCount)
	}
}

// Test search operations
func TestSearchWithRequest(t *testing.T) {
	server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("Expected path /search, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}

		var searchRequest SearchRequest
		if err := json.Unmarshal(body, &searchRequest); err != nil {
			t.Fatalf("Failed to unmarshal search request: %v", err)
		}

		if searchRequest.Index != "documents" {
			t.Errorf("Expected index 'documents', got %s", searchRequest.Index)
		}

		if searchRequest.Limit != 10 {
			t.Errorf("Expected limit 10, got %d", searchRequest.Limit)
		}

		w.WriteHeader(200)
		w.Write([]byte(`{
			"took": 5,
			"timed_out": false,
			"hits": {
				"total": 2,
				"total_relation": "eq",
				"hits": [
					{
						"_index": "documents",
						"_id": 1,
						"_score": 1.5,
						"_source": {
							"title": "Test Document 1",
							"content": "This is test content 1",
							"url": "http://example.com/1"
						}
					},
					{
						"_index": "documents",
						"_id": 2,
						"_score": 1.2,
						"_source": {
							"title": "Test Document 2",
							"content": "This is test content 2",
							"url": "http://example.com/2"
						}
					}
				]
			}
		}`))
	})
	defer server.Close()

	config := DefaultHTTPClientConfig(server.URL)
	client := NewHTTPClient(config)

	request := SearchRequest{
		Index: "documents",
		Query: map[string]interface{}{
			"match": map[string]interface{}{
				"*": "test",
			},
		},
		Limit:  10,
		Offset: 0,
	}

	response, err := client.SearchWithRequest(request)
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}

	if response.Hits.Total != 2 {
		t.Errorf("Expected 2 hits, got %d", response.Hits.Total)
	}

	if len(response.Hits.Hits) != 2 {
		t.Errorf("Expected 2 hit items, got %d", len(response.Hits.Hits))
	}

	// Verify first hit
	firstHit := response.Hits.Hits[0]
	if firstHit.ID != 1 {
		t.Errorf("Expected first hit ID 1, got %d", firstHit.ID)
	}
	if firstHit.Score != 1.5 {
		t.Errorf("Expected first hit score 1.5, got %f", firstHit.Score)
	}
	if title, ok := firstHit.Source["title"].(string); !ok || title != "Test Document 1" {
		t.Errorf("Expected first hit title 'Test Document 1', got %v", firstHit.Source["title"])
	}
}

func TestGetAllDocuments(t *testing.T) {
	server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}

		var searchRequest SearchRequest
		if err := json.Unmarshal(body, &searchRequest); err != nil {
			t.Fatalf("Failed to unmarshal search request: %v", err)
		}

		// Verify it's a match_all query
		if matchAll, ok := searchRequest.Query["match_all"]; !ok {
			t.Error("Expected match_all query")
		} else if matchAllMap, ok := matchAll.(map[string]interface{}); !ok || len(matchAllMap) != 0 {
			t.Error("Expected empty match_all query")
		}

		w.WriteHeader(200)
		w.Write([]byte(`{
			"took": 2,
			"timed_out": false,
			"hits": {
				"total": 1,
				"total_relation": "eq",
				"hits": [
					{
						"_index": "documents",
						"_id": 1,
						"_score": 1.0,
						"_source": {
							"title": "All Documents Test",
							"content": "This is content for all documents test",
							"url": "http://example.com/all"
						}
					}
				]
			}
		}`))
	})
	defer server.Close()

	config := DefaultHTTPClientConfig(server.URL)
	client := NewHTTPClient(config)

	documents, err := client.GetAllDocuments()
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}

	if len(documents) != 1 {
		t.Errorf("Expected 1 document, got %d", len(documents))
	}

	if documents[0].ID != 1 {
		t.Errorf("Expected document ID 1, got %d", documents[0].ID)
	}
	if documents[0].Title != "All Documents Test" {
		t.Errorf("Expected title 'All Documents Test', got %s", documents[0].Title)
	}
}

// Test error handling
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectedError string
	}{
		{
			name:          "400 bad request",
			statusCode:    400,
			responseBody:  `{"error":"bad request"}`,
			expectedError: "HTTP 400",
		},
		{
			name:          "404 not found",
			statusCode:    404,
			responseBody:  `{"error":"not found"}`,
			expectedError: "HTTP 404",
		},
		{
			name:          "500 internal server error",
			statusCode:    500,
			responseBody:  `{"error":"internal server error"}`,
			expectedError: "HTTP 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			})
			defer server.Close()

			config := DefaultHTTPClientConfig(server.URL)
			client := NewHTTPClient(config)

			request := SearchRequest{
				Index: "documents",
				Query: map[string]interface{}{"match_all": map[string]interface{}{}},
				Limit: 10,
			}

			_, err := client.SearchWithRequest(request)
			if err == nil {
				t.Error("Expected error but got none")
			}

			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error to contain %q, got %q", tt.expectedError, err.Error())
			}
		})
	}
}

// Test retry logic with circuit breaker
func TestRetryLogic(t *testing.T) {
	attemptCount := 0
	maxAttempts := 3

	server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < maxAttempts {
			// Fail first few attempts
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"temporary failure"}`))
		} else {
			// Succeed on final attempt
			w.WriteHeader(200)
			w.Write([]byte(`{"hits":{"total":0,"hits":[]}}`))
		}
	})
	defer server.Close()

	config := DefaultHTTPClientConfig(server.URL)
	// Reduce retry delays for faster testing
	config.RetryConfig.BaseDelay = 10 * time.Millisecond
	config.RetryConfig.MaxDelay = 50 * time.Millisecond
	config.RetryConfig.MaxAttempts = maxAttempts

	client := NewHTTPClient(config)

	request := SearchRequest{
		Index: "documents",
		Query: map[string]interface{}{"match_all": map[string]interface{}{}},
		Limit: 10,
	}

	_, err := client.SearchWithRequest(request)
	if err != nil {
		t.Errorf("Expected no error after retries, got: %v", err)
	}

	if attemptCount != maxAttempts {
		t.Errorf("Expected %d attempts, got %d", maxAttempts, attemptCount)
	}
}

// Test circuit breaker state transitions
func TestCircuitBreakerStateTransitions(t *testing.T) {
	failureCount := 0
	failureThreshold := 3

	server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		failureCount++
		// Always fail to trigger circuit breaker
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"persistent failure"}`))
	})
	defer server.Close()

	config := DefaultHTTPClientConfig(server.URL)
	// Configure circuit breaker for quick testing
	config.CircuitBreakerConfig.FailureThreshold = failureThreshold
	config.CircuitBreakerConfig.RecoveryTimeout = 100 * time.Millisecond
	config.RetryConfig.MaxAttempts = 1 // Disable retries for this test

	client := NewHTTPClient(config).(*manticoreHTTPClient)

	request := SearchRequest{
		Index: "documents",
		Query: map[string]interface{}{"match_all": map[string]interface{}{}},
		Limit: 10,
	}

	// Make requests to trigger circuit breaker
	for i := 0; i < failureThreshold+2; i++ {
		_, err := client.SearchWithRequest(request)
		if err == nil {
			t.Errorf("Expected error on request %d", i+1)
		}
	}

	// Check circuit breaker state
	stats := client.circuitBreakerWithRetry.GetCircuitBreakerStats()
	if stats.State == CircuitBreakerClosed {
		t.Error("Expected circuit breaker to be open after failures")
	}

	// Verify failure count is reasonable (should be at least the threshold)
	if failureCount < failureThreshold {
		t.Errorf("Expected at least %d failures, got %d", failureThreshold, failureCount)
	}
}

// Test vector operations
func TestVectorOperations(t *testing.T) {
	t.Run("format vector as JSON array", func(t *testing.T) {
		tests := []struct {
			input    []float64
			expected string
		}{
			{[]float64{}, "[]"},
			{[]float64{1.0}, "[1.000000]"},
			{[]float64{1.0, 2.5, 3.14159}, "[1.000000,2.500000,3.141590]"},
		}

		for _, tt := range tests {
			result := formatVectorAsJSONArray(tt.input)
			if result != tt.expected {
				t.Errorf("formatVectorAsJSONArray(%v) = %q, expected %q", tt.input, result, tt.expected)
			}
		}
	})

	t.Run("cosine similarity", func(t *testing.T) {
		client := NewHTTPClient(DefaultHTTPClientConfig("http://localhost:9308")).(*manticoreHTTPClient)

		tests := []struct {
			name     string
			a        []float64
			b        []float64
			expected float64
		}{
			{"identical vectors", []float64{1, 0, 0}, []float64{1, 0, 0}, 1.0},
			{"orthogonal vectors", []float64{1, 0}, []float64{0, 1}, 0.0},
			{"opposite vectors", []float64{1, 0}, []float64{-1, 0}, -1.0},
			{"empty vectors", []float64{}, []float64{}, 0.0},
			{"mismatched length", []float64{1, 2}, []float64{1}, 0.0},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := client.cosineSimilarity(tt.a, tt.b)
				if abs(result-tt.expected) > 1e-10 {
					t.Errorf("cosineSimilarity(%v, %v) = %f, expected %f", tt.a, tt.b, result, tt.expected)
				}
			})
		}
	})
}

// Helper function for floating point comparison
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Test bulk operation strategies
func TestBulkOperationStrategies(t *testing.T) {
	t.Run("single bulk for small datasets", func(t *testing.T) {
		requestCount := 0
		server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			w.WriteHeader(200)
			w.Write([]byte(`{"items":[],"errors":false}`))
		})
		defer server.Close()

		config := DefaultHTTPClientConfig(server.URL)
		config.BulkConfig.BatchSize = 100
		config.BulkConfig.StreamingThreshold = 1000

		client := NewHTTPClient(config)

		// Small dataset (< batch size)
		documents := make([]*models.Document, 50)
		for i := range documents {
			documents[i] = &models.Document{ID: i + 1, Title: fmt.Sprintf("Doc %d", i+1)}
		}

		err := client.IndexDocuments(documents, nil)
		if err != nil {
			t.Errorf("Expected no error but got: %v", err)
		}

		// Should use single bulk operation (1 request for full-text)
		if requestCount != 1 {
			t.Errorf("Expected 1 request for single bulk, got %d", requestCount)
		}
	})

	t.Run("batched bulk for medium datasets", func(t *testing.T) {
		requestCount := 0
		server := createMockServer(t, func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			w.WriteHeader(200)
			w.Write([]byte(`{"items":[],"errors":false}`))
		})
		defer server.Close()

		config := DefaultHTTPClientConfig(server.URL)
		config.BulkConfig.BatchSize = 50
		config.BulkConfig.StreamingThreshold = 1000

		client := NewHTTPClient(config)

		// Medium dataset (> batch size, < streaming threshold)
		documents := make([]*models.Document, 150)
		for i := range documents {
			documents[i] = &models.Document{ID: i + 1, Title: fmt.Sprintf("Doc %d", i+1)}
		}

		err := client.IndexDocuments(documents, nil)
		if err != nil {
			t.Errorf("Expected no error but got: %v", err)
		}

		// Should use batched approach (150/50 = 3 batches for full-text)
		expectedRequests := 3
		if requestCount != expectedRequests {
			t.Errorf("Expected %d requests for batched bulk, got %d", expectedRequests, requestCount)
		}
	})
}

// Test request creation methods
func TestRequestCreationMethods(t *testing.T) {
	client := NewHTTPClient(DefaultHTTPClientConfig("http://localhost:9308")).(*manticoreHTTPClient)

	t.Run("basic search request", func(t *testing.T) {
		request := client.CreateBasicSearchRequest("documents", "test query", 20, 10)

		if request.Index != "documents" {
			t.Errorf("Expected index 'documents', got %s", request.Index)
		}
		if request.Limit != 20 {
			t.Errorf("Expected limit 20, got %d", request.Limit)
		}
		if request.Offset != 10 {
			t.Errorf("Expected offset 10, got %d", request.Offset)
		}

		// Verify match query structure
		if match, ok := request.Query["match"].(map[string]interface{}); ok {
			if query, ok := match["*"].(string); !ok || query != "test query" {
				t.Errorf("Expected match query 'test query', got %v", match["*"])
			}
		} else {
			t.Error("Expected match query in request")
		}
	})

	t.Run("full-text search request", func(t *testing.T) {
		request := client.CreateFullTextSearchRequest("documents", "title:test AND content:search", 15, 5)

		if queryString, ok := request.Query["query_string"].(string); !ok || queryString != "title:test AND content:search" {
			t.Errorf("Expected query_string 'title:test AND content:search', got %v", request.Query["query_string"])
		}
	})

	t.Run("match query request", func(t *testing.T) {
		request := client.CreateMatchQueryRequest("documents", "title", "specific title", 25, 0)

		if match, ok := request.Query["match"].(map[string]interface{}); ok {
			if query, ok := match["title"].(string); !ok || query != "specific title" {
				t.Errorf("Expected match query for title 'specific title', got %v", match["title"])
			}
		} else {
			t.Error("Expected match query in request")
		}
	})

	t.Run("match all request", func(t *testing.T) {
		request := client.CreateMatchAllRequest("documents", 100, 50)

		if matchAll, ok := request.Query["match_all"].(map[string]interface{}); !ok || len(matchAll) != 0 {
			t.Errorf("Expected empty match_all query, got %v", request.Query["match_all"])
		}
	})
}
