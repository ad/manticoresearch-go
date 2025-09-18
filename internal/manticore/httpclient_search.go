package manticore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ad/manticoresearch-go/internal/models"
)

// Search operations

// SearchWithRequest performs search operations using the JSON API with comprehensive logging
func (mc *manticoreHTTPClient) SearchWithRequest(request SearchRequest) (*SearchResponse, error) {
	startTime := time.Now()
	log.Printf("[SEARCH] Starting search operation: index='%s', limit=%d, offset=%d", request.Index, request.Limit, request.Offset)

	operation := func(ctx context.Context) (*SearchResponse, error) {
		requestStartTime := time.Now()

		// Marshal the search request
		reqBody, err := json.Marshal(request)
		if err != nil {
			log.Printf("[SEARCH] [ERROR] Failed to marshal search request: %v", err)
			return nil, fmt.Errorf("failed to marshal search request: %v", err)
		}

		log.Printf("[SEARCH] [REQUEST] POST %s/search - Body size: %d bytes", mc.baseURL, len(reqBody))
		log.Printf("[SEARCH] [REQUEST] Payload: %s", string(reqBody))

		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, "POST", mc.baseURL+"/search", bytes.NewReader(reqBody))
		if err != nil {
			log.Printf("[SEARCH] [ERROR] Failed to create HTTP request: %v", err)
			return nil, fmt.Errorf("failed to create search request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Execute request
		resp, err := mc.httpClient.Do(req)
		requestDuration := time.Since(requestStartTime)

		if err != nil {
			log.Printf("[SEARCH] [ERROR] HTTP request failed after %v: %v", requestDuration, err)
			return nil, fmt.Errorf("search request failed: %v", err)
		}
		defer resp.Body.Close()

		// Read response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[SEARCH] [ERROR] Failed to read response body after %v: %v", requestDuration, err)
			return nil, fmt.Errorf("failed to read search response: %v", err)
		}

		log.Printf("[SEARCH] [RESPONSE] HTTP %d - Response size: %d bytes - Duration: %v", resp.StatusCode, len(body), requestDuration)
		log.Printf("[SEARCH] [RESPONSE] Body: %s", string(body))

		if resp.StatusCode >= 400 {
			log.Printf("[SEARCH] [ERROR] Search operation failed: HTTP %d, %s", resp.StatusCode, string(body))
			return nil, fmt.Errorf("search operation failed: HTTP %d, %s", resp.StatusCode, string(body))
		}

		// Parse response
		var searchResponse SearchResponse
		if err := json.Unmarshal(body, &searchResponse); err != nil {
			log.Printf("[SEARCH] [ERROR] Failed to parse search response: %v", err)
			return nil, fmt.Errorf("failed to parse search response: %v", err)
		}

		log.Printf("[SEARCH] [SUCCESS] Search completed: %d hits found - Duration: %v", searchResponse.Hits.Total, requestDuration)
		return &searchResponse, nil
	}

	// Execute with circuit breaker and retry logic
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := mc.executeSearchWithRetry(ctx, operation)

	totalDuration := time.Since(startTime)

	// Record metrics
	if mc.metricsCollector != nil {
		mc.metricsCollector.RecordRequest("Search", totalDuration, err == nil, "")
		mc.metricsCollector.RecordSearchOperation()
	}

	if err != nil {
		log.Printf("[SEARCH] [FINAL] Search failed after %v: %v", totalDuration, err)
		if mc.logger != nil {
			mc.logger.LogOperation("Search", totalDuration, false, fmt.Sprintf("Index: %s, Error: %v", request.Index, err))
		}
	} else {
		log.Printf("[SEARCH] [FINAL] Search completed successfully after %v: %d hits", totalDuration, result.Hits.Total)
		if mc.logger != nil {
			mc.logger.LogOperation("Search", totalDuration, true, fmt.Sprintf("Index: %s, Hits: %d", request.Index, result.Hits.Total))
		}
	}

	return result, err
}

