package manticore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ad/manticoresearch-go/internal/models"
)

// Document indexing operations

// IndexDocument indexes a single document in unified table with Auto Embeddings
func (mc *manticoreHTTPClient) IndexDocument(doc *models.Document, vector []float64) error {
	startTime := time.Now()
	log.Printf("[INDEX] [SINGLE] Starting document indexing with Auto Embeddings: ID=%d, Title='%s'", doc.ID, doc.Title)

	// Index in unified documents table (Auto Embeddings will generate vectors automatically)
	if err := mc.indexDocumentUnified(doc); err != nil {
		log.Printf("[INDEX] [SINGLE] [ERROR] Failed to index document in unified table after %v: %v", time.Since(startTime), err)
		return fmt.Errorf("failed to index document with Auto Embeddings: %v", err)
	}

	totalDuration := time.Since(startTime)

	// Record metrics
	if mc.metricsCollector != nil {
		mc.metricsCollector.RecordRequest("IndexDocument", totalDuration, true, "")
		mc.metricsCollector.RecordIndexOperation()
	}

	if mc.logger != nil {
		mc.logger.LogOperation("IndexDocument", totalDuration, true, fmt.Sprintf("ID=%d, Title='%s'", doc.ID, doc.Title))
	}

	log.Printf("[INDEX] [SINGLE] [SUCCESS] Document indexed successfully with Auto Embeddings in %v: ID=%d", totalDuration, doc.ID)
	return nil
}

