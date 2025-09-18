package search

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/ad/manticoresearch-go/internal/manticore"
	"github.com/ad/manticoresearch-go/internal/models"
	"github.com/ad/manticoresearch-go/internal/vectorizer"
)

// ValidateSearchMode validates and returns the search mode
func ValidateSearchMode(modeStr string) (models.SearchMode, error) {
	switch modeStr {
	case "basic":
		return models.SearchModeBasic, nil
	case "fulltext":
		return models.SearchModeFullText, nil
	case "vector":
		return models.SearchModeVector, nil
	case "hybrid":
		return models.SearchModeHybrid, nil
	case "ai":
		return models.SearchModeAI, nil
	default:
		return "", fmt.Errorf("invalid search mode: %s. Valid modes are: basic, fulltext, vector, hybrid, ai", modeStr)
	}
}

// SearchEngine handles all search operations using the Manticore client interface
type SearchEngine struct {
	client        manticore.ClientInterface
	searchAdapter *manticore.SearchAdapter
	vectorizer    *vectorizer.TFIDFVectorizer
	aiConfig      *models.AISearchConfig
}

// NewSearchEngine creates a new search engine with the Manticore client interface
func NewSearchEngine(client manticore.ClientInterface, vectorizer *vectorizer.TFIDFVectorizer, aiConfig *models.AISearchConfig) *SearchEngine {
	return &SearchEngine{
		client:        client,
		searchAdapter: manticore.NewSearchAdapter(client),
		vectorizer:    vectorizer,
		aiConfig:      aiConfig,
	}
}

// Search performs search across different modes using official client
func (e *SearchEngine) Search(query string, mode models.SearchMode, page, pageSize int) (*models.SearchResponse, error) {
	switch mode {
	case models.SearchModeBasic:
		return e.BasicSearch(query, page, pageSize)
	case models.SearchModeFullText:
		return e.FullTextSearch(query, page, pageSize)
	case models.SearchModeVector:
		return e.VectorSearch(query, page, pageSize)
	case models.SearchModeHybrid:
		return e.HybridSearch(query, page, pageSize)
	case models.SearchModeAI:
		return e.AISearch(query, page, pageSize)
	default:
		return nil, fmt.Errorf("unknown search mode: %s", mode)
	}
}

// BasicSearch performs simple text matching
func (e *SearchEngine) BasicSearch(query string, page, pageSize int) (*models.SearchResponse, error) {
	return e.searchAdapter.BasicSearch(query, page, pageSize)
}

// FullTextSearch performs full-text search with Manticore's query language
func (e *SearchEngine) FullTextSearch(query string, page, pageSize int) (*models.SearchResponse, error) {
	return e.searchAdapter.FullTextSearch(query, page, pageSize)
}

// VectorSearch performs vector similarity search
func (e *SearchEngine) VectorSearch(query string, page, pageSize int) (*models.SearchResponse, error) {
	// Get all documents with pre-computed vectors from documents_vector table
	documents, vectors, err := e.searchAdapter.GetAllDocumentsWithVectors()
	if err != nil {
		return nil, fmt.Errorf("failed to get documents with vectors: %v", err)
	}

	if len(documents) == 0 {
		return &models.SearchResponse{
			Documents: []models.SearchResult{},
			Total:     0,
			Page:      page,
			Mode:      string(models.SearchModeVector),
		}, nil
	}

	// Vectorize query using same TF-IDF approach
	queryVec := e.vectorizer.TransformQuery(query)
	if len(queryVec) == 0 {
		return &models.SearchResponse{
			Documents: []models.SearchResult{},
			Total:     0,
			Page:      page,
			Mode:      string(models.SearchModeVector),
		}, nil
	}

	// Calculate cosine similarity with pre-computed vectors
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

	// Convert to search results
	searchResults := make([]models.SearchResult, 0, len(similarities))
	for _, sim := range similarities {
		searchResults = append(searchResults, models.SearchResult{
			Document: sim.document,
			Score:    sim.similarity,
		})
	}

	// Apply pagination
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(searchResults) {
		searchResults = []models.SearchResult{}
	} else if end > len(searchResults) {
		searchResults = searchResults[start:]
	} else {
		searchResults = searchResults[start:end]
	}

	return &models.SearchResponse{
		Documents: searchResults,
		Total:     len(similarities),
		Page:      page,
		Mode:      string(models.SearchModeVector),
	}, nil
}

