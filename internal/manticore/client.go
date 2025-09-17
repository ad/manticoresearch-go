package manticore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ad/manticoresearch-go/internal/models"
	openapi "github.com/manticoresoftware/manticoresearch-go"
)

// retryOperation retries an operation with enhanced exponential backoff for network issues
func retryOperation(operation func() error, maxAttempts int, baseDelay time.Duration) error {
	var lastErr error

	// Increase attempts for network-related operations
	if maxAttempts < 5 {
		maxAttempts = 5 // Minimum 5 attempts for network operations
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := operation()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Enhanced error logging with detailed information
		log.Printf("DETAILED ERROR (attempt %d/%d): %+v", attempt+1, maxAttempts, err)

		// Try to extract more details from OpenAPI error
		if openAPIErr, ok := err.(*openapi.GenericOpenAPIError); ok {
			log.Printf("OpenAPI Error Body: %s\n", string(openAPIErr.Body()))
			log.Printf("OpenAPI Error Model: %+v\n", openAPIErr.Model())
		}

		// Check if error is retryable
		if !isRetryableError(err) {
			log.Printf("Error is not retryable, stopping attempts")
			break // Don't retry non-retryable errors
		}

		if attempt < maxAttempts-1 {
			// Progressive backoff with special handling for different error types
			delay := calculateBackoffDelay(err, attempt, baseDelay)
			jitter := time.Duration(rand.Intn(200)) * time.Millisecond // Increased jitter
			totalDelay := delay + jitter

			log.Printf("Retrying operation (attempt %d/%d) after %v delay due to error: %v", attempt+2, maxAttempts, totalDelay, err)
			time.Sleep(totalDelay)
		}
	}
	return fmt.Errorf("operation failed after %d attempts: %w", maxAttempts, lastErr)
}

// retryOperationWithCircuitBreaker retries operation with circuit breaker protection
func (mc *ManticoreClient) retryOperationWithCircuitBreaker(operation func() error, maxAttempts int, baseDelay time.Duration) error {
	// Check circuit breaker before attempting operation
	if !mc.circuitBreaker.shouldAllowRequest() {
		return fmt.Errorf("circuit breaker is OPEN: too many consecutive failures")
	}

	err := retryOperation(operation, maxAttempts, baseDelay)

	if err != nil {
		mc.circuitBreaker.recordFailure()
	} else {
		mc.circuitBreaker.recordSuccess()
	}

	return err
}

// calculateBackoffDelay calculates progressive delay based on error type and attempt
func calculateBackoffDelay(err error, attempt int, baseDelay time.Duration) time.Duration {
	errStr := strings.ToLower(err.Error())

	// Base exponential backoff
	delay := baseDelay * time.Duration(1<<attempt)

	// Special handling for different types of network errors
	switch {
	case strings.Contains(errStr, "connection reset"):
		// Connection reset needs longer delays to allow network recovery
		return delay * 3
	case strings.Contains(errStr, "broken pipe") || strings.Contains(errStr, "closed network connection"):
		// Broken pipe/closed connection - progressive increase
		return delay * 2
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "i/o timeout"):
		// Timeout errors - moderate increase
		return delay * 2
	case strings.Contains(errStr, "connection refused"):
		// Service unavailable - aggressive backoff
		return delay * 4
	default:
		// Standard exponential backoff for other errors
		return delay
	}
}

// isRetryableError determines if an error is worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Network-related errors that are typically transient
	retryableErrors := []string{
		"connection reset by peer",
		"connection refused",
		"timeout",
		"temporary failure",
		"broken pipe",
		"no such host",
		"network is unreachable",
		"i/o timeout",
		"eof",
		"use of closed network connection",
		"write: broken pipe",
		"read: connection reset",
		"context deadline exceeded",
		"dial tcp", // General TCP dial issues
		"no route to host",
		"host is down",
		"operation timed out",
		"server closed idle connection",
		"connection lost",
		"readfrom tcp", // Docker networking issues
		"write tcp",    // TCP write failures
	}

	for _, retryableErr := range retryableErrors {
		if strings.Contains(errStr, retryableErr) {
			return true
		}
	}

	return false
}

// CircuitBreaker tracks connection health
type CircuitBreaker struct {
	consecutiveFailures int
	lastFailureTime     time.Time
	isOpen              bool
}

// recordFailure records a failure and potentially opens the circuit
func (cb *CircuitBreaker) recordFailure() {
	cb.consecutiveFailures++
	cb.lastFailureTime = time.Now()

	// Open circuit after 10 consecutive failures
	if cb.consecutiveFailures >= 10 {
		cb.isOpen = true
		log.Printf("Circuit breaker OPEN: %d consecutive failures", cb.consecutiveFailures)
	}
}

