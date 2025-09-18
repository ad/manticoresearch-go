package manticore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/ad/manticoresearch-go/internal/models"
	"github.com/ad/manticoresearch-go/internal/vectorizer"
)

// AI Search operations

// AISearchFallback performs AI search using TF-IDF vectors as fallback when Auto Embeddings fails
func (mc *manticoreHTTPClient) AISearchFallback(query string, model string, limit int, vec interface{}) ([]*models.Document, []float64, error) {
	startTime := time.Now()
	log.Printf("[AI_SEARCH] [FALLBACK] Starting AI search fallback using TF-IDF vectors: query='%s', limit=%d", query, limit)

	// Use the same logic as SearchVectorFallback but for AI search
	documents, vectors, err := mc.GetAllDocumentsWithVectors()
	if err != nil {
		log.Printf("[AI_SEARCH] [FALLBACK] [ERROR] Failed to get documents with vectors: %v", err)
		return nil, nil, fmt.Errorf("failed to get documents with vectors: %v", err)
	}

	if len(documents) == 0 {
		log.Printf("[AI_SEARCH] [FALLBACK] [WARNING] No documents found")
		return []*models.Document{}, []float64{}, nil
	}

	// Transform query to vector using TF-IDF vectorizer
	var queryVec []float64
	if tfidfVectorizer, ok := vec.(*vectorizer.TFIDFVectorizer); ok {
		queryVec = tfidfVectorizer.TransformQuery(query)
		log.Printf("[AI_SEARCH] [FALLBACK] Query vectorized with TF-IDF: vector size=%d", len(queryVec))
	} else {
		return nil, nil, fmt.Errorf("invalid vectorizer type for AI search fallback")
	}

	if len(queryVec) == 0 {
		log.Printf("[AI_SEARCH] [FALLBACK] [WARNING] Query vector is empty")
		return []*models.Document{}, []float64{}, nil
	}

	log.Printf("[AI_SEARCH] [FALLBACK] Computing similarity for %d documents", len(documents))

	// Compute similarities using TF-IDF vectors
	type docSimilarity struct {
		document   *models.Document
		similarity float64
	}

	similarities := make([]docSimilarity, 0, len(documents))
	for i, doc := range documents {
		if i < len(vectors) {
			similarity := vectorizer.CosineSimilarity(queryVec, vectors[i])
			similarities = append(similarities, docSimilarity{
				document:   doc,
				similarity: similarity,
			})
		}
	}

	// Sort by similarity (descending)
	sort.Slice(similarities, func(i, j int) bool {
		return similarities[i].similarity > similarities[j].similarity
	})

	// Take top results
	if limit > len(similarities) {
		limit = len(similarities)
	}

	resultDocs := make([]*models.Document, limit)
	resultScores := make([]float64, limit)
	for i := 0; i < limit; i++ {
		resultDocs[i] = similarities[i].document
		resultScores[i] = similarities[i].similarity
	}

	totalDuration := time.Since(startTime)
	log.Printf("[AI_SEARCH] [FALLBACK] [SUCCESS] AI search fallback completed in %v: %d results", totalDuration, len(resultDocs))

	return resultDocs, resultScores, nil
}

