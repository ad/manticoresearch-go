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

// IndexDocument indexes a single document in both full-text and vector tables
func (mc *manticoreHTTPClient) IndexDocument(doc *models.Document, vector []float64) error {
	startTime := time.Now()
	log.Printf("[INDEX] [SINGLE] Starting document indexing: ID=%d, Title='%s'", doc.ID, doc.Title)

	// Index in full-text table first
	if err := mc.indexDocumentFullText(doc); err != nil {
		log.Printf("[INDEX] [SINGLE] [ERROR] Failed to index document in full-text table after %v: %v", time.Since(startTime), err)
		return fmt.Errorf("failed to index document in full-text table: %v", err)
	}

	// Index in vector table if vector data is provided
	if len(vector) > 0 {
		if err := mc.indexDocumentVector(doc, vector); err != nil {
			log.Printf("[INDEX] [SINGLE] [ERROR] Failed to index document in vector table after %v: %v", time.Since(startTime), err)
			return fmt.Errorf("failed to index document in vector table: %v", err)
		}
	} else {
		log.Printf("[INDEX] [SINGLE] [WARNING] No vector data provided for document ID=%d, skipping vector indexing", doc.ID)
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

	log.Printf("[INDEX] [SINGLE] [SUCCESS] Document indexed successfully in %v: ID=%d", totalDuration, doc.ID)
	return nil
}

// indexDocumentFullText indexes a document in the full-text search table using /replace endpoint
func (mc *manticoreHTTPClient) indexDocumentFullText(doc *models.Document) error {
	operation := func(ctx context.Context) error {
		requestStartTime := time.Now()

		// Create replace request for full-text table
		replaceReq := ReplaceRequest{
			Index: "documents",
			ID:    int64(doc.ID),
			Doc: map[string]interface{}{
				"title":   doc.Title,
				"content": doc.Content,
				"url":     doc.URL,
			},
		}

		reqBody, err := json.Marshal(replaceReq)
		if err != nil {
			log.Printf("[INDEX] [FULLTEXT] [ERROR] Failed to marshal replace request for doc ID=%d: %v", doc.ID, err)
			return fmt.Errorf("failed to marshal replace request: %v", err)
		}

		log.Printf("[INDEX] [FULLTEXT] [REQUEST] POST %s/replace - Doc ID=%d, Body size: %d bytes", mc.baseURL, doc.ID, len(reqBody))
		log.Printf("[INDEX] [FULLTEXT] [REQUEST] Payload: %s", string(reqBody))

		req, err := http.NewRequestWithContext(ctx, "POST", mc.baseURL+"/replace", bytes.NewReader(reqBody))
		if err != nil {
			log.Printf("[INDEX] [FULLTEXT] [ERROR] Failed to create HTTP request for doc ID=%d: %v", doc.ID, err)
			return fmt.Errorf("failed to create replace request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := mc.httpClient.Do(req)
		requestDuration := time.Since(requestStartTime)

		if err != nil {
			log.Printf("[INDEX] [FULLTEXT] [ERROR] HTTP request failed for doc ID=%d after %v: %v", doc.ID, requestDuration, err)
			return fmt.Errorf("replace request failed: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[INDEX] [FULLTEXT] [ERROR] Failed to read response body for doc ID=%d after %v: %v", doc.ID, requestDuration, err)
			return fmt.Errorf("failed to read replace response: %v", err)
		}

		log.Printf("[INDEX] [FULLTEXT] [RESPONSE] HTTP %d - Response size: %d bytes - Duration: %v", resp.StatusCode, len(body), requestDuration)
		log.Printf("[INDEX] [FULLTEXT] [RESPONSE] Body: %s", string(body))

		if resp.StatusCode >= 400 {
			log.Printf("[INDEX] [FULLTEXT] [ERROR] Replace operation failed for doc ID=%d: HTTP %d, %s", doc.ID, resp.StatusCode, string(body))
			return fmt.Errorf("replace operation failed: HTTP %d, %s", resp.StatusCode, string(body))
		}

		log.Printf("[INDEX] [FULLTEXT] [SUCCESS] Document indexed in full-text table: ID=%d - Duration: %v", doc.ID, requestDuration)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return mc.circuitBreakerWithRetry.Execute(ctx, mc.baseURL+"/replace", "POST", operation)
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