// HybridSearch combines full-text and vector search results
func (e *SearchEngine) HybridSearch(query string, page, pageSize int) (*models.SearchResponse, error) {
	log.Printf("HybridSearch: Starting hybrid search for query='%s', page=%d, pageSize=%d", query, page, pageSize)

	// Get full-text search results
	ftResults, err := e.FullTextSearch(query, 1, pageSize*2) // Get more results for merging
	if err != nil {
		log.Printf("HybridSearch: Full-text search failed: %v", err)
		ftResults = &models.SearchResponse{Documents: []models.SearchResult{}}
	} else {
		log.Printf("HybridSearch: Full-text search returned %d results", len(ftResults.Documents))
		if len(ftResults.Documents) > 0 {
			log.Printf("HybridSearch: FT top result: '%s' (score: %.2f)",
				ftResults.Documents[0].Document.Title, ftResults.Documents[0].Score)
		}
	}

	// Get vector search results
	vectorResults, err := e.VectorSearch(query, 1, pageSize*2) // Get more results for merging
	if err != nil {
		log.Printf("HybridSearch: Vector search failed: %v", err)
		vectorResults = &models.SearchResponse{Documents: []models.SearchResult{}}
	} else {
		log.Printf("HybridSearch: Vector search returned %d results", len(vectorResults.Documents))
		if len(vectorResults.Documents) > 0 {
			log.Printf("HybridSearch: Vector top result: '%s' (score: %.4f)",
				vectorResults.Documents[0].Document.Title, vectorResults.Documents[0].Score)
		}
	}

	// Combine and deduplicate results
	combined := e.combineResults(ftResults.Documents, vectorResults.Documents)

	// Apply pagination
	start := (page - 1) * pageSize
	end := start + pageSize
	totalResults := len(combined)

	if start > len(combined) {
		combined = []models.SearchResult{}
	} else if end > len(combined) {
		combined = combined[start:]
	} else {
		combined = combined[start:end]
	}

	log.Printf("HybridSearch: Returning %d results (total: %d) after pagination", len(combined), totalResults)
	if len(combined) > 0 {
		log.Printf("HybridSearch: Final top result: '%s' (combined score: %.4f)",
			combined[0].Document.Title, combined[0].Score)
	}

	return &models.SearchResponse{
		Documents: combined,
		Total:     totalResults,
		Page:      page,
		Mode:      string(models.SearchModeHybrid),
	}, nil
}

// getAllDocuments retrieves all documents using client interface
func (e *SearchEngine) getAllDocuments() ([]*models.Document, error) {
	return e.searchAdapter.GetAllDocuments()
}

// normalizeScores normalizes scores to 0-1 range based on max score
func normalizeScores(results []models.SearchResult) []models.SearchResult {
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

	// Normalize if max > 0
	if maxScore > 0 {
		for i := range results {
			results[i].Score = results[i].Score / maxScore
		}
	}

	return results
}

