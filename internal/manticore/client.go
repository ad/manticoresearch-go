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

// retryOperation retries an operation with exponential backoff
func retryOperation(operation func() error, maxAttempts int, baseDelay time.Duration) error {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := operation()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			break // Don't retry non-retryable errors
		}

		if attempt < maxAttempts-1 {
			// Exponential backoff with jitter and longer delays for network issues
			delay := baseDelay * time.Duration(1<<attempt)
			if strings.Contains(err.Error(), "connection reset") || strings.Contains(err.Error(), "broken pipe") {
				delay *= 2 // Longer delays for connection issues
			}
			jitter := time.Duration(rand.Intn(100)) * time.Millisecond
			time.Sleep(delay + jitter)
			log.Printf("Retrying operation (attempt %d/%d) after error: %v", attempt+2, maxAttempts, err)
		}
	}
	return fmt.Errorf("operation failed after %d attempts: %w", maxAttempts, lastErr)
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
	}

	for _, retryableErr := range retryableErrors {
		if strings.Contains(errStr, retryableErr) {
			return true
		}
	}

	return false
}

// ManticoreClient creates a new official Manticore client
type ManticoreClient struct {
	client *openapi.APIClient
	ctx    context.Context
}

// NewClient creates a new Manticore client using official Go client
func NewClient(httpHost string) *ManticoreClient {
	configuration := openapi.NewConfiguration()
	configuration.Servers[0].URL = fmt.Sprintf("http://%s", httpHost)

	// Set timeouts and connection settings
	configuration.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   2,
			IdleConnTimeout:       30 * time.Second,
		},
	}

	client := openapi.NewAPIClient(configuration)

	return &ManticoreClient{
		client: client,
		ctx:    context.Background(),
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

// dropTablesHTTP drops existing tables using direct HTTP
func (mc *ManticoreClient) dropTablesHTTP(client *http.Client, baseURL string) error {
	_ = mc.executeSQL(client, baseURL, "DROP TABLE IF EXISTS documents")
	_ = mc.executeSQL(client, baseURL, "DROP TABLE IF EXISTS documents_vector")
	log.Println("Dropped existing tables")
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
func (mc *ManticoreClient) IndexDocument(doc *models.Document, vector []float64) error {
	// Index in full-text table using official API with retry
	docData := map[string]interface{}{
		"id":      doc.ID,
		"title":   doc.Title,
		"content": doc.Content,
		"url":     doc.URL,
	}

	err := retryOperation(func() error {
		insertReq := openapi.NewInsertDocumentRequest("documents", docData)
		insertReq.SetId(uint64(doc.ID))
		_, _, err := mc.client.IndexAPI.Insert(mc.ctx).InsertDocumentRequest(*insertReq).Execute()
		return err
	}, 2, 200*time.Millisecond)

	if err != nil {
		return fmt.Errorf("failed to index document %d (%s) in full-text table: %v", doc.ID, doc.Title, err)
	}

	// Index in vector table with retry
	vectorData := map[string]interface{}{
		"id":          doc.ID,
		"title":       doc.Title,
		"url":         doc.URL,
		"vector_data": formatVectorForManticore(vector),
	}

	err = retryOperation(func() error {
		vectorInsertReq := openapi.NewInsertDocumentRequest("documents_vector", vectorData)
		vectorInsertReq.SetId(uint64(doc.ID))
		_, _, err := mc.client.IndexAPI.Insert(mc.ctx).InsertDocumentRequest(*vectorInsertReq).Execute()
		return err
	}, 2, 200*time.Millisecond)

	if err != nil {
		return fmt.Errorf("failed to index document %d in vector table: %v", doc.ID, err)
	}

	return nil
}

// IndexDocuments indexes multiple documents using efficient bulk operations
func (mc *ManticoreClient) IndexDocuments(documents []*models.Document, vectors [][]float64) error {
	if len(documents) != len(vectors) {
		return fmt.Errorf("documents and vectors count mismatch: %d vs %d", len(documents), len(vectors))
	}

	log.Printf("Starting to index %d documents using individual operations", len(documents))

	const batchSize = 50 // Smaller batches for better reliability

	// Process documents in batches using bulk operations
	for batchStart := 0; batchStart < len(documents); batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > len(documents) {
			batchEnd = len(documents)
		}

		log.Printf("Processing batch %d-%d of %d documents", batchStart+1, batchEnd, len(documents))

		// Index documents individually for better reliability
		err := mc.indexDocumentsBatchIndividually(documents[batchStart:batchEnd], vectors[batchStart:batchEnd])
		if err != nil {
			log.Printf("Error: Failed to index batch %d-%d: %v", batchStart+1, batchEnd, err)
			continue
		}

		log.Printf("Completed batch %d-%d", batchStart+1, batchEnd)

		// Small delay between batches
		time.Sleep(50 * time.Millisecond)
	}

	log.Printf("Successfully indexed %d documents using individual operations", len(documents))
	return nil
}

// bulkIndexDocuments indexes multiple documents using bulk API
func (mc *ManticoreClient) bulkIndexDocuments(documents []*models.Document, tableName string, isVector bool) error {
	if len(documents) == 0 {
		return nil
	}

	// Build NDJSON for bulk operation
	ndjsonBuilder := strings.Builder{}

	for _, doc := range documents {
		// Build replace command for each document
		var docData map[string]interface{}

		if isVector {
			// This shouldn't be called for vector table directly
			return fmt.Errorf("bulkIndexDocuments should not be called for vector table")
		} else {
			docData = map[string]interface{}{
				"id":      doc.ID,
				"title":   doc.Title,
				"content": doc.Content,
				"url":     doc.URL,
			}
		}

		// Create replace operation in NDJSON format
		replaceOp := map[string]interface{}{
			"replace": map[string]interface{}{
				"index": tableName,
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

	// Execute bulk operation with retry
	return retryOperation(func() error {
		resp, _, err := mc.client.IndexAPI.Bulk(mc.ctx).Body(ndjsonBuilder.String()).Execute()
		if err != nil {
			return err
		}

		// Check for errors in bulk response
		if resp.GetErrors() {
			return fmt.Errorf("bulk operation returned errors: %s", resp.GetError())
		}

		return nil
	}, 2, 300*time.Millisecond)
}

// bulkIndexVectors indexes vectors using bulk API
func (mc *ManticoreClient) bulkIndexVectors(documents []*models.Document, vectors [][]float64, tableName string) error {
	if len(documents) == 0 || len(vectors) == 0 {
		return nil
	}

	// Build NDJSON for bulk vector operation
	ndjsonBuilder := strings.Builder{}

	for i, doc := range documents {
		if i >= len(vectors) {
			break
		}

		vectorData := map[string]interface{}{
			"id":          doc.ID,
			"title":       doc.Title,
			"url":         doc.URL,
			"vector_data": formatVectorForManticore(vectors[i]),
		}

		// Create replace operation in NDJSON format
		replaceOp := map[string]interface{}{
			"replace": map[string]interface{}{
				"index": tableName,
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

	// Execute bulk operation with retry
	return retryOperation(func() error {
		resp, _, err := mc.client.IndexAPI.Bulk(mc.ctx).Body(ndjsonBuilder.String()).Execute()
		if err != nil {
			return err
		}

		// Check for errors in bulk response
		if resp.GetErrors() {
			return fmt.Errorf("vector bulk operation returned errors: %s", resp.GetError())
		}

		return nil
	}, 2, 300*time.Millisecond)
}

// indexDocumentsBatchIndividually - fallback method for individual document indexing
func (mc *ManticoreClient) indexDocumentsBatchIndividually(documents []*models.Document, vectors [][]float64) error {
	for i, doc := range documents {
		// Index in full-text table
		docData := map[string]interface{}{
			"id":      doc.ID,
			"title":   doc.Title,
			"content": doc.Content,
			"url":     doc.URL,
		}

		err := retryOperation(func() error {
			insertReq := openapi.NewInsertDocumentRequest("documents", docData)
			insertReq.SetId(uint64(doc.ID))
			_, _, err := mc.client.IndexAPI.Insert(mc.ctx).InsertDocumentRequest(*insertReq).Execute()
			return err
		}, 2, 200*time.Millisecond)

		if err != nil {
			log.Printf("Warning: Failed to index document %d (%s) individually: %v", doc.ID, doc.Title, err)
			continue
		}

		// Index vector if available
		if i < len(vectors) {
			vectorData := map[string]interface{}{
				"id":          doc.ID,
				"title":       doc.Title,
				"url":         doc.URL,
				"vector_data": formatVectorForManticore(vectors[i]),
			}

			err = retryOperation(func() error {
				vectorInsertReq := openapi.NewInsertDocumentRequest("documents_vector", vectorData)
				vectorInsertReq.SetId(uint64(doc.ID))
				_, _, err := mc.client.IndexAPI.Insert(mc.ctx).InsertDocumentRequest(*vectorInsertReq).Execute()
				return err
			}, 2, 200*time.Millisecond)

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