// indexDocumentUnified indexes a document in the unified table with Auto Embeddings using /replace endpoint
func (mc *manticoreHTTPClient) indexDocumentUnified(doc *models.Document) error {
	operation := func(ctx context.Context) error {
		requestStartTime := time.Now()

		// Create replace request for unified documents table with Auto Embeddings
		// Note: content_vector field will be populated automatically by ManticoreSearch
		replaceReq := ReplaceRequest{
			Index: "documents",
			ID:    int64(doc.ID),
			Doc: map[string]interface{}{
				"title":   doc.Title,
				"content": doc.Content,
				"url":     doc.URL,
				// content_vector field is omitted - it will be generated automatically from title+content
			},
		}

		reqBody, err := json.Marshal(replaceReq)
		if err != nil {
			log.Printf("[INDEX] [UNIFIED] [ERROR] Failed to marshal replace request for doc ID=%d: %v", doc.ID, err)
			return fmt.Errorf("failed to marshal replace request: %v", err)
		}

		log.Printf("[INDEX] [UNIFIED] [REQUEST] POST %s/replace - Doc ID=%d, Body size: %d bytes (Auto Embeddings)", mc.baseURL, doc.ID, len(reqBody))
		log.Printf("[INDEX] [UNIFIED] [REQUEST] Payload: %s", string(reqBody))

		req, err := http.NewRequestWithContext(ctx, "POST", mc.baseURL+"/replace", bytes.NewReader(reqBody))
		if err != nil {
			log.Printf("[INDEX] [UNIFIED] [ERROR] Failed to create HTTP request for doc ID=%d: %v", doc.ID, err)
			return fmt.Errorf("failed to create replace request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := mc.httpClient.Do(req)
		requestDuration := time.Since(requestStartTime)

		if err != nil {
			log.Printf("[INDEX] [UNIFIED] [ERROR] HTTP request failed for doc ID=%d after %v: %v", doc.ID, requestDuration, err)
			return fmt.Errorf("replace request failed: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[INDEX] [UNIFIED] [ERROR] Failed to read response body for doc ID=%d after %v: %v", doc.ID, requestDuration, err)
			return fmt.Errorf("failed to read replace response: %v", err)
		}

		log.Printf("[INDEX] [UNIFIED] [RESPONSE] HTTP %d - Response size: %d bytes - Duration: %v", resp.StatusCode, len(body), requestDuration)
		log.Printf("[INDEX] [UNIFIED] [RESPONSE] Body: %s", string(body))

		if resp.StatusCode >= 400 {
			log.Printf("[INDEX] [UNIFIED] [ERROR] Replace operation failed for doc ID=%d: HTTP %d, %s", doc.ID, resp.StatusCode, string(body))
			return fmt.Errorf("replace operation failed: HTTP %d, %s", resp.StatusCode, string(body))
		}

		log.Printf("[INDEX] [UNIFIED] [SUCCESS] Document indexed with Auto Embeddings: ID=%d - Duration: %v", doc.ID, requestDuration)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return mc.circuitBreakerWithRetry.Execute(ctx, mc.baseURL+"/replace", "POST", operation)
}

// indexDocumentFullText indexes a document in the full-text search table using /replace endpoint
// DEPRECATED: This function is kept for compatibility, but indexDocumentUnified should be used instead
func (mc *manticoreHTTPClient) indexDocumentFullText(doc *models.Document) error {
	log.Printf("[INDEX] [FULLTEXT] [DEPRECATED] Using deprecated indexDocumentFullText for doc ID=%d", doc.ID)
	return mc.indexDocumentUnified(doc)
}

// indexDocumentVector indexes a document in the vector search table using /replace endpoint
func (mc *manticoreHTTPClient) indexDocumentVector(doc *models.Document, vector []float64) error {
	operation := func(ctx context.Context) error {
		requestStartTime := time.Now()

		// Format vector as JSON array string for Manticore
		vectorStr := formatVectorAsJSONArray(vector)

		// Create replace request for vector table
		replaceReq := ReplaceRequest{
			Index: "documents_vector",
			ID:    int64(doc.ID),
			Doc: map[string]interface{}{
				"title":       doc.Title,
				"url":         doc.URL,
				"vector_data": vectorStr,
			},
		}

		reqBody, err := json.Marshal(replaceReq)
		if err != nil {
			log.Printf("[INDEX] [VECTOR] [ERROR] Failed to marshal replace request for doc ID=%d: %v", doc.ID, err)
			return fmt.Errorf("failed to marshal vector replace request: %v", err)
		}

		log.Printf("[INDEX] [VECTOR] [REQUEST] POST %s/replace - Doc ID=%d, Vector size: %d, Body size: %d bytes", mc.baseURL, doc.ID, len(vector), len(reqBody))
		log.Printf("[INDEX] [VECTOR] [REQUEST] Payload: %s", string(reqBody))

		req, err := http.NewRequestWithContext(ctx, "POST", mc.baseURL+"/replace", bytes.NewReader(reqBody))
		if err != nil {
			log.Printf("[INDEX] [VECTOR] [ERROR] Failed to create HTTP request for doc ID=%d: %v", doc.ID, err)
			return fmt.Errorf("failed to create vector replace request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := mc.httpClient.Do(req)
		requestDuration := time.Since(requestStartTime)

		if err != nil {
			log.Printf("[INDEX] [VECTOR] [ERROR] HTTP request failed for doc ID=%d after %v: %v", doc.ID, requestDuration, err)
			return fmt.Errorf("vector replace request failed: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[INDEX] [VECTOR] [ERROR] Failed to read response body for doc ID=%d after %v: %v", doc.ID, requestDuration, err)
			return fmt.Errorf("failed to read vector replace response: %v", err)
		}

		log.Printf("[INDEX] [VECTOR] [RESPONSE] HTTP %d - Response size: %d bytes - Duration: %v", resp.StatusCode, len(body), requestDuration)
		log.Printf("[INDEX] [VECTOR] [RESPONSE] Body: %s", string(body))

		if resp.StatusCode >= 400 {
			log.Printf("[INDEX] [VECTOR] [ERROR] Vector replace operation failed for doc ID=%d: HTTP %d, %s", doc.ID, resp.StatusCode, string(body))
			return fmt.Errorf("vector replace operation failed: HTTP %d, %s", resp.StatusCode, string(body))
		}

		log.Printf("[INDEX] [VECTOR] [SUCCESS] Document indexed in vector table: ID=%d - Duration: %v", doc.ID, requestDuration)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return mc.circuitBreakerWithRetry.Execute(ctx, mc.baseURL+"/replace", "POST", operation)
}

// IndexDocuments indexes multiple documents using efficient bulk operations with optimization
func (mc *manticoreHTTPClient) IndexDocuments(documents []*models.Document, vectors [][]float64) error {
	if len(documents) == 0 {
		log.Printf("[INDEX] [BULK] No documents to index")
		return nil
	}

	startTime := time.Now()
	log.Printf("[INDEX] [BULK] Starting optimized bulk document indexing: %d documents", len(documents))

	// Validate vectors length matches documents length if provided
	if len(vectors) > 0 && len(vectors) != len(documents) {
		return fmt.Errorf("vectors length (%d) does not match documents length (%d)", len(vectors), len(documents))
	}

	var err error
	// Choose indexing strategy based on document count and configuration
	if len(documents) >= mc.bulkConfig.StreamingThreshold {
		log.Printf("[INDEX] [BULK] Using streaming batch processing for %d documents (threshold: %d)", len(documents), mc.bulkConfig.StreamingThreshold)
		err = mc.streamingBulkIndex(documents, vectors)
	} else if len(documents) > mc.bulkConfig.BatchSize {
		log.Printf("[INDEX] [BULK] Using batch processing for %d documents (batch size: %d)", len(documents), mc.bulkConfig.BatchSize)
		err = mc.batchedBulkIndex(documents, vectors)
	} else {
		log.Printf("[INDEX] [BULK] Using single bulk operation for %d documents", len(documents))
		err = mc.singleBulkIndex(documents, vectors)
	}

	totalDuration := time.Since(startTime)

	// Record metrics
	if mc.metricsCollector != nil {
		mc.metricsCollector.RecordRequest("IndexDocuments", totalDuration, err == nil, "")
		mc.metricsCollector.RecordBulkOperation(len(documents))
	}

	if err != nil {
		log.Printf("[INDEX] [BULK] [FINAL] Bulk indexing failed after %v: %v", totalDuration, err)
		if mc.logger != nil {
			mc.logger.LogOperation("IndexDocuments", totalDuration, false, fmt.Sprintf("%d documents, Error: %v", len(documents), err))
		}
	} else {
		log.Printf("[INDEX] [BULK] [FINAL] Bulk indexing completed successfully in %v: %d documents", totalDuration, len(documents))
		if mc.logger != nil {
			mc.logger.LogBulkOperation("IndexDocuments", len(documents), len(documents), totalDuration)
		}
	}

	return err
}

// formatVectorAsJSONArray formats a vector as a JSON array string
func formatVectorAsJSONArray(vector []float64) string {
	if len(vector) == 0 {
		return "[]"
	}

	parts := make([]string, len(vector))
	for i, val := range vector {
		parts[i] = fmt.Sprintf("%.6f", val)
	}

	return "[" + strings.Join(parts, ",") + "]"
}