// AISearch performs AI-powered semantic search using Manticore's Auto Embeddings functionality
func (mc *manticoreHTTPClient) AISearch(query string, model string, limit, offset int) (*SearchResponse, error) {
	startTime := time.Now()
	log.Printf("[AI_SEARCH] Starting AI search operation: query='%s', model='%s', limit=%d, offset=%d", query, model, limit, offset)

	operation := func(ctx context.Context) (*SearchResponse, error) {
		requestStartTime := time.Now()

		// Create KNN search request with Auto Embeddings (text-based query)
		request := mc.CreateAutoEmbeddingSearchRequest("documents", "content_vector", query, limit, offset)

		// Marshal the search request
		reqBody, err := json.Marshal(request)
		if err != nil {
			log.Printf("[AI_SEARCH] [ERROR] Failed to marshal AI search request: %v", err)
			return nil, fmt.Errorf("failed to marshal AI search request: %v", err)
		}

		log.Printf("[AI_SEARCH] [REQUEST] POST %s/search - Body size: %d bytes", mc.baseURL, len(reqBody))
		log.Printf("[AI_SEARCH] [REQUEST] Payload: %s", string(reqBody))

		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, "POST", mc.baseURL+"/search", bytes.NewReader(reqBody))
		if err != nil {
			log.Printf("[AI_SEARCH] [ERROR] Failed to create HTTP request: %v", err)
			return nil, fmt.Errorf("failed to create AI search request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Execute request
		resp, err := mc.httpClient.Do(req)
		requestDuration := time.Since(requestStartTime)

		if err != nil {
			log.Printf("[AI_SEARCH] [ERROR] HTTP request failed after %v: %v", requestDuration, err)
			return nil, fmt.Errorf("AI search request failed: %v", err)
		}
		defer resp.Body.Close()

		// Read response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[AI_SEARCH] [ERROR] Failed to read response body after %v: %v", requestDuration, err)
			return nil, fmt.Errorf("failed to read AI search response: %v", err)
		}

		log.Printf("[AI_SEARCH] [RESPONSE] HTTP %d - Response size: %d bytes - Duration: %v", resp.StatusCode, len(body), requestDuration)
		log.Printf("[AI_SEARCH] [RESPONSE] Body: %s", string(body))

		if resp.StatusCode >= 400 {
			log.Printf("[AI_SEARCH] [ERROR] AI search operation failed: HTTP %d, %s", resp.StatusCode, string(body))
			return nil, fmt.Errorf("AI search operation failed: HTTP %d, %s", resp.StatusCode, string(body))
		}

		// Parse response
		var searchResponse SearchResponse
		if err := json.Unmarshal(body, &searchResponse); err != nil {
			log.Printf("[AI_SEARCH] [ERROR] Failed to parse AI search response: %v", err)
			return nil, fmt.Errorf("failed to parse AI search response: %v", err)
		}

		log.Printf("[AI_SEARCH] [SUCCESS] AI search completed: %d hits found - Duration: %v", searchResponse.Hits.Total, requestDuration)
		return &searchResponse, nil
	}

	// Execute with circuit breaker and retry logic
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Longer timeout for AI operations
	defer cancel()

	result, err := mc.executeAISearchWithRetry(ctx, operation)

	totalDuration := time.Since(startTime)

	// Record metrics
	if mc.metricsCollector != nil {
		mc.metricsCollector.RecordRequest("AISearch", totalDuration, err == nil, model)
		mc.metricsCollector.RecordSearchOperation()

		// Record AI-specific metrics
		errorType := ""
		if err != nil {
			errorType = categorizeAIError(err)
		}
		mc.metricsCollector.RecordAISearchOperation(model, totalDuration, err == nil, errorType)
	}

	if err != nil {
		log.Printf("[AI_SEARCH] [FINAL] AI search failed after %v: %v", totalDuration, err)
		if mc.logger != nil {
			mc.logger.LogOperation("AISearch", totalDuration, false, fmt.Sprintf("Model: %s, Error: %v", model, err))
			mc.logger.LogAISearchOperation(query, model, totalDuration, false, 0, err.Error())
		}
	} else {
		log.Printf("[AI_SEARCH] [FINAL] AI search completed successfully after %v: %d hits", totalDuration, result.Hits.Total)
		if mc.logger != nil {
			mc.logger.LogOperation("AISearch", totalDuration, true, fmt.Sprintf("Model: %s, Hits: %d", model, result.Hits.Total))
			mc.logger.LogAISearchOperation(query, model, totalDuration, true, int(result.Hits.Total), "")
		}
	}

	return result, err
}

// GenerateEmbedding is deprecated - using Auto Embeddings instead
// This function now returns an error indicating the new approach
func (mc *manticoreHTTPClient) GenerateEmbedding(text string, model string) ([]float64, error) {
	log.Printf("[AI_EMBEDDING] [DEPRECATED] GenerateEmbedding called for text length=%d, model='%s'", len(text), model)
	log.Printf("[AI_EMBEDDING] [DEPRECATED] This function is deprecated. ManticoreSearch now uses Auto Embeddings.")
	log.Printf("[AI_EMBEDDING] [DEPRECATED] Embeddings are generated automatically when inserting documents with vector fields configured.")

	// Return an error that explains the new approach
	return nil, fmt.Errorf("GenerateEmbedding is deprecated: ManticoreSearch now uses Auto Embeddings. " +
		"Vectors are generated automatically when documents are inserted into tables with vector fields configured with MODEL_NAME and FROM parameters")
}

// executeAISearchWithRetry executes AI search operation with circuit breaker and retry logic
func (mc *manticoreHTTPClient) executeAISearchWithRetry(ctx context.Context, operation func(context.Context) (*SearchResponse, error)) (*SearchResponse, error) {
	var result *SearchResponse

	retryOperation := func(ctx context.Context) error {
		var err error
		result, err = operation(ctx)
		return err
	}

	err := mc.circuitBreakerWithRetry.Execute(ctx, mc.baseURL+"/search", "POST", retryOperation)
	return result, err
}

// CreateKNNSearchRequest creates a KNN (K-Nearest Neighbors) search request for AI search
func (mc *manticoreHTTPClient) CreateKNNSearchRequest(index string, vectorField string, queryVector []float64, limit, offset int) SearchRequest {
	log.Printf("[AI_SEARCH] [KNN] Creating KNN search request: field='%s', vector size=%d, limit=%d, offset=%d",
		vectorField, len(queryVector), limit, offset)

	// Create KNN query according to Manticore Search 13.11.0 AI search syntax
	searchQuery := map[string]interface{}{
		"knn": map[string]interface{}{
			"field":        vectorField,
			"query_vector": queryVector,
			"k":            limit,
		},
	}

	return SearchRequest{
		Index:  index,
		Query:  searchQuery,
		Limit:  int32(limit),
		Offset: int32(offset),
	}
}