// recordSuccess resets the circuit breaker
func (cb *CircuitBreaker) recordSuccess() {
	if cb.consecutiveFailures > 0 {
		log.Printf("Circuit breaker RESET: after %d failures", cb.consecutiveFailures)
	}
	cb.consecutiveFailures = 0
	cb.isOpen = false
}

// shouldAllowRequest determines if request should be allowed
func (cb *CircuitBreaker) shouldAllowRequest() bool {
	if !cb.isOpen {
		return true
	}

	// Allow request after 30 seconds to test if service recovered
	if time.Since(cb.lastFailureTime) > 30*time.Second {
		log.Printf("Circuit breaker: allowing test request after %v", time.Since(cb.lastFailureTime))
		return true
	}

	return false
}

// ManticoreClient creates a new official Manticore client
type ManticoreClient struct {
	client         *openapi.APIClient
	ctx            context.Context
	circuitBreaker *CircuitBreaker
}

// NewClient creates a new Manticore client using official Go client
func NewClient(httpHost string) *ManticoreClient {
	configuration := openapi.NewConfiguration()
	configuration.Servers[0].URL = fmt.Sprintf("http://%s", httpHost)

	// Enhanced timeouts and connection settings for Docker environment
	configuration.HTTPClient = &http.Client{
		Timeout: 60 * time.Second, // Increased from 30s for Docker network latency
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   15 * time.Second, // Increased from 10s
				KeepAlive: 60 * time.Second, // Increased from 30s
			}).DialContext,
			TLSHandshakeTimeout:   15 * time.Second, // Increased from 10s
			ResponseHeaderTimeout: 20 * time.Second, // Increased from 10s
			ExpectContinueTimeout: 2 * time.Second,  // Increased from 1s
			MaxIdleConns:          20,               // Increased from 10
			MaxIdleConnsPerHost:   10,               // Increased from 2 for better throughput
			IdleConnTimeout:       90 * time.Second, // Increased from 30s
			// Additional settings for connection stability
			DisableCompression: false,
			ForceAttemptHTTP2:  false, // Disable HTTP/2 for better compatibility
			WriteBufferSize:    32768, // 32KB write buffer
			ReadBufferSize:     32768, // 32KB read buffer
		},
	}

	client := openapi.NewAPIClient(configuration)

	return &ManticoreClient{
		client: client,
		ctx:    context.Background(),
		circuitBreaker: &CircuitBreaker{
			consecutiveFailures: 0,
			isOpen:              false,
		},
	}
} // WaitForReady waits for Manticore to be ready with timeout
func (mc *ManticoreClient) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if err := mc.HealthCheck(); err == nil {
			return nil
		}
		log.Printf("Waiting for Manticore to be ready...")
		time.Sleep(time.Second * 2)
	}

	return fmt.Errorf("timeout waiting for Manticore to be ready")
}

// HealthCheck verifies that the Manticore connection is healthy
func (mc *ManticoreClient) HealthCheck() error {
	// Use a simple HTTP client to check if Manticore is responding
	config := mc.client.GetConfig()
	if len(config.Servers) == 0 {
		return fmt.Errorf("no servers configured")
	}

	// Make a simple HTTP POST request to search endpoint to verify connection
	httpClient := &http.Client{Timeout: 5 * time.Second}
	testURL := fmt.Sprintf("%s/search", config.Servers[0].URL)

	// Try a simple search to verify connection (this will return an error about missing table but indicates server is responding)
	resp, err := httpClient.Post(testURL, "application/json", strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	// Accept both success responses and 400 responses that indicate the server is responding
	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check failed: HTTP %d, %s", resp.StatusCode, string(body))
	}

	return nil
} // CreateSchema creates the necessary tables for documents and vectors using the official API
func (mc *ManticoreClient) CreateSchema() error {
	log.Println("Creating schema using official Manticore API...")

	// Drop existing tables if they exist (ignore errors for first run)
	if err := mc.ResetDatabase(); err != nil {
		log.Printf("Warning during table cleanup (this is normal for first run): %v", err)
	}

	// Create full-text search table using SQL via utils API
	fullTextQuery := "CREATE TABLE documents (id bigint, title text, content text, url string)"

	_, _, err := mc.client.UtilsAPI.Sql(mc.ctx).Body(fullTextQuery).Execute()
	if err != nil {
		return fmt.Errorf("failed to create documents table: %v", err)
	}
	log.Println("Created full-text search table 'documents'")

	// Create vector search table
	vectorQuery := "CREATE TABLE documents_vector (id bigint, title string, url string, vector_data text)"

	_, _, err = mc.client.UtilsAPI.Sql(mc.ctx).Body(vectorQuery).Execute()
	if err != nil {
		return fmt.Errorf("failed to create documents_vector table: %v", err)
	}
	log.Println("Created vector search table 'documents_vector'")

	return nil
}