// combineResults merges and deduplicates search results from different sources with proper normalization
func (e *SearchEngine) combineResults(ftResults, vectorResults []models.SearchResult) []models.SearchResult {
	log.Printf("HybridSearch: Combining %d FullText results with %d Vector results", len(ftResults), len(vectorResults))

	// Debug: Log first few FT results
	for i, result := range ftResults {
		if i < 3 && result.Document != nil {
			log.Printf("HybridSearch: FT[%d]: ID=%d, Title='%s', Score=%.2f",
				i, result.Document.ID, result.Document.Title, result.Score)
		}
	}

	// Debug: Log first few Vector results
	for i, result := range vectorResults {
		if i < 3 && result.Document != nil {
			log.Printf("HybridSearch: Vector[%d]: ID=%d, Title='%s', Score=%.4f",
				i, result.Document.ID, result.Document.Title, result.Score)
		}
	}

	// Normalize scores to 0-1 range for both result sets
	normalizedFTResults := normalizeScores(append([]models.SearchResult(nil), ftResults...))         // Copy slice
	normalizedVectorResults := normalizeScores(append([]models.SearchResult(nil), vectorResults...)) // Copy slice

	log.Printf("HybridSearch: After normalization - FT max score: %.4f, Vector max score: %.4f",
		getMaxScore(normalizedFTResults), getMaxScore(normalizedVectorResults))

	// Create a map to track documents by ID and merge scores
	docMap := make(map[int]*models.SearchResult)

	// Weights for combining
	ftWeight := 0.6     // 60% for full-text
	vectorWeight := 0.4 // 40% for vector

	// Add full-text results with weight
	for _, result := range normalizedFTResults {
		if result.Document != nil {
			docMap[result.Document.ID] = &models.SearchResult{
				Document: result.Document,
				Score:    result.Score * ftWeight,
			}
		}
	}

	log.Printf("HybridSearch: After adding FT results, docMap has %d entries", len(docMap))

	// Add vector results with weight, merging with existing
	for _, result := range normalizedVectorResults {
		if result.Document != nil {
			if existing, exists := docMap[result.Document.ID]; exists {
				// Combine normalized scores
				existing.Score += result.Score * vectorWeight
				log.Printf("HybridSearch: Combined ID=%d: FT=%.4f + Vector=%.4f = %.4f",
					result.Document.ID, existing.Score-result.Score*vectorWeight,
					result.Score*vectorWeight, existing.Score)
			} else {
				// Document only in vector results
				docMap[result.Document.ID] = &models.SearchResult{
					Document: result.Document,
					Score:    result.Score * vectorWeight,
				}
				log.Printf("HybridSearch: Added Vector-only ID=%d, Score=%.4f",
					result.Document.ID, result.Score*vectorWeight)
			}
		}
	}

	log.Printf("HybridSearch: After adding Vector results, docMap has %d entries", len(docMap))

	// Convert map back to slice
	combined := make([]models.SearchResult, 0, len(docMap))
	for _, result := range docMap {
		combined = append(combined, *result)
	}

	// Sort by combined score (descending)
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].Score > combined[j].Score
	})

	log.Printf("HybridSearch: Combined to %d unique results, top score: %.4f",
		len(combined), getMaxScore(combined))

	// Log top 3 combined results
	for i, result := range combined {
		if i < 3 && result.Document != nil {
			log.Printf("HybridSearch: Combined[%d]: ID=%d, Title='%s', Score=%.4f",
				i, result.Document.ID, result.Document.Title, result.Score)
		}
	}

	return combined
}

// getMaxScore helper function to get max score from results
func getMaxScore(results []models.SearchResult) float64 {
	maxScore := 0.0
	for _, result := range results {
		if result.Score > maxScore {
			maxScore = result.Score
		}
	}
	return maxScore
}

