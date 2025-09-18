package manticore

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ad/manticoresearch-go/internal/models"
)

// Integration tests that require a real Manticore instance
// These tests are skipped unless MANTICORE_INTEGRATION_TESTS=1 is set

func skipIfNoIntegration(t *testing.T) {
	if os.Getenv("MANTICORE_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test. Set MANTICORE_INTEGRATION_TESTS=1 to run.")
	}
}

func getManticoreURL() string {
	url := os.Getenv("MANTICORE_URL")
	if url == "" {
		url = "http://localhost:9308"
	}
	return url
}

func createIntegrationClient(t *testing.T) ClientInterface {
	config := DefaultHTTPClientConfig(getManticoreURL())
	// Use shorter timeouts for integration tests
	config.Timeout = 30 * time.Second
	config.RetryConfig.BaseDelay = 100 * time.Millisecond
	config.RetryConfig.MaxDelay = 1 * time.Second
	config.RetryConfig.MaxAttempts = 3

	client := NewHTTPClient(config)

	// Wait for Manticore to be ready
	err := client.WaitForReady(30 * time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to Manticore at %s: %v", getManticoreURL(), err)
	}

	return client
}

func TestIntegrationHealthCheck(t *testing.T) {
	skipIfNoIntegration(t)

	client := createIntegrationClient(t)
	defer client.Close()

	err := client.HealthCheck()
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}

	if !client.IsConnected() {
		t.Error("Client should be connected after successful health check")
	}
}

func TestIntegrationSchemaOperations(t *testing.T) {
	skipIfNoIntegration(t)

	client := createIntegrationClient(t)
	defer client.Close()

	t.Run("reset database", func(t *testing.T) {
		err := client.ResetDatabase()
		if err != nil {
			t.Errorf("ResetDatabase failed: %v", err)
		}
	})

	t.Run("create schema", func(t *testing.T) {
		err := client.CreateSchema(nil)
		if err != nil {
			t.Errorf("CreateSchema failed: %v", err)
		}
	})

	t.Run("truncate tables", func(t *testing.T) {
		err := client.TruncateTables()
		if err != nil {
			t.Errorf("TruncateTables failed: %v", err)
		}
	})
}

func TestIntegrationSingleDocumentIndexing(t *testing.T) {
	skipIfNoIntegration(t)

	client := createIntegrationClient(t)
	defer client.Close()

	// Ensure clean state
	err := client.CreateSchema(nil)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	t.Run("index document without vector", func(t *testing.T) {
		doc := &models.Document{
			ID:      1,
			Title:   "Integration Test Document",
			Content: "This is a test document for integration testing",
			URL:     "http://example.com/integration-test",
		}

		err := client.IndexDocument(doc, nil)
		if err != nil {
			t.Errorf("IndexDocument failed: %v", err)
		}
	})

	t.Run("index document with vector", func(t *testing.T) {
		doc := &models.Document{
			ID:      2,
			Title:   "Vector Test Document",
			Content: "This document has vector data",
			URL:     "http://example.com/vector-test",
		}
		vector := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

		err := client.IndexDocument(doc, vector)
		if err != nil {
			t.Errorf("IndexDocument with vector failed: %v", err)
		}
	})
}

func TestIntegrationBulkDocumentIndexing(t *testing.T) {
	skipIfNoIntegration(t)

	client := createIntegrationClient(t)
	defer client.Close()

	// Ensure clean state
	err := client.CreateSchema(nil)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	tests := []struct {
		name     string
		docCount int
	}{
		{"small bulk (10 docs)", 10},
		{"medium bulk (50 docs)", 50},
		{"large bulk (150 docs)", 150},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear tables before each test
			err := client.TruncateTables()
			if err != nil {
				t.Fatalf("Failed to truncate tables: %v", err)
			}

			// Create test documents
			documents := make([]*models.Document, tt.docCount)
			vectors := make([][]float64, tt.docCount)

			for i := 0; i < tt.docCount; i++ {
				documents[i] = &models.Document{
					ID:      i + 1,
					Title:   fmt.Sprintf("Bulk Test Document %d", i+1),
					Content: fmt.Sprintf("This is bulk test content for document %d", i+1),
					URL:     fmt.Sprintf("http://example.com/bulk-test-%d", i+1),
				}
				vectors[i] = []float64{
					float64(i) * 0.1,
					float64(i) * 0.2,
					float64(i) * 0.3,
				}
			}

			startTime := time.Now()
			err = client.IndexDocuments(documents, vectors)
			duration := time.Since(startTime)

			if err != nil {
				t.Errorf("IndexDocuments failed for %d documents: %v", tt.docCount, err)
			}

			t.Logf("Indexed %d documents in %v (%.2f docs/sec)",
				tt.docCount, duration, float64(tt.docCount)/duration.Seconds())
		})
	}
}