// executeSQL executes SQL command via HTTP to avoid large MySQL requests
func (mc *ManticoreClient) executeSQL(client *http.Client, baseURL, query string) error {
	url := fmt.Sprintf("%s/sql", baseURL)

	resp, err := client.Post(url, "application/x-www-form-urlencoded", strings.NewReader(query))
	if err != nil {
		return fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SQL execution failed: HTTP %d, %s", resp.StatusCode, string(body))
	}

	return nil
}

// ResetDatabase drops existing tables to start fresh
func (mc *ManticoreClient) ResetDatabase() error {
	// Drop existing tables using SQL API
	dropDocuments := "DROP TABLE IF EXISTS documents"
	_, _, _ = mc.client.UtilsAPI.Sql(mc.ctx).Body(dropDocuments).Execute()

	dropVectors := "DROP TABLE IF EXISTS documents_vector"
	_, _, _ = mc.client.UtilsAPI.Sql(mc.ctx).Body(dropVectors).Execute()

	log.Println("Dropped existing tables")
	return nil
}

// TruncateTables clears all data from existing tables
func (mc *ManticoreClient) TruncateTables() error {
	config := mc.client.GetConfig()
	if len(config.Servers) == 0 {
		return fmt.Errorf("no servers configured")
	}

	baseURL := config.Servers[0].URL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Use HTTP method instead of SQL API to avoid large requests
	_ = mc.executeSQL(httpClient, baseURL, "TRUNCATE TABLE documents")
	_ = mc.executeSQL(httpClient, baseURL, "TRUNCATE TABLE documents_vector")

	log.Println("Truncated all tables")
	return nil
}

// IndexDocument indexes a single document in both full-text and vector tables
// For single documents, we use bulk operations with 1 document for consistency
func (mc *ManticoreClient) IndexDocument(doc *models.Document, vector []float64) error {
	// Use bulk operations even for single document - more consistent and reliable
	documents := []*models.Document{doc}
	vectors := [][]float64{vector}

	log.Printf("Indexing single document %d (%s) using bulk operations", doc.ID, doc.Title)

	// Index in full-text table
	err := mc.bulkIndexDocuments(documents)
	if err != nil {
		// Fallback to individual operation
		log.Printf("Bulk indexing failed for document %d, using fallback", doc.ID)

		docData := map[string]interface{}{
			"title":   doc.Title,
			"content": doc.Content,
			"url":     doc.URL,
		}

		err = mc.retryOperationWithCircuitBreaker(func() error {
			insertReq := openapi.NewInsertDocumentRequest("documents", docData)
			insertReq.SetId(uint64(doc.ID))
			_, _, err := mc.client.IndexAPI.Replace(mc.ctx).InsertDocumentRequest(*insertReq).Execute()
			return err
		}, 5, 500*time.Millisecond)

		if err != nil {
			return fmt.Errorf("failed to index document %d (%s) in full-text table: %v", doc.ID, doc.Title, err)
		}
	}

	// Index in vector table
	err = mc.bulkIndexVectors(documents, vectors)
	if err != nil {
		// Fallback to individual operation
		log.Printf("Bulk vector indexing failed for document %d, using fallback", doc.ID)

		vectorData := map[string]interface{}{
			"title":       doc.Title,
			"url":         doc.URL,
			"vector_data": formatVectorForManticore(vector),
		}

		err = mc.retryOperationWithCircuitBreaker(func() error {
			vectorInsertReq := openapi.NewInsertDocumentRequest("documents_vector", vectorData)
			vectorInsertReq.SetId(uint64(doc.ID))
			_, _, err := mc.client.IndexAPI.Replace(mc.ctx).InsertDocumentRequest(*vectorInsertReq).Execute()
			return err
		}, 5, 500*time.Millisecond)

		if err != nil {
			return fmt.Errorf("failed to index document %d in vector table: %v", doc.ID, err)
		}
	}

	log.Printf("Successfully indexed document %d (%s)", doc.ID, doc.Title)
	return nil
}