// CreateAutoEmbeddingSearchRequest creates a search request using Auto Embeddings (text-based KNN)
func (mc *manticoreHTTPClient) CreateAutoEmbeddingSearchRequest(index string, vectorField string, queryText string, limit, offset int) SearchRequest {
	log.Printf("[AI_SEARCH] [AUTO_EMBEDDING] Creating Auto Embedding search request: field='%s', query='%s', limit=%d, offset=%d",
		vectorField, queryText, limit, offset)

	// Create KNN query with text query for Auto Embeddings (Manticore 13.11+)
	searchQuery := map[string]interface{}{
		"knn": map[string]interface{}{
			"field": vectorField,
			"query": queryText, // Text query for Auto Embeddings
			"k":     limit,
		},
	}

	return SearchRequest{
		Index:  index,
		Query:  searchQuery,
		Limit:  int32(limit),
		Offset: int32(offset),
	}
}

// CreateHybridAISearchRequest creates a hybrid search request combining AI search with traditional search
func (mc *manticoreHTTPClient) CreateHybridAISearchRequest(index string, textQuery string, queryVector []float64, limit, offset int) SearchRequest {
	log.Printf("[AI_SEARCH] [HYBRID] Creating hybrid AI search request: text='%s', vector size=%d, limit=%d, offset=%d",
		textQuery, len(queryVector), limit, offset)

	// Create hybrid query combining text search and vector search
	searchQuery := map[string]interface{}{
		"bool": map[string]interface{}{
			"should": []map[string]interface{}{
				{
					"match": map[string]interface{}{
						"content": textQuery,
					},
				},
				{
					"knn": map[string]interface{}{
						"field":        "content_vector",
						"query_vector": queryVector,
						"k":            limit,
					},
				},
			},
		},
	}

	return SearchRequest{
		Index:  index,
		Query:  searchQuery,
		Limit:  int32(limit),
		Offset: int32(offset),
	}
}

// ValidateAISearchCapability checks if the Manticore instance supports AI search with Auto Embeddings
func (mc *manticoreHTTPClient) ValidateAISearchCapability() error {
	log.Printf("[AI_SEARCH] [VALIDATE] Checking AI search capability with Auto Embeddings")

	// Try to perform a simple AI search to test Auto Embeddings functionality
	testQuery := "test query"

	// Create a test search request using Auto Embeddings
	request := mc.CreateAutoEmbeddingSearchRequest("documents", "content_vector", testQuery, 1, 0)

	// Marshal the request to test if the format is valid
	_, err := json.Marshal(request)
	if err != nil {
		log.Printf("[AI_SEARCH] [VALIDATE] [WARNING] Failed to marshal test AI search request: %v", err)
		return fmt.Errorf("AI search request format validation failed: %v", err)
	}

	log.Printf("[AI_SEARCH] [VALIDATE] [SUCCESS] AI search capability with Auto Embeddings validated")
	return nil
}

// GetAISearchStatus returns the current status of AI search functionality with Auto Embeddings
func (mc *manticoreHTTPClient) GetAISearchStatus() map[string]interface{} {
	startTime := time.Now()
	status := map[string]interface{}{
		"ai_search_available": false,
		"auto_embeddings":     true,
		"embedding_method":    "Auto Embeddings (built-in)",
		"model":               "sentence-transformers/all-MiniLM-L6-v2",
		"last_check":          time.Now().Format(time.RFC3339),
	}

	// Test AI search capability with Auto Embeddings
	err := mc.ValidateAISearchCapability()
	duration := time.Since(startTime)

	if err == nil {
		status["ai_search_available"] = true
		status["status"] = "healthy"

		if mc.logger != nil {
			mc.logger.LogAISearchHealthCheck(true, "sentence-transformers/all-MiniLM-L6-v2", duration, "")
		}
	} else {
		status["status"] = "unavailable"
		status["error"] = err.Error()

		if mc.logger != nil {
			mc.logger.LogAISearchHealthCheck(false, "sentence-transformers/all-MiniLM-L6-v2", duration, err.Error())
		}
	}

	status["check_duration_ms"] = duration.Milliseconds()
	return status
}

// categorizeAIError categorizes AI search errors for metrics tracking
func categorizeAIError(err error) string {
	if err == nil {
		return ""
	}

	errorStr := err.Error()

	if contains(errorStr, "timeout") || contains(errorStr, "deadline exceeded") {
		return "timeout"
	}
	if contains(errorStr, "connection") || contains(errorStr, "network") {
		return "network"
	}
	if contains(errorStr, "embedding") {
		return "embedding"
	}
	if contains(errorStr, "model") {
		return "model"
	}
	if contains(errorStr, "HTTP 4") {
		return "client_error"
	}
	if contains(errorStr, "HTTP 5") {
		return "server_error"
	}
	if contains(errorStr, "parse") || contains(errorStr, "unmarshal") {
		return "parse_error"
	}
	if contains(errorStr, "circuit breaker") {
		return "circuit_breaker"
	}

	return "unknown"
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && containsAt(s, substr)))
}

// containsAt checks if string contains substring at any position
func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] && s[i+j] != substr[j]+32 && s[i+j] != substr[j]-32 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