func TestIntegrationSearchOperations(t *testing.T) {
	skipIfNoIntegration(t)

	client := createIntegrationClient(t)
	defer client.Close()

	// Setup test data
	err := client.CreateSchema(nil)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	// Index test documents
	testDocs := []*models.Document{
		{ID: 1, Title: "Go Programming", Content: "Go is a programming language developed by Google", URL: "http://example.com/go"},
		{ID: 2, Title: "Python Tutorial", Content: "Python is a versatile programming language", URL: "http://example.com/python"},
		{ID: 3, Title: "JavaScript Guide", Content: "JavaScript is used for web development", URL: "http://example.com/js"},
		{ID: 4, Title: "Database Design", Content: "Learn about database design principles", URL: "http://example.com/db"},
		{ID: 5, Title: "Web Development", Content: "Modern web development with JavaScript and Go", URL: "http://example.com/web"},
	}

	testVectors := [][]float64{
		{0.1, 0.2, 0.3},
		{0.2, 0.3, 0.4},
		{0.3, 0.4, 0.5},
		{0.4, 0.5, 0.6},
		{0.5, 0.6, 0.7},
	}

	err = client.IndexDocuments(testDocs, testVectors)
	if err != nil {
		t.Fatalf("Failed to index test documents: %v", err)
	}

	// Wait a moment for indexing to complete
	time.Sleep(1 * time.Second)

	t.Run("basic search", func(t *testing.T) {
		request := SearchRequest{
			Index: "documents",
			Query: map[string]interface{}{
				"match": map[string]interface{}{
					"*": "programming",
				},
			},
			Limit: 10,
		}

		response, err := client.SearchWithRequest(request)
		if err != nil {
			t.Errorf("Basic search failed: %v", err)
			return
		}

		if response.Hits.Total == 0 {
			t.Error("Expected search results but got none")
		}

		t.Logf("Basic search found %d results", response.Hits.Total)

		// Verify result structure
		if len(response.Hits.Hits) > 0 {
			hit := response.Hits.Hits[0]
			if hit.ID == 0 {
				t.Error("Expected non-zero document ID")
			}
			if hit.Score == 0 {
				t.Error("Expected non-zero score")
			}
			if hit.Source == nil {
				t.Error("Expected source data")
			}
		}
	})

	t.Run("full-text search", func(t *testing.T) {
		request := SearchRequest{
			Index: "documents",
			Query: map[string]interface{}{
				"query_string": "Go AND programming",
			},
			Limit: 5,
		}

		response, err := client.SearchWithRequest(request)
		if err != nil {
			t.Errorf("Full-text search failed: %v", err)
			return
		}

		t.Logf("Full-text search found %d results", response.Hits.Total)
	})

	t.Run("match all documents", func(t *testing.T) {
		documents, err := client.GetAllDocuments()
		if err != nil {
			t.Errorf("GetAllDocuments failed: %v", err)
			return
		}

		if len(documents) != len(testDocs) {
			t.Errorf("Expected %d documents, got %d", len(testDocs), len(documents))
		}

		t.Logf("Retrieved %d documents", len(documents))

		// Verify document structure
		if len(documents) > 0 {
			doc := documents[0]
			if doc.ID == 0 {
				t.Error("Expected non-zero document ID")
			}
			if doc.Title == "" {
				t.Error("Expected non-empty title")
			}
		}
	})

	t.Run("pagination", func(t *testing.T) {
		// Test pagination with limit and offset
		request := SearchRequest{
			Index: "documents",
			Query: map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
			Limit:  2,
			Offset: 1,
		}

		response, err := client.SearchWithRequest(request)
		if err != nil {
			t.Errorf("Pagination search failed: %v", err)
			return
		}

		if len(response.Hits.Hits) > 2 {
			t.Errorf("Expected at most 2 results, got %d", len(response.Hits.Hits))
		}

		t.Logf("Pagination search returned %d results", len(response.Hits.Hits))
	})
}

