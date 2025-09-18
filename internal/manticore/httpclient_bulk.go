package manticore

import (
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

// Bulk operations for efficient document indexing

// singleBulkIndex performs a single bulk operation for small document sets
func (mc *manticoreHTTPClient) singleBulkIndex(documents []*models.Document, vectors [][]float64) error {
	startTime := time.Now()

	// Try bulk operations first, fallback to individual operations on failure
	if err := mc.bulkIndexDocuments(documents, vectors); err != nil {
		log.Printf("[INDEX] [BULK] [WARNING] Bulk operation failed, falling back to individual operations: %v", err)
		return mc.fallbackToIndividualIndexing(documents, vectors)
	}

	totalDuration := time.Since(startTime)
	log.Printf("[INDEX] [BULK] [SUCCESS] Single bulk indexing completed successfully in %v: %d documents", totalDuration, len(documents))
	return nil
}

// batchedBulkIndex processes documents in batches for medium-sized document sets
func (mc *manticoreHTTPClient) batchedBulkIndex(documents []*models.Document, vectors [][]float64) error {
	startTime := time.Now()
	batchSize := mc.bulkConfig.BatchSize
	totalBatches := (len(documents) + batchSize - 1) / batchSize

	log.Printf("[INDEX] [BULK] [BATCHED] Processing %d documents in %d batches of size %d", len(documents), totalBatches, batchSize)

	successfulBatches := 0
	var lastError error

	for i := 0; i < len(documents); i += batchSize {
		batchStart := i
		batchEnd := i + batchSize
		if batchEnd > len(documents) {
			batchEnd = len(documents)
		}

		batchDocs := documents[batchStart:batchEnd]
		var batchVectors [][]float64
		if len(vectors) > 0 {
			batchVectors = vectors[batchStart:batchEnd]
		}

		batchNum := (i / batchSize) + 1
		log.Printf("[INDEX] [BULK] [BATCHED] Processing batch %d/%d: documents %d-%d", batchNum, totalBatches, batchStart+1, batchEnd)

		if err := mc.bulkIndexDocuments(batchDocs, batchVectors); err != nil {
			log.Printf("[INDEX] [BULK] [BATCHED] [WARNING] Batch %d failed, falling back to individual operations: %v", batchNum, err)
			if err := mc.fallbackToIndividualIndexing(batchDocs, batchVectors); err != nil {
				log.Printf("[INDEX] [BULK] [BATCHED] [ERROR] Individual fallback also failed for batch %d: %v", batchNum, err)
				lastError = err
				continue
			}
		}

		successfulBatches++
		log.Printf("[INDEX] [BULK] [BATCHED] Completed batch %d/%d", batchNum, totalBatches)

		// Small delay between batches to avoid overwhelming the server
		time.Sleep(100 * time.Millisecond)
	}

	totalDuration := time.Since(startTime)
	log.Printf("[INDEX] [BULK] [BATCHED] [SUCCESS] Batched indexing completed in %v: %d/%d batches successful", totalDuration, successfulBatches, totalBatches)

	return lastError
}

// streamingBulkIndex processes documents using streaming approach for large document sets
func (mc *manticoreHTTPClient) streamingBulkIndex(documents []*models.Document, vectors [][]float64) error {
	startTime := time.Now()
	batchSize := mc.bulkConfig.BatchSize
	maxConcurrent := mc.bulkConfig.MaxConcurrentBatch
	progressInterval := mc.bulkConfig.ProgressLogInterval

	log.Printf("[INDEX] [BULK] [STREAMING] Processing %d documents with streaming approach (batch size: %d, max concurrent: %d)", len(documents), batchSize, maxConcurrent)

	// Channel for batch processing
	batchChan := make(chan batchJob, maxConcurrent)
	resultChan := make(chan batchResult, maxConcurrent)

	// Start worker goroutines
	for i := 0; i < maxConcurrent; i++ {
		go mc.batchWorker(batchChan, resultChan)
	}

	// Send batches to workers
	totalBatches := (len(documents) + batchSize - 1) / batchSize
	go func() {
		defer close(batchChan)
		for i := 0; i < len(documents); i += batchSize {
			batchStart := i
			batchEnd := i + batchSize
			if batchEnd > len(documents) {
				batchEnd = len(documents)
			}

			batchDocs := documents[batchStart:batchEnd]
			var batchVectors [][]float64
			if len(vectors) > 0 {
				batchVectors = vectors[batchStart:batchEnd]
			}

			batchChan <- batchJob{
				documents: batchDocs,
				vectors:   batchVectors,
				batchNum:  (i / batchSize) + 1,
				total:     totalBatches,
			}
		}
	}()

	// Collect results
	successfulBatches := 0
	processedDocuments := 0
	var lastError error

	for i := 0; i < totalBatches; i++ {
		result := <-resultChan
		if result.err != nil {
			log.Printf("[INDEX] [BULK] [STREAMING] [ERROR] Batch %d failed: %v", result.batchNum, result.err)
			lastError = result.err
		} else {
			successfulBatches++
		}

		processedDocuments += result.documentCount
		if processedDocuments%progressInterval == 0 || processedDocuments == len(documents) {
			log.Printf("[INDEX] [BULK] [STREAMING] [PROGRESS] Processed %d/%d documents (%d%% complete)", processedDocuments, len(documents), (processedDocuments*100)/len(documents))
		}
	}

	totalDuration := time.Since(startTime)
	log.Printf("[INDEX] [BULK] [STREAMING] [SUCCESS] Streaming indexing completed in %v: %d/%d batches successful, %d documents processed", totalDuration, successfulBatches, totalBatches, processedDocuments)

	return lastError
}

// batchJob represents a batch processing job
type batchJob struct {
	documents []*models.Document
	vectors   [][]float64
	batchNum  int
	total     int
}

// batchResult represents the result of a batch processing job
type batchResult struct {
	batchNum      int
	documentCount int
	err           error
}

// batchWorker processes batch jobs
func (mc *manticoreHTTPClient) batchWorker(jobs <-chan batchJob, results chan<- batchResult) {
	for job := range jobs {
		log.Printf("[INDEX] [BULK] [STREAMING] [WORKER] Processing batch %d/%d with %d documents", job.batchNum, job.total, len(job.documents))

		err := mc.bulkIndexDocuments(job.documents, job.vectors)
		if err != nil {
			log.Printf("[INDEX] [BULK] [STREAMING] [WORKER] Batch %d failed, trying individual fallback", job.batchNum)
			err = mc.fallbackToIndividualIndexing(job.documents, job.vectors)
		}

		results <- batchResult{
			batchNum:      job.batchNum,
			documentCount: len(job.documents),
			err:           err,
		}
	}
}

// bulkIndexDocuments performs bulk indexing using the /bulk endpoint with NDJSON format
func (mc *manticoreHTTPClient) bulkIndexDocuments(documents []*models.Document, vectors [][]float64) error {
	// Index documents in unified table with Auto Embeddings (vectors will be generated automatically)
	if err := mc.bulkIndexUnified(documents); err != nil {
		return fmt.Errorf("bulk unified indexing with Auto Embeddings failed: %v", err)
	}

	// Also index documents with TF-IDF vectors in documents_vector table (if vectors provided)
	if len(vectors) > 0 {
		if err := mc.bulkIndexVectors(documents, vectors); err != nil {
			log.Printf("[INDEX] [BULK] [WARNING] Vector indexing failed, but unified indexing succeeded: %v", err)
			// Don't fail the whole operation if vector indexing fails
		}
	}

	return nil
}

// bulkIndexUnified performs bulk indexing for documents with Auto Embeddings using NDJSON format
func (mc *manticoreHTTPClient) bulkIndexUnified(documents []*models.Document) error {
	if len(documents) == 0 {
		return nil
	}

	operation := func(ctx context.Context) error {
		requestStartTime := time.Now()

		// Build NDJSON payload for bulk operation
		var ndjsonBuilder strings.Builder
		for _, doc := range documents {
			bulkReq := map[string]interface{}{
				"replace": map[string]interface{}{
					"index": "documents",
					"id":    doc.ID,
					"doc": map[string]interface{}{
						"title":   doc.Title,
						"content": doc.Content,
						"url":     doc.URL,
					},
				},
			}

			jsonBytes, err := json.Marshal(bulkReq)
			if err != nil {
				return fmt.Errorf("failed to marshal bulk request: %v", err)
			}
			ndjsonBuilder.Write(jsonBytes)
			ndjsonBuilder.WriteByte('\n')
		}

		payload := ndjsonBuilder.String()
		log.Printf("[INDEX] [BULK] [UNIFIED] [REQUEST] POST %s/bulk - Documents: %d, Body size: %d bytes (Auto Embeddings)", mc.baseURL, len(documents), len(payload))
		log.Printf("[INDEX] [BULK] [UNIFIED] [REQUEST] Sample payload (first 500 chars): %s", truncateString(payload, 500))

		req, err := http.NewRequestWithContext(ctx, "POST", mc.baseURL+"/bulk", strings.NewReader(payload))
		if err != nil {
			return fmt.Errorf("failed to create bulk request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-ndjson")

		resp, err := mc.httpClient.Do(req)
		requestDuration := time.Since(requestStartTime)

		if err != nil {
			log.Printf("[INDEX] [BULK] [UNIFIED] [ERROR] HTTP request failed after %v: %v", requestDuration, err)
			return fmt.Errorf("bulk request failed: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[INDEX] [BULK] [UNIFIED] [ERROR] Failed to read response body after %v: %v", requestDuration, err)
			return fmt.Errorf("failed to read bulk response: %v", err)
		}

		log.Printf("[INDEX] [BULK] [UNIFIED] [RESPONSE] HTTP %d - Response size: %d bytes - Duration: %v", resp.StatusCode, len(body), requestDuration)
		log.Printf("[INDEX] [BULK] [UNIFIED] [RESPONSE] Body: %s", string(body))

		if resp.StatusCode >= 400 {
			log.Printf("[INDEX] [BULK] [UNIFIED] [ERROR] Bulk operation failed: HTTP %d, %s", resp.StatusCode, string(body))
			return fmt.Errorf("bulk operation failed: HTTP %d, %s", resp.StatusCode, string(body))
		}

		// Parse response to check for individual item errors
		var bulkResponse BulkResponse
		if err := json.Unmarshal(body, &bulkResponse); err == nil {
			if bulkResponse.Errors {
				// Log individual item errors but don't fail the entire operation
				errorCount := 0
				for i, item := range bulkResponse.Items {
					if item.Replace != nil && item.Replace.Error != "" {
						log.Printf("[INDEX] [BULK] [UNIFIED] [ERROR] Item %d failed: %s", i, item.Replace.Error)
						errorCount++
					}
				}
				if errorCount > 0 {
					log.Printf("[INDEX] [BULK] [UNIFIED] [WARNING] %d out of %d items had errors", errorCount, len(documents))
				}
			}
		}

		log.Printf("[INDEX] [BULK] [UNIFIED] [SUCCESS] Bulk indexing with Auto Embeddings completed: %d documents - Duration: %v", len(documents), requestDuration)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), mc.bulkConfig.BatchTimeout)
	defer cancel()

	return mc.circuitBreakerWithRetry.Execute(ctx, mc.baseURL+"/bulk", "POST", operation)
}

// bulkIndexVectors performs bulk indexing for vector documents using NDJSON format
func (mc *manticoreHTTPClient) bulkIndexVectors(documents []*models.Document, vectors [][]float64) error {
	if len(documents) == 0 || len(vectors) == 0 {
		return nil
	}

	if len(documents) != len(vectors) {
		return fmt.Errorf("documents and vectors count mismatch: %d vs %d", len(documents), len(vectors))
	}

	operation := func(ctx context.Context) error {
		requestStartTime := time.Now()

		// Build NDJSON payload for bulk vector operation
		var ndjsonBuilder strings.Builder
		for i, doc := range documents {
			vectorStr := formatVectorAsJSONArray(vectors[i])

			bulkReq := map[string]interface{}{
				"replace": map[string]interface{}{
					"index": "documents_vector",
					"id":    doc.ID,
					"doc": map[string]interface{}{
						"title":       doc.Title,
						"url":         doc.URL,
						"vector_data": vectorStr,
					},
				},
			}

			jsonBytes, err := json.Marshal(bulkReq)
			if err != nil {
				return fmt.Errorf("failed to marshal vector bulk request: %v", err)
			}
			ndjsonBuilder.Write(jsonBytes)
			ndjsonBuilder.WriteByte('\n')
		}

		payload := ndjsonBuilder.String()
		log.Printf("[INDEX] [BULK] [VECTOR] [REQUEST] POST %s/bulk - Documents: %d, Body size: %d bytes", mc.baseURL, len(documents), len(payload))
		log.Printf("[INDEX] [BULK] [VECTOR] [REQUEST] Sample payload (first 500 chars): %s", truncateString(payload, 500))

		req, err := http.NewRequestWithContext(ctx, "POST", mc.baseURL+"/bulk", strings.NewReader(payload))
		if err != nil {
			return fmt.Errorf("failed to create vector bulk request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-ndjson")

		resp, err := mc.httpClient.Do(req)
		requestDuration := time.Since(requestStartTime)

		if err != nil {
			log.Printf("[INDEX] [BULK] [VECTOR] [ERROR] HTTP request failed after %v: %v", requestDuration, err)
			return fmt.Errorf("vector bulk request failed: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[INDEX] [BULK] [VECTOR] [ERROR] Failed to read response body after %v: %v", requestDuration, err)
			return fmt.Errorf("failed to read vector bulk response: %v", err)
		}

		log.Printf("[INDEX] [BULK] [VECTOR] [RESPONSE] HTTP %d - Response size: %d bytes - Duration: %v", resp.StatusCode, len(body), requestDuration)
		log.Printf("[INDEX] [BULK] [VECTOR] [RESPONSE] Body: %s", string(body))

		if resp.StatusCode >= 400 {
			log.Printf("[INDEX] [BULK] [VECTOR] [ERROR] Vector bulk operation failed: HTTP %d, %s", resp.StatusCode, string(body))
			return fmt.Errorf("vector bulk operation failed: HTTP %d, %s", resp.StatusCode, string(body))
		}

		// Parse response to check for individual item errors
		var bulkResponse BulkResponse
		if err := json.Unmarshal(body, &bulkResponse); err == nil {
			if bulkResponse.Errors {
				// Log individual item errors but don't fail the entire operation
				errorCount := 0
				for i, item := range bulkResponse.Items {
					if item.Replace != nil && item.Replace.Error != "" {
						log.Printf("[INDEX] [BULK] [VECTOR] [ERROR] Item %d failed: %s", i, item.Replace.Error)
						errorCount++
					}
				}
				if errorCount > 0 {
					log.Printf("[INDEX] [BULK] [VECTOR] [WARNING] %d out of %d items had errors", errorCount, len(documents))
				}
			}
		}

		log.Printf("[INDEX] [BULK] [VECTOR] [SUCCESS] Bulk indexing completed: %d documents - Duration: %v", len(documents), requestDuration)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), mc.bulkConfig.BatchTimeout)
	defer cancel()

	return mc.circuitBreakerWithRetry.Execute(ctx, mc.baseURL+"/bulk", "POST", operation)
}

// fallbackToIndividualIndexing falls back to individual document indexing when bulk operations fail
func (mc *manticoreHTTPClient) fallbackToIndividualIndexing(documents []*models.Document, vectors [][]float64) error {
	log.Printf("[INDEX] [FALLBACK] Starting individual indexing fallback for %d documents", len(documents))

	var lastError error
	successCount := 0

	for i, doc := range documents {
		var vector []float64
		if i < len(vectors) {
			vector = vectors[i]
		}

		if err := mc.IndexDocument(doc, vector); err != nil {
			log.Printf("[INDEX] [FALLBACK] [ERROR] Failed to index document %d individually: %v", doc.ID, err)
			lastError = err
		} else {
			successCount++
		}

		// Small delay between individual operations
		time.Sleep(50 * time.Millisecond)
	}

	log.Printf("[INDEX] [FALLBACK] [FINAL] Individual indexing completed: %d/%d documents successful", successCount, len(documents))
	return lastError
}

// bulkIndexFullText is a deprecated wrapper for bulkIndexUnified
// DEPRECATED: Use bulkIndexUnified instead. This is kept for compatibility.
func (mc *manticoreHTTPClient) bulkIndexFullText(documents []*models.Document) error {
	log.Printf("[INDEX] [BULK] [FULLTEXT] [DEPRECATED] Using deprecated bulkIndexFullText, redirecting to bulkIndexUnified")
	return mc.bulkIndexUnified(documents)
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