// executeSearchWithRetry executes search operation with circuit breaker and retry logic
func (mc *manticoreHTTPClient) executeSearchWithRetry(ctx context.Context, operation func(context.Context) (*SearchResponse, error)) (*SearchResponse, error) {
	var result *SearchResponse

	retryOperation := func(ctx context.Context) error {
		var err error
		result, err = operation(ctx)
		return err
	}

	err := mc.circuitBreakerWithRetry.Execute(ctx, mc.baseURL+"/search", "POST", retryOperation)
	return result, err
}

// GetAllDocuments retrieves all documents using match_all query (used for vector search fallback)
func (mc *manticoreHTTPClient) GetAllDocuments() ([]*models.Document, error) {
	startTime := time.Now()
	log.Printf("[SEARCH] [GETALL] Starting GetAllDocuments operation")

	// Create match_all request with large limit
	request := mc.CreateMatchAllRequest("documents", 10000, 0)

	// Execute search
	response, err := mc.SearchWithRequest(request)
	if err != nil {
		log.Printf("[SEARCH] [GETALL] [ERROR] Failed to execute match_all query: %v", err)
		return nil, fmt.Errorf("failed to get all documents: %v", err)
	}

	// Convert response to documents
	documents, err := mc.convertSearchResponse(response)
	if err != nil {
		log.Printf("[SEARCH] [GETALL] [ERROR] Failed to convert search response: %v", err)
		return nil, fmt.Errorf("failed to convert search response: %v", err)
	}

	totalDuration := time.Since(startTime)
	log.Printf("[SEARCH] [GETALL] [SUCCESS] Retrieved %d documents in %v", len(documents), totalDuration)
	return documents, nil
}

// GetAllDocumentsWithVectors retrieves all documents with their vector data from documents_vector table
func (mc *manticoreHTTPClient) GetAllDocumentsWithVectors() ([]*models.Document, [][]float64, error) {
	startTime := time.Now()
	log.Printf("[SEARCH] [VECTOR] [GETALL] Starting GetAllDocumentsWithVectors operation")

	// Create match_all request for vector table with large limit
	request := mc.CreateMatchAllRequest("documents_vector", 10000, 0)

	// Execute search
	response, err := mc.SearchWithRequest(request)
	if err != nil {
		log.Printf("[SEARCH] [VECTOR] [GETALL] [ERROR] Failed to execute match_all query on vector table: %v", err)
		return nil, nil, fmt.Errorf("failed to get all documents with vectors: %v", err)
	}

	// Convert response to documents and vectors
	documents, vectors, err := mc.convertVectorSearchResponse(response)
	if err != nil {
		log.Printf("[SEARCH] [VECTOR] [GETALL] [ERROR] Failed to convert vector search response: %v", err)
		return nil, nil, fmt.Errorf("failed to convert vector search response: %v", err)
	}

	totalDuration := time.Since(startTime)
	log.Printf("[SEARCH] [VECTOR] [GETALL] [SUCCESS] Retrieved %d documents with vectors in %v", len(documents), totalDuration)
	return documents, vectors, nil
}

// Search request creation methods

// CreateBasicSearchRequest creates a basic search request with match query
func (mc *manticoreHTTPClient) CreateBasicSearchRequest(index, query string, limit, offset int32) SearchRequest {
	log.Printf("[SEARCH] [BASIC] Creating basic search request: query='%s', limit=%d, offset=%d", query, limit, offset)

	searchQuery := map[string]interface{}{
		"match": map[string]interface{}{
			"*": query, // Match against all fields
		},
	}

	return SearchRequest{
		Index:  index,
		Query:  searchQuery,
		Limit:  limit,
		Offset: offset,
	}
}

// CreateFullTextSearchRequest creates a full-text search request with query_string
func (mc *manticoreHTTPClient) CreateFullTextSearchRequest(index, query string, limit, offset int32) SearchRequest {
	log.Printf("[SEARCH] [FULLTEXT] Creating full-text search request: query='%s', limit=%d, offset=%d", query, limit, offset)

	searchQuery := map[string]interface{}{
		"query_string": query,
	}

	return SearchRequest{
		Index:  index,
		Query:  searchQuery,
		Limit:  limit,
		Offset: offset,
	}
}