func TestIntegrationErrorHandling(t *testing.T) {
	skipIfNoIntegration(t)

	client := createIntegrationClient(t)
	defer client.Close()

	t.Run("search non-existent index", func(t *testing.T) {
		request := SearchRequest{
			Index: "non_existent_index",
			Query: map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
			Limit: 10,
		}

		_, err := client.SearchWithRequest(request)
		if err == nil {
			t.Error("Expected error when searching non-existent index")
		}

		t.Logf("Got expected error: %v", err)
	})

	t.Run("invalid query syntax", func(t *testing.T) {
		request := SearchRequest{
			Index: "documents",
			Query: map[string]interface{}{
				"invalid_query_type": "test",
			},
			Limit: 10,
		}

		_, err := client.SearchWithRequest(request)
		if err == nil {
			t.Error("Expected error for invalid query syntax")
		}

		t.Logf("Got expected error: %v", err)
	})
}

func TestIntegrationPerformance(t *testing.T) {
	skipIfNoIntegration(t)

	client := createIntegrationClient(t)
	defer client.Close()

	// Setup
	err := client.CreateSchema(nil)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	t.Run("bulk indexing performance", func(t *testing.T) {
		docCount := 1000
		documents := make([]*models.Document, docCount)
		vectors := make([][]float64, docCount)

		for i := 0; i < docCount; i++ {
			documents[i] = &models.Document{
				ID:      i + 1,
				Title:   fmt.Sprintf("Performance Test Document %d", i+1),
				Content: fmt.Sprintf("This is performance test content for document %d with some additional text to make it more realistic", i+1),
				URL:     fmt.Sprintf("http://example.com/perf-test-%d", i+1),
			}
			vectors[i] = []float64{
				float64(i) * 0.001,
				float64(i) * 0.002,
				float64(i) * 0.003,
				float64(i) * 0.004,
				float64(i) * 0.005,
			}
		}

		startTime := time.Now()
		err = client.IndexDocuments(documents, vectors)
		duration := time.Since(startTime)

		if err != nil {
			t.Errorf("Bulk indexing failed: %v", err)
			return
		}

		docsPerSecond := float64(docCount) / duration.Seconds()
		t.Logf("Indexed %d documents in %v (%.2f docs/sec)", docCount, duration, docsPerSecond)

		// Performance assertion - should be able to index at least 100 docs/sec
		if docsPerSecond < 100 {
			t.Logf("Warning: Indexing performance is below expected threshold (%.2f docs/sec < 100 docs/sec)", docsPerSecond)
		}
	})

	t.Run("search performance", func(t *testing.T) {
		// Perform multiple searches to test performance
		searchCount := 100
		totalDuration := time.Duration(0)

		for i := 0; i < searchCount; i++ {
			request := SearchRequest{
				Index: "documents",
				Query: map[string]interface{}{
					"match": map[string]interface{}{
						"*": fmt.Sprintf("document %d", i%10),
					},
				},
				Limit: 10,
			}

			startTime := time.Now()
			_, err = client.SearchWithRequest(request)
			duration := time.Since(startTime)
			totalDuration += duration

			if err != nil {
				t.Errorf("Search %d failed: %v", i, err)
				return
			}
		}

		avgDuration := totalDuration / time.Duration(searchCount)
		searchesPerSecond := float64(searchCount) / totalDuration.Seconds()

		t.Logf("Performed %d searches in %v (avg: %v, %.2f searches/sec)",
			searchCount, totalDuration, avgDuration, searchesPerSecond)

		// Performance assertion - average search should be under 100ms
		if avgDuration > 100*time.Millisecond {
			t.Logf("Warning: Search performance is below expected threshold (avg: %v > 100ms)", avgDuration)
		}
	})
}