// AISearch performs AI-powered semantic search using Manticore's AI search functionality
func (e *SearchEngine) AISearch(query string, page, pageSize int) (*models.SearchResponse, error) {
	startTime := time.Now()
	log.Printf("AISearch: Starting AI search for query='%s', page=%d, pageSize=%d", query, page, pageSize)

	// Check if AI search is enabled
	if e.aiConfig == nil || !e.aiConfig.Enabled {
		log.Printf("AISearch: AI search is disabled in configuration")
		return nil, fmt.Errorf("AI search is disabled in configuration")
	}

	// Validate query
	if query == "" {
		log.Printf("AISearch: Empty query provided, returning empty results")
		return &models.SearchResponse{
			Documents: []models.SearchResult{},
			Total:     0,
			Page:      page,
			Mode:      string(models.SearchModeAI),
		}, nil
	}

	// Check client availability
	if e.client == nil {
		log.Printf("AISearch: Manticore client is not available")
		return nil, fmt.Errorf("Manticore client is not available for AI search")
	}

	// Calculate offset for pagination
	offset := (page - 1) * pageSize

	// Use the configured AI model
	model := e.aiConfig.Model
	if model == "" {
		model = "sentence-transformers/all-MiniLM-L6-v2" // Default fallback
		log.Printf("AISearch: Using default AI model: %s", model)
	} else {
		log.Printf("AISearch: Using configured AI model: %s", model)
	}

	// Log AI search configuration for monitoring
	log.Printf("AISearch: Configuration - Model: %s, Enabled: %t, Timeout: %v",
		model, e.aiConfig.Enabled, e.aiConfig.Timeout)

	// Perform AI search using the client
	response, err := e.client.AISearch(query, model, pageSize, offset)
	searchDuration := time.Since(startTime)

	if err != nil {
		log.Printf("AISearch: AI search request failed after %v: %v", searchDuration, err)
		// Log detailed error information for monitoring
		log.Printf("AISearch: Error details - Query: '%s', Model: '%s', Page: %d, PageSize: %d",
			query, model, page, pageSize)
		return nil, fmt.Errorf("AI search request failed: %w", err)
	}

	// Process AI search results
	searchResults, err := e.processAISearchResults(response)
	if err != nil {
		log.Printf("AISearch: Failed to process AI search results after %v: %v", searchDuration, err)
		return nil, fmt.Errorf("failed to process AI search results: %w", err)
	}

	totalDuration := time.Since(startTime)
	resultCount := len(searchResults)

	log.Printf("AISearch: Successfully completed AI search in %v - Query: '%s', Model: '%s', Results: %d/%d",
		totalDuration, query, model, resultCount, int(response.Hits.Total))

	// Log performance metrics for monitoring
	log.Printf("AISearch: Performance - Search Duration: %v, Processing Duration: %v, Total Duration: %v",
		searchDuration, totalDuration-searchDuration, totalDuration)

	return &models.SearchResponse{
		Documents: searchResults,
		Total:     int(response.Hits.Total),
		Page:      page,
		Mode:      string(models.SearchModeAI),
	}, nil
}

// processAISearchResults converts Manticore AI search response to SearchResult format
func (e *SearchEngine) processAISearchResults(response *manticore.SearchResponse) ([]models.SearchResult, error) {
	if response == nil || len(response.Hits.Hits) == 0 {
		return []models.SearchResult{}, nil
	}

	results := make([]models.SearchResult, 0, len(response.Hits.Hits))

	for _, hit := range response.Hits.Hits {
		// Extract document information from the hit source
		doc, err := e.extractDocumentFromHit(hit)
		if err != nil {
			log.Printf("AISearch: Failed to extract document from hit: %v", err)
			continue
		}

		// Create search result with AI similarity score
		result := models.SearchResult{
			Document: doc,
			Score:    float64(hit.Score),
		}

		results = append(results, result)
	}

	log.Printf("AISearch: Processed %d AI search results with scores", len(results))
	return results, nil
}

// extractDocumentFromHit extracts document information from a Manticore search hit
func (e *SearchEngine) extractDocumentFromHit(hit struct {
	Index  string                 `json:"_index"`
	ID     int64                  `json:"_id"`
	Score  float32                `json:"_score"`
	Source map[string]interface{} `json:"_source"`
}) (*models.Document, error) {
	// Extract document fields from source
	title, _ := hit.Source["title"].(string)
	content, _ := hit.Source["content"].(string)
	url, _ := hit.Source["url"].(string)

	// Create document
	doc := &models.Document{
		ID:      int(hit.ID),
		Title:   title,
		Content: content,
		URL:     url,
	}

	return doc, nil
}