// CreateMatchQueryRequest creates a match query for specific fields
func (mc *manticoreHTTPClient) CreateMatchQueryRequest(index string, field, query string, limit, offset int32) SearchRequest {
	log.Printf("[SEARCH] [MATCH] Creating match query request: field='%s', query='%s', limit=%d, offset=%d", field, query, limit, offset)

	searchQuery := map[string]interface{}{
		"match": map[string]interface{}{
			field: query,
		},
	}

	return SearchRequest{
		Index:  index,
		Query:  searchQuery,
		Limit:  limit,
		Offset: offset,
	}
}

// CreateMatchAllRequest creates a match_all query to retrieve all documents
func (mc *manticoreHTTPClient) CreateMatchAllRequest(index string, limit, offset int32) SearchRequest {
	log.Printf("[SEARCH] [MATCHALL] Creating match_all request: limit=%d, offset=%d", limit, offset)

	searchQuery := map[string]interface{}{
		"match_all": map[string]interface{}{},
	}

	return SearchRequest{
		Index:  index,
		Query:  searchQuery,
		Limit:  limit,
		Offset: offset,
	}
}

// Response conversion methods

// convertSearchResponse converts Manticore JSON API response to internal models
func (mc *manticoreHTTPClient) convertSearchResponse(response *SearchResponse) ([]*models.Document, error) {
	log.Printf("[SEARCH] [CONVERT] Converting search response: %d hits", response.Hits.Total)

	documents := make([]*models.Document, 0, len(response.Hits.Hits))

	for _, hit := range response.Hits.Hits {
		doc := &models.Document{
			ID: int(hit.ID),
		}

		// Extract fields from source
		if title, ok := hit.Source["title"].(string); ok {
			doc.Title = title
		}
		if content, ok := hit.Source["content"].(string); ok {
			doc.Content = content
		}
		if url, ok := hit.Source["url"].(string); ok {
			doc.URL = url
		}

		documents = append(documents, doc)
	}

	log.Printf("[SEARCH] [CONVERT] Successfully converted %d documents", len(documents))
	return documents, nil
}

// convertSearchResponseWithScores converts Manticore JSON API response to search results with scores
func (mc *manticoreHTTPClient) convertSearchResponseWithScores(response *SearchResponse) ([]models.SearchResult, error) {
	log.Printf("[SEARCH] [CONVERT] Converting search response with scores: %d hits", response.Hits.Total)

	results := make([]models.SearchResult, 0, len(response.Hits.Hits))

	for _, hit := range response.Hits.Hits {
		doc := &models.Document{
			ID: int(hit.ID),
		}

		// Extract fields from source
		if title, ok := hit.Source["title"].(string); ok {
			doc.Title = title
		}
		if content, ok := hit.Source["content"].(string); ok {
			doc.Content = content
		}
		if url, ok := hit.Source["url"].(string); ok {
			doc.URL = url
		}

		result := models.SearchResult{
			Document: doc,
			Score:    float64(hit.Score),
		}

		results = append(results, result)
	}

	log.Printf("[SEARCH] [CONVERT] Successfully converted %d search results", len(results))
	return results, nil
}

// convertVectorSearchResponse converts search response from documents_vector table to documents and vectors
func (mc *manticoreHTTPClient) convertVectorSearchResponse(response *SearchResponse) ([]*models.Document, [][]float64, error) {
	log.Printf("[SEARCH] [VECTOR] [CONVERT] Converting vector search response: %d hits", response.Hits.Total)

	documents := make([]*models.Document, 0, len(response.Hits.Hits))
	vectors := make([][]float64, 0, len(response.Hits.Hits))

	for _, hit := range response.Hits.Hits {
		doc := &models.Document{
			ID: int(hit.ID),
		}

		// Extract fields from source
		if title, ok := hit.Source["title"].(string); ok {
			doc.Title = title
		}
		if url, ok := hit.Source["url"].(string); ok {
			doc.URL = url
		}

		// Parse vector data
		var vector []float64
		if vectorData, ok := hit.Source["vector_data"].(string); ok {
			parsedVector, err := parseVectorFromJSONArray(vectorData)
			if err != nil {
				log.Printf("[SEARCH] [VECTOR] [CONVERT] [WARNING] Failed to parse vector for document %d: %v", doc.ID, err)
				// Use empty vector as fallback
				vector = make([]float64, 0)
			} else {
				vector = parsedVector
			}
		}

		documents = append(documents, doc)
		vectors = append(vectors, vector)
	}

	log.Printf("[SEARCH] [VECTOR] [CONVERT] Successfully converted %d documents with vectors", len(documents))
	return documents, vectors, nil
}