// IndexDocuments indexes multiple documents using efficient bulk operations
func (mc *ManticoreClient) IndexDocuments(documents []*models.Document, vectors [][]float64) error {
	if len(documents) != len(vectors) {
		return fmt.Errorf("documents and vectors count mismatch: %d vs %d", len(documents), len(vectors))
	}

	log.Printf("Starting to index %d documents using bulk operations", len(documents))

	const batchSize = 100 // Larger batches for bulk operations - more efficient

	var lastError error
	successfulBatches := 0
	totalBatches := (len(documents) + batchSize - 1) / batchSize

	// Process documents in batches using bulk operations
	for batchStart := 0; batchStart < len(documents); batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > len(documents) {
			batchEnd = len(documents)
		}

		batchDocs := documents[batchStart:batchEnd]
		batchVectors := vectors[batchStart:batchEnd]

		log.Printf("Processing batch %d/%d: documents %d-%d", successfulBatches+1, totalBatches, batchStart+1, batchEnd)

		// Index documents in full-text table using bulk operation
		err := mc.bulkIndexDocuments(batchDocs)
		if err != nil {
			log.Printf("Warning: Failed to bulk index documents batch %d-%d: %v", batchStart+1, batchEnd, err)
			lastError = err

			// Fallback to individual operations for this batch
			log.Printf("Falling back to individual indexing for batch %d-%d", batchStart+1, batchEnd)
			err = mc.indexDocumentsBatchIndividually(batchDocs, batchVectors)
			if err != nil {
				log.Printf("Error: Individual fallback also failed for batch %d-%d: %v", batchStart+1, batchEnd, err)
				continue
			}
		} else {
			// Index vectors in vector table using bulk operation
			err = mc.bulkIndexVectors(batchDocs, batchVectors)
			if err != nil {
				log.Printf("Warning: Failed to bulk index vectors batch %d-%d: %v", batchStart+1, batchEnd, err)
				lastError = err

				// Try individual vector indexing as fallback
				for i, doc := range batchDocs {
					vectorData := map[string]interface{}{
						"title":       doc.Title,
						"url":         doc.URL,
						"vector_data": formatVectorForManticore(batchVectors[i]),
					}

					err = mc.retryOperationWithCircuitBreaker(func() error {
						vectorInsertReq := openapi.NewInsertDocumentRequest("documents_vector", vectorData)
						vectorInsertReq.SetId(uint64(doc.ID))
						_, _, err := mc.client.IndexAPI.Replace(mc.ctx).InsertDocumentRequest(*vectorInsertReq).Execute()
						return err
					}, 5, 500*time.Millisecond)

					if err != nil {
						log.Printf("Warning: Failed to index vector %d individually: %v", doc.ID, err)
					}
				}
			}
		}

		successfulBatches++
		log.Printf("Completed batch %d/%d", successfulBatches, totalBatches)

		// Small delay between batches to avoid overwhelming the server
		time.Sleep(100 * time.Millisecond)
	}

	if lastError != nil {
		log.Printf("Warning: Some batches failed during bulk indexing, last error: %v", lastError)
	}

	log.Printf("Successfully processed %d documents using bulk operations", len(documents))
	return nil
}

// bulkIndexDocuments indexes multiple documents using bulk API for full-text table
func (mc *ManticoreClient) bulkIndexDocuments(documents []*models.Document) error {
	if len(documents) == 0 {
		return nil
	}

	log.Printf("Starting bulk indexing of %d documents in 'documents' table", len(documents))

	// Build NDJSON for bulk operation
	ndjsonBuilder := strings.Builder{}

	for _, doc := range documents {
		// Build replace command for each document
		docData := map[string]interface{}{
			"title":   doc.Title,
			"content": doc.Content,
			"url":     doc.URL,
		}

		// Create replace operation in NDJSON format
		replaceOp := map[string]interface{}{
			"replace": map[string]interface{}{
				"index": "documents",
				"id":    doc.ID,
				"doc":   docData,
			},
		}

		// Convert to JSON and add newline
		jsonBytes, err := json.Marshal(replaceOp)
		if err != nil {
			return fmt.Errorf("failed to marshal bulk operation: %v", err)
		}
		ndjsonBuilder.Write(jsonBytes)
		ndjsonBuilder.WriteByte('\n')
	}

	// Execute bulk operation with enhanced retry and circuit breaker
	return mc.retryOperationWithCircuitBreaker(func() error {
		resp, _, err := mc.client.IndexAPI.Bulk(mc.ctx).Body(ndjsonBuilder.String()).Execute()
		if err != nil {
			return err
		}

		// Check for errors in bulk response
		if resp.GetErrors() {
			return fmt.Errorf("bulk operation returned errors: %s", resp.GetError())
		}

		log.Printf("Successfully bulk indexed %d documents", len(documents))
		return nil
	}, 5, 500*time.Millisecond)
}