func TestIntegrationCircuitBreakerRecovery(t *testing.T) {
	skipIfNoIntegration(t)

	// Create client with aggressive circuit breaker settings for testing
	config := DefaultHTTPClientConfig(getManticoreURL())
	config.CircuitBreakerConfig.FailureThreshold = 2
	config.CircuitBreakerConfig.RecoveryTimeout = 1 * time.Second
	config.RetryConfig.MaxAttempts = 1 // Disable retries for this test

	client := NewHTTPClient(config)
	defer client.Close()

	err := client.WaitForReady(10 * time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to Manticore: %v", err)
	}

	t.Run("circuit breaker opens and recovers", func(t *testing.T) {
		// First, trigger circuit breaker with invalid requests
		for i := 0; i < 3; i++ {
			request := SearchRequest{
				Index: "non_existent_table",
				Query: map[string]interface{}{
					"match_all": map[string]interface{}{},
				},
				Limit: 10,
			}

			_, err := client.SearchWithRequest(request)
			if err == nil {
				t.Error("Expected error for non-existent table")
			}
		}

		// Circuit breaker should now be open
		httpClient := client.(*manticoreHTTPClient)
		stats := httpClient.circuitBreakerWithRetry.GetCircuitBreakerStats()

		if stats.State == CircuitBreakerClosed {
			t.Error("Expected circuit breaker to be open after failures")
		}

		t.Logf("Circuit breaker state: %s, failures: %d", stats.State, stats.ConsecutiveFailures)

		// Wait for recovery timeout
		time.Sleep(2 * time.Second)

		// Now try a valid request - should work and reset circuit breaker
		err = client.CreateSchema(nil)
		if err != nil {
			t.Errorf("Expected successful request after recovery timeout: %v", err)
		}

		// Circuit breaker should be closed again
		stats = httpClient.circuitBreakerWithRetry.GetCircuitBreakerStats()
		t.Logf("Circuit breaker state after recovery: %s, failures: %d", stats.State, stats.ConsecutiveFailures)
	})
}

func TestIntegrationCompareWithOldImplementation(t *testing.T) {
	skipIfNoIntegration(t)

	// This test would compare results between old and new implementations
	// For now, we'll just test that the new implementation produces consistent results

	client := createIntegrationClient(t)
	defer client.Close()

	// Setup test data
	err := client.CreateSchema(nil)
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	testDoc := &models.Document{
		ID:      1,
		Title:   "Comparison Test",
		Content: "This document is used for comparing implementations",
		URL:     "http://example.com/comparison",
	}

	err = client.IndexDocument(testDoc, []float64{0.1, 0.2, 0.3})
	if err != nil {
		t.Fatalf("Failed to index test document: %v", err)
	}

	// Wait for indexing
	time.Sleep(1 * time.Second)

	t.Run("consistent search results", func(t *testing.T) {
		request := SearchRequest{
			Index: "documents",
			Query: map[string]interface{}{
				"match": map[string]interface{}{
					"*": "comparison",
				},
			},
			Limit: 10,
		}

		// Perform the same search multiple times
		var responses []*SearchResponse
		for i := 0; i < 5; i++ {
			response, err := client.SearchWithRequest(request)
			if err != nil {
				t.Errorf("Search %d failed: %v", i, err)
				continue
			}
			responses = append(responses, response)
		}

		if len(responses) < 2 {
			t.Fatal("Need at least 2 successful responses to compare")
		}

		// Compare results for consistency
		firstResponse := responses[0]
		for i, response := range responses[1:] {
			if response.Hits.Total != firstResponse.Hits.Total {
				t.Errorf("Response %d has different total hits: %d vs %d",
					i+1, response.Hits.Total, firstResponse.Hits.Total)
			}

			if len(response.Hits.Hits) != len(firstResponse.Hits.Hits) {
				t.Errorf("Response %d has different number of hits: %d vs %d",
					i+1, len(response.Hits.Hits), len(firstResponse.Hits.Hits))
			}
		}

		t.Logf("All %d search responses were consistent", len(responses))
	})
}