// Vector search utilities

// SearchVectorSimilarity performs vector similarity search using JSON API (if supported)
func (mc *manticoreHTTPClient) SearchVectorSimilarity(queryVector []float64, limit, offset int32) (*SearchResponse, error) {
	startTime := time.Now()
	log.Printf("[SEARCH] [VECTOR] [SIMILARITY] Starting vector similarity search: vector size=%d, limit=%d, offset=%d",
		len(queryVector), limit, offset)

	// Create vector similarity request
	request := mc.CreateVectorSimilarityRequest("documents_vector", "vector_data", queryVector, limit, offset)

	// Execute search
	response, err := mc.SearchWithRequest(request)
	if err != nil {
		log.Printf("[SEARCH] [VECTOR] [SIMILARITY] [WARNING] Vector similarity search failed, this may not be supported by Manticore JSON API: %v", err)
		return nil, fmt.Errorf("vector similarity search failed: %v", err)
	}

	totalDuration := time.Since(startTime)
	log.Printf("[SEARCH] [VECTOR] [SIMILARITY] [SUCCESS] Vector similarity search completed in %v: %d hits",
		totalDuration, response.Hits.Total)

	return response, nil
}

// SearchVectorFallback performs vector search using fallback method (retrieve all and compute similarity)
func (mc *manticoreHTTPClient) SearchVectorFallback(queryVector []float64, limit int) ([]*models.Document, []float64, error) {
	startTime := time.Now()
	log.Printf("[SEARCH] [VECTOR] [FALLBACK] Starting vector fallback search: vector size=%d, limit=%d", len(queryVector), limit)

	// Get all documents with vectors
	documents, vectors, err := mc.GetAllDocumentsWithVectors()
	if err != nil {
		log.Printf("[SEARCH] [VECTOR] [FALLBACK] [ERROR] Failed to get documents with vectors: %v", err)
		return nil, nil, fmt.Errorf("failed to get documents with vectors: %v", err)
	}

	if len(documents) == 0 {
		log.Printf("[SEARCH] [VECTOR] [FALLBACK] [WARNING] No documents found")
		return []*models.Document{}, []float64{}, nil
	}

	log.Printf("[SEARCH] [VECTOR] [FALLBACK] Computing similarity for %d documents", len(documents))

	// Compute similarities
	type docSimilarity struct {
		document   *models.Document
		similarity float64
	}

	similarities := make([]docSimilarity, 0, len(documents))
	for i, doc := range documents {
		if i < len(vectors) {
			similarity := mc.cosineSimilarity(queryVector, vectors[i])
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
	log.Printf("[SEARCH] [VECTOR] [FALLBACK] [SUCCESS] Vector fallback search completed in %v: %d results", totalDuration, len(resultDocs))

	return resultDocs, resultScores, nil
}

// CreateVectorSimilarityRequest creates a vector similarity search request (if supported by Manticore JSON API)
func (mc *manticoreHTTPClient) CreateVectorSimilarityRequest(index string, vectorField string, queryVector []float64, limit, offset int32) SearchRequest {
	log.Printf("[SEARCH] [VECTOR] [SIMILARITY] Creating vector similarity request: field='%s', vector size=%d, limit=%d, offset=%d",
		vectorField, len(queryVector), limit, offset)

	// Note: This is a placeholder implementation
	// Actual vector similarity syntax may vary depending on Manticore version and configuration
	vectorStr := formatVectorAsJSONArray(queryVector)

	searchQuery := map[string]interface{}{
		"knn": map[string]interface{}{
			"field": vectorField,
			"query": vectorStr,
			"k":     limit,
		},
	}

	return SearchRequest{
		Index:  index,
		Query:  searchQuery,
		Limit:  limit,
		Offset: offset,
	}
}

// cosineSimilarity computes cosine similarity between two vectors
func (mc *manticoreHTTPClient) cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		log.Printf("[SEARCH] [VECTOR] [SIMILARITY] [WARNING] Vector length mismatch: %d vs %d", len(a), len(b))
		return 0.0
	}

	if len(a) == 0 {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// parseVectorFromJSONArray parses a vector from JSON array string
func parseVectorFromJSONArray(vectorStr string) ([]float64, error) {
	var vector []float64
	if err := json.Unmarshal([]byte(vectorStr), &vector); err != nil {
		return nil, fmt.Errorf("failed to parse vector JSON: %v", err)
	}
	return vector, nil
}

// NewSearchResultProcessor creates a new search result processor
func (mc *manticoreHTTPClient) NewSearchResultProcessor() *SearchResultProcessor {
	return &SearchResultProcessor{
		client: mc,
	}
}

// ProcessSearchResults processes search results with normalization and ranking
func (srp *SearchResultProcessor) ProcessSearchResults(response *SearchResponse, mode models.SearchMode) (*models.SearchResponse, error) {
	log.Printf("[SEARCH] [PROCESS] Processing search results: mode=%s, hits=%d", mode, response.Hits.Total)

	// Convert to search results with scores
	results, err := srp.client.(*manticoreHTTPClient).convertSearchResponseWithScores(response)
	if err != nil {
		return nil, fmt.Errorf("failed to convert search response: %v", err)
	}

	// Normalize scores
	normalizedResults := srp.normalizeScores(results)

	// Apply ranking based on mode
	rankedResults := srp.rankResults(normalizedResults, mode)

	// Validate results
	validatedResults := srp.validateResults(rankedResults)

	return &models.SearchResponse{
		Documents: validatedResults,
		Total:     int(response.Hits.Total),
		Page:      1, // Default page
		Mode:      string(mode),
	}, nil
}

// normalizeScores normalizes scores to 0-1 range based on max score
func (srp *SearchResultProcessor) normalizeScores(results []models.SearchResult) []models.SearchResult {
	log.Printf("[SEARCH] [NORMALIZE] Normalizing scores for %d results", len(results))

	if len(results) == 0 {
		return results
	}

	// Find max score
	maxScore := 0.0
	for _, result := range results {
		if result.Score > maxScore {
			maxScore = result.Score
		}
	}

	log.Printf("[SEARCH] [NORMALIZE] Max score found: %.4f", maxScore)

	// Normalize if max > 0
	if maxScore > 0 {
		for i := range results {
			oldScore := results[i].Score
			results[i].Score = results[i].Score / maxScore
			log.Printf("[SEARCH] [NORMALIZE] Document ID=%d: %.4f -> %.4f",
				results[i].Document.ID, oldScore, results[i].Score)
		}
	}

	log.Printf("[SEARCH] [NORMALIZE] Score normalization completed")
	return results
}

// rankResults applies additional ranking logic based on search mode
func (srp *SearchResultProcessor) rankResults(results []models.SearchResult, mode models.SearchMode) []models.SearchResult {
	log.Printf("[SEARCH] [RANK] Ranking %d results for mode=%s", len(results), mode)

	switch mode {
	case models.SearchModeBasic:
		return srp.rankBasicResults(results)
	case models.SearchModeFullText:
		return srp.rankFullTextResults(results)
	case models.SearchModeVector:
		return srp.rankVectorResults(results)
	case models.SearchModeHybrid:
		return srp.rankHybridResults(results)
	default:
		return results
	}
}

// rankBasicResults applies basic ranking (primarily by score)
func (srp *SearchResultProcessor) rankBasicResults(results []models.SearchResult) []models.SearchResult {
	log.Printf("[SEARCH] [RANK] [BASIC] Applying basic ranking")

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// rankFullTextResults applies full-text specific ranking
func (srp *SearchResultProcessor) rankFullTextResults(results []models.SearchResult) []models.SearchResult {
	log.Printf("[SEARCH] [RANK] [FULLTEXT] Applying full-text ranking")

	// Sort by score descending with title boost
	sort.Slice(results, func(i, j int) bool {
		scoreI := results[i].Score
		scoreJ := results[j].Score

		// Boost documents with matches in title
		if strings.Contains(strings.ToLower(results[i].Document.Title), "search") {
			scoreI *= 1.2
		}
		if strings.Contains(strings.ToLower(results[j].Document.Title), "search") {
			scoreJ *= 1.2
		}

		return scoreI > scoreJ
	})

	return results
}

// rankVectorResults applies vector-specific ranking
func (srp *SearchResultProcessor) rankVectorResults(results []models.SearchResult) []models.SearchResult {
	log.Printf("[SEARCH] [RANK] [VECTOR] Applying vector ranking")

	// For vector search, scores are already similarity scores, just sort descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// rankHybridResults applies hybrid ranking combining multiple factors
func (srp *SearchResultProcessor) rankHybridResults(results []models.SearchResult) []models.SearchResult {
	log.Printf("[SEARCH] [RANK] [HYBRID] Applying hybrid ranking")

	// Complex ranking that considers multiple factors
	sort.Slice(results, func(i, j int) bool {
		scoreI := results[i].Score
		scoreJ := results[j].Score

		// Factor in document length (shorter documents get slight boost)
		contentLenI := len(results[i].Document.Content)
		contentLenJ := len(results[j].Document.Content)

		if contentLenI > 0 && contentLenI < 1000 {
			scoreI *= 1.1
		}
		if contentLenJ > 0 && contentLenJ < 1000 {
			scoreJ *= 1.1
		}

		// Factor in title matches
		if strings.Contains(strings.ToLower(results[i].Document.Title), "important") {
			scoreI *= 1.15
		}
		if strings.Contains(strings.ToLower(results[j].Document.Title), "important") {
			scoreJ *= 1.15
		}

		return scoreI > scoreJ
	})

	return results
}

// validateResults validates and cleans up search results
func (srp *SearchResultProcessor) validateResults(results []models.SearchResult) []models.SearchResult {
	log.Printf("[SEARCH] [VALIDATE] Validating %d results", len(results))

	validResults := make([]models.SearchResult, 0, len(results))

	for _, result := range results {
		// Skip results with nil documents
		if result.Document == nil {
			log.Printf("[SEARCH] [VALIDATE] [WARNING] Skipping result with nil document")
			continue
		}

		// Skip results with empty titles and content
		if result.Document.Title == "" && result.Document.Content == "" {
			log.Printf("[SEARCH] [VALIDATE] [WARNING] Skipping result with empty title and content")
			continue
		}

		// Ensure score is not negative
		if result.Score < 0 {
			result.Score = 0
		}

		// Ensure score is not greater than 1 (after normalization)
		if result.Score > 1 {
			result.Score = 1
		}

		validResults = append(validResults, result)
	}

	log.Printf("[SEARCH] [VALIDATE] Validation completed: %d valid results", len(validResults))
	return validResults
}

// CalculatePagination calculates pagination information
func (srp *SearchResultProcessor) CalculatePagination(offset, limit, total int) (page int, totalPages int) {
	// Handle zero limit case
	if limit <= 0 {
		page = 1
		totalPages = 1
		log.Printf("[SEARCH] [PAGINATION] Calculated: page=%d, totalPages=%d (offset=%d, limit=%d, total=%d)",
			page, totalPages, offset, limit, total)
		return page, totalPages
	}

	page = (offset / limit) + 1
	if page < 1 {
		page = 1
	}

	totalPages = (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	log.Printf("[SEARCH] [PAGINATION] Calculated: page=%d, totalPages=%d (offset=%d, limit=%d, total=%d)",
		page, totalPages, offset, limit, total)

	return page, totalPages
}
