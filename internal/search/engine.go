package search

import (
	"fmt"
	"log"
	"sort"

	"github.com/ad/manticoresearch-go/internal/manticore"
	"github.com/ad/manticoresearch-go/internal/models"
	"github.com/ad/manticoresearch-go/internal/vectorizer"
	openapi "github.com/manticoresoftware/manticoresearch-go"
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
	default:
		return "", fmt.Errorf("invalid search mode: %s. Valid modes are: basic, fulltext, vector, hybrid", modeStr)
	}
}

// SearchEngine handles all search operations using the official Manticore client
type SearchEngine struct {
	client     *manticore.ManticoreClient
	vectorizer *vectorizer.TFIDFVectorizer
}

// NewSearchEngine creates a new search engine with the official Manticore client
func NewSearchEngine(client *manticore.ManticoreClient, vectorizer *vectorizer.TFIDFVectorizer) *SearchEngine {
	return &SearchEngine{
		client:     client,
		vectorizer: vectorizer,
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
	default:
		return nil, fmt.Errorf("unknown search mode: %s", mode)
	}
}

// BasicSearch performs simple text matching
func (e *SearchEngine) BasicSearch(query string, page, pageSize int) (*models.SearchResponse, error) {
	log.Printf("BasicSearch: query='%s', page=%d, pageSize=%d", query, page, pageSize)

	searchReq := openapi.NewSearchRequest("documents")

	// Create a basic match query
	searchQuery := openapi.NewSearchQuery()
	matchQuery := map[string]interface{}{
		"*": query,
	}
	searchQuery.SetMatch(matchQuery)
	searchReq.SetQuery(*searchQuery)

	// Set pagination
	offset := int32((page - 1) * pageSize)
	limit := int32(pageSize)
	searchReq.SetOffset(offset)
	searchReq.SetLimit(limit)

	log.Printf("BasicSearch: executing search with offset=%d, limit=%d", offset, limit)

	// Execute search
	resp, _, err := e.client.GetClient().SearchAPI.Search(e.client.GetContext()).SearchRequest(*searchReq).Execute()
	if err != nil {
		log.Printf("BasicSearch: search failed: %v", err)
		return nil, fmt.Errorf("basic search failed: %v", err)
	}

	log.Printf("BasicSearch: got response with %d hits", len(resp.Hits.Hits))
	result, errConvertSearchResponse := e.convertSearchResponse(*resp, models.SearchModeBasic, page)
	log.Printf("BasicSearch: returning %d results", len(result.Documents))

	return result, errConvertSearchResponse
}

// FullTextSearch performs full-text search with Manticore's query language
func (e *SearchEngine) FullTextSearch(query string, page, pageSize int) (*models.SearchResponse, error) {
	searchReq := openapi.NewSearchRequest("documents")

	// Create a query string search
	searchQuery := openapi.NewSearchQuery()
	searchQuery.SetQueryString(query)
	searchReq.SetQuery(*searchQuery)

	// Set pagination
	offset := int32((page - 1) * pageSize)
	limit := int32(pageSize)
	searchReq.SetOffset(offset)
	searchReq.SetLimit(limit)

	// Execute search
	resp, _, err := e.client.GetClient().SearchAPI.Search(e.client.GetContext()).SearchRequest(*searchReq).Execute()
	if err != nil {
		return nil, fmt.Errorf("full-text search failed: %v", err)
	}

	return e.convertSearchResponse(*resp, models.SearchModeFullText, page)
}

