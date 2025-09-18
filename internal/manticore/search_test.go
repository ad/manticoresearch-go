package manticore

import (
	"testing"

	"github.com/ad/manticoresearch-go/internal/models"
)

func TestCreateBasicSearchRequest(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config)
	httpClient := client.(*manticoreHTTPClient)

	request := httpClient.CreateBasicSearchRequest("documents", "test query", 10, 0)

	if request.Index != "documents" {
		t.Errorf("Expected index 'documents', got '%s'", request.Index)
	}

	if request.Limit != 10 {
		t.Errorf("Expected limit 10, got %d", request.Limit)
	}

	if request.Offset != 0 {
		t.Errorf("Expected offset 0, got %d", request.Offset)
	}

	// Check query structure
	if request.Query == nil {
		t.Error("Expected query to be set")
	}

	if match, exists := request.Query["match"]; exists {
		if matchMap, ok := match.(map[string]interface{}); ok {
			if query, exists := matchMap["*"]; exists {
				if queryStr, ok := query.(string); ok {
					if queryStr != "test query" {
						t.Errorf("Expected query 'test query', got '%s'", queryStr)
					}
				} else {
					t.Error("Expected query to be a string")
				}
			} else {
				t.Error("Expected '*' field in match query")
			}
		} else {
			t.Error("Expected match to be a map")
		}
	} else {
		t.Error("Expected 'match' in query")
	}
}

func TestCreateFullTextSearchRequest(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config)
	httpClient := client.(*manticoreHTTPClient)

	request := httpClient.CreateFullTextSearchRequest("documents", "test query", 20, 10)

	if request.Index != "documents" {
		t.Errorf("Expected index 'documents', got '%s'", request.Index)
	}

	if request.Limit != 20 {
		t.Errorf("Expected limit 20, got %d", request.Limit)
	}

	if request.Offset != 10 {
		t.Errorf("Expected offset 10, got %d", request.Offset)
	}

	// Check query structure
	if queryString, exists := request.Query["query_string"]; exists {
		if queryStr, ok := queryString.(string); ok {
			if queryStr != "test query" {
				t.Errorf("Expected query_string 'test query', got '%s'", queryStr)
			}
		} else {
			t.Error("Expected query_string to be a string")
		}
	} else {
		t.Error("Expected 'query_string' in query")
	}
}

func TestCreateMatchAllRequest(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config)
	httpClient := client.(*manticoreHTTPClient)

	request := httpClient.CreateMatchAllRequest("documents", 100, 0)

	if request.Index != "documents" {
		t.Errorf("Expected index 'documents', got '%s'", request.Index)
	}

	// Check query structure
	if matchAll, exists := request.Query["match_all"]; exists {
		if matchAllMap, ok := matchAll.(map[string]interface{}); ok {
			if len(matchAllMap) != 0 {
				t.Error("Expected match_all to be an empty map")
			}
		} else {
			t.Error("Expected match_all to be a map")
		}
	} else {
		t.Error("Expected 'match_all' in query")
	}
}

func TestCosineSimilarity(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config)
	httpClient := client.(*manticoreHTTPClient)

	// Test identical vectors
	vec1 := []float64{1.0, 2.0, 3.0}
	vec2 := []float64{1.0, 2.0, 3.0}
	similarity := httpClient.cosineSimilarity(vec1, vec2)
	if similarity != 1.0 {
		t.Errorf("Expected similarity 1.0 for identical vectors, got %f", similarity)
	}

	// Test orthogonal vectors
	vec3 := []float64{1.0, 0.0}
	vec4 := []float64{0.0, 1.0}
	similarity = httpClient.cosineSimilarity(vec3, vec4)
	if similarity != 0.0 {
		t.Errorf("Expected similarity 0.0 for orthogonal vectors, got %f", similarity)
	}

	// Test different length vectors
	vec5 := []float64{1.0, 2.0}
	vec6 := []float64{1.0, 2.0, 3.0}
	similarity = httpClient.cosineSimilarity(vec5, vec6)
	if similarity != 0.0 {
		t.Errorf("Expected similarity 0.0 for different length vectors, got %f", similarity)
	}
}

func TestSearchResultProcessor(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config)
	httpClient := client.(*manticoreHTTPClient)

	processor := httpClient.NewSearchResultProcessor()
	if processor == nil {
		t.Error("Expected processor to be created")
	}

	if processor.client != httpClient {
		t.Error("Expected processor to have correct client reference")
	}
}

func TestNormalizeScores(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config)
	httpClient := client.(*manticoreHTTPClient)
	processor := httpClient.NewSearchResultProcessor()

	// Test with empty results
	emptyResults := []models.SearchResult{}
	normalized := processor.normalizeScores(emptyResults)
	if len(normalized) != 0 {
		t.Error("Expected empty results to remain empty")
	}

	// Test with results
	results := []models.SearchResult{
		{Document: &models.Document{ID: 1}, Score: 10.0},
		{Document: &models.Document{ID: 2}, Score: 5.0},
		{Document: &models.Document{ID: 3}, Score: 2.0},
	}

	normalized = processor.normalizeScores(results)
	if len(normalized) != 3 {
		t.Errorf("Expected 3 results, got %d", len(normalized))
	}

	// Check that max score is now 1.0
	maxScore := 0.0
	for _, result := range normalized {
		if result.Score > maxScore {
			maxScore = result.Score
		}
	}

	if maxScore != 1.0 {
		t.Errorf("Expected max score to be 1.0 after normalization, got %f", maxScore)
	}

	// Check that relative ordering is preserved
	if normalized[0].Score <= normalized[1].Score || normalized[1].Score <= normalized[2].Score {
		t.Error("Expected relative ordering to be preserved after normalization")
	}
}

func TestCalculatePagination(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config)
	httpClient := client.(*manticoreHTTPClient)
	processor := httpClient.NewSearchResultProcessor()

	// Test first page
	page, totalPages := processor.CalculatePagination(0, 10, 100)
	if page != 1 {
		t.Errorf("Expected page 1, got %d", page)
	}
	if totalPages != 10 {
		t.Errorf("Expected 10 total pages, got %d", totalPages)
	}

	// Test second page
	page, totalPages = processor.CalculatePagination(10, 10, 100)
	if page != 2 {
		t.Errorf("Expected page 2, got %d", page)
	}

	// Test with non-even division
	page, totalPages = processor.CalculatePagination(0, 10, 95)
	if totalPages != 10 {
		t.Errorf("Expected 10 total pages for 95 items with limit 10, got %d", totalPages)
	}

	// Test with zero limit
	page, totalPages = processor.CalculatePagination(0, 0, 100)
	if page != 1 || totalPages != 1 {
		t.Errorf("Expected page=1, totalPages=1 for zero limit, got page=%d, totalPages=%d", page, totalPages)
	}
}