// bulkIndexVectors indexes vectors using bulk API for vector table
func (mc *ManticoreClient) bulkIndexVectors(documents []*models.Document, vectors [][]float64) error {
	if len(documents) == 0 || len(vectors) == 0 {
		return nil
	}

	if len(documents) != len(vectors) {
		return fmt.Errorf("documents and vectors count mismatch: %d vs %d", len(documents), len(vectors))
	}

	log.Printf("Starting bulk indexing of %d vectors in 'documents_vector' table", len(documents))

	// Build NDJSON for bulk vector operation
	ndjsonBuilder := strings.Builder{}

	for i, doc := range documents {
		vectorData := map[string]interface{}{
			"title":       doc.Title,
			"url":         doc.URL,
			"vector_data": formatVectorForManticore(vectors[i]),
		}

		// Create replace operation in NDJSON format
		replaceOp := map[string]interface{}{
			"replace": map[string]interface{}{
				"index": "documents_vector",
				"id":    doc.ID,
				"doc":   vectorData,
			},
		}

		// Convert to JSON and add newline
		jsonBytes, err := json.Marshal(replaceOp)
		if err != nil {
			return fmt.Errorf("failed to marshal vector bulk operation: %v", err)
		}
		ndjsonBuilder.Write(jsonBytes)
		ndjsonBuilder.WriteByte('\n')
	}

	// Execute bulk operation with enhanced retry and circuit breaker
	return mc.retryOperationWithCircuitBreaker(func() error {
		resp, _, err := mc.client.IndexAPI.Bulk(mc.ctx).Body(ndjsonBuilder.String()).Execute()
		if err != nil {
			return err
		}

		// Check for errors in bulk response
		if resp.GetErrors() {
			return fmt.Errorf("vector bulk operation returned errors: %s", resp.GetError())
		}

		log.Printf("Successfully bulk indexed %d vectors", len(documents))
		return nil
	}, 5, 500*time.Millisecond)
}

// indexDocumentsBatchIndividually - fallback method for individual document indexing
func (mc *ManticoreClient) indexDocumentsBatchIndividually(documents []*models.Document, vectors [][]float64) error {
	for i, doc := range documents {
		// Index in full-text table
		docData := map[string]interface{}{
			// "id":      doc.ID,
			"title":   doc.Title,
			"content": doc.Content,
			"url":     doc.URL,
		}

		err := retryOperation(func() error {
			insertReq := openapi.NewInsertDocumentRequest("documents", docData)
			insertReq.SetId(uint64(doc.ID))
			_, _, err := mc.client.IndexAPI.Replace(mc.ctx).InsertDocumentRequest(*insertReq).Execute()
			return err
		}, 5, 500*time.Millisecond) // Increased attempts and base delay

		if err != nil {
			log.Printf("Warning: Failed to index document %d (%s) individually: %v", doc.ID, doc.Title, err)
			continue
		}

		// Index vector if available
		if i < len(vectors) {
			vectorData := map[string]interface{}{
				// "id":          doc.ID,
				"title":       doc.Title,
				"url":         doc.URL,
				"vector_data": formatVectorForManticore(vectors[i]),
			}

			err = retryOperation(func() error {
				vectorInsertReq := openapi.NewInsertDocumentRequest("documents_vector", vectorData)
				vectorInsertReq.SetId(uint64(doc.ID))
				_, _, err := mc.client.IndexAPI.Replace(mc.ctx).InsertDocumentRequest(*vectorInsertReq).Execute()
				return err
			}, 5, 500*time.Millisecond) // Increased attempts and base delay

			if err != nil {
				log.Printf("Warning: Failed to index vector %d individually: %v", doc.ID, err)
			}
		}
	}

	return nil
}

// GetClient returns the official API client for use in search operations
func (mc *ManticoreClient) GetClient() *openapi.APIClient {
	return mc.client
}

// GetContext returns the context
func (mc *ManticoreClient) GetContext() context.Context {
	return mc.ctx
}

// Close is a no-op for the official client (connections are managed internally)
func (mc *ManticoreClient) Close() error {
	return nil
}

// IsConnected always returns true for the official client
func (mc *ManticoreClient) IsConnected() bool {
	return true
}

// formatVectorForManticore formats a vector as a comma-separated string for Manticore
func formatVectorForManticore(vector []float64) string {
	if len(vector) == 0 {
		return ""
	}

	parts := make([]string, len(vector))
	for i, val := range vector {
		parts[i] = fmt.Sprintf("%.6f", val)
	}

	return strings.Join(parts, ",")
}