// VectorSearch performs vector similarity search
func (e *SearchEngine) VectorSearch(query string, page, pageSize int) (*models.SearchResponse, error) {
	// Get all documents for vector computation
	documents, err := e.getAllDocuments()
	if err != nil {
		return nil, fmt.Errorf("failed to get documents for vector search: %v", err)
	}

	if len(documents) == 0 {
		return &models.SearchResponse{
			Documents: []models.SearchResult{},
			Total:     0,
			Page:      page,
			Mode:      string(models.SearchModeVector),
		}, nil
	}

	// First we need to get vectors for all documents
	// This is a simplified approach - in production we'd cache vectors
	allVectors := e.vectorizer.FitTransform(documents)

	// Perform TF-IDF vector search using the global function
	results := vectorizer.VectorSearch(query, documents, allVectors, e.vectorizer, pageSize)

	// Convert to search results
	searchResults := make([]models.SearchResult, 0, len(results))
	for _, result := range results {
		searchResults = append(searchResults, models.SearchResult{
			Document: result.Document,
			Score:    result.Similarity,
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
		Total:     len(results),
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

// getAllDocuments retrieves all documents using official client
func (e *SearchEngine) getAllDocuments() ([]*models.Document, error) {
	// Use search API to get all documents from the documents table
	searchReq := openapi.NewSearchRequest("documents")

	// Create a match_all query to get all documents
	searchQuery := openapi.NewSearchQuery()
	matchAll := map[string]interface{}{}
	searchQuery.SetMatchAll(matchAll)
	searchReq.SetQuery(*searchQuery)

	// Set large limit to get all documents (adjust if needed)
	searchReq.SetLimit(10000)

	// Execute search
	resp, _, err := e.client.GetClient().SearchAPI.Search(e.client.GetContext()).SearchRequest(*searchReq).Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query all documents: %v", err)
	}

	documents := make([]*models.Document, 0)

	if resp.Hits != nil && resp.Hits.Hits != nil {
		for _, hit := range resp.Hits.Hits {
			doc := &models.Document{}

			// Parse document fields from hit
			// Parse ID from hit.Id (not from Source)
			if hit.Id != nil {
				doc.ID = int(*hit.Id)
			}

			if hit.Source != nil {
				sourceMap := hit.Source

				// Parse Title
				if title, exists := sourceMap["title"]; exists {
					if titleStr, ok := title.(string); ok {
						doc.Title = titleStr
					}
				}

				// Parse Content
				if content, exists := sourceMap["content"]; exists {
					if contentStr, ok := content.(string); ok {
						doc.Content = contentStr
					}
				}

				// Parse URL
				if url, exists := sourceMap["url"]; exists {
					if urlStr, ok := url.(string); ok {
						doc.URL = urlStr
					}
				}
			}

			documents = append(documents, doc)
		}
	}

	return documents, nil
} // convertSearchResponse converts official API response to our format
func (e *SearchEngine) convertSearchResponse(resp openapi.SearchResponse, mode models.SearchMode, page int) (*models.SearchResponse, error) {
	results := make([]models.SearchResult, 0)

	if resp.Hits != nil && resp.Hits.Hits != nil {
		for _, hit := range resp.Hits.Hits {
			doc := &models.Document{}

			// Parse document fields from hit
			// Parse ID from hit.Id (not from Source)
			if hit.Id != nil {
				doc.ID = int(*hit.Id)
			}

			if hit.Source != nil {
				sourceMap := hit.Source

				// Parse Title
				if title, exists := sourceMap["title"]; exists {
					if titleStr, ok := title.(string); ok {
						doc.Title = titleStr
					}
				}

				// Parse Content
				if content, exists := sourceMap["content"]; exists {
					if contentStr, ok := content.(string); ok {
						doc.Content = contentStr
					}
				}

				// Parse URL
				if url, exists := sourceMap["url"]; exists {
					if urlStr, ok := url.(string); ok {
						doc.URL = urlStr
					}
				}
			}

			// Get score
			score := 0.0
			if hit.Score != nil {
				score = float64(*hit.Score)
			}

			results = append(results, models.SearchResult{
				Document: doc,
				Score:    score,
			})
		}
	}

	total := 0
	if resp.Hits != nil && resp.Hits.Total != nil {
		total = int(*resp.Hits.Total)
	}

	return &models.SearchResponse{
		Documents: results,
		Total:     total,
		Page:      page,
		Mode:      string(mode),
	}, nil
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
