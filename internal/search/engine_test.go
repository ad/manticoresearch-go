package search

import (
	"testing"
	"time"

	"github.com/ad/manticoresearch-go/internal/manticore"
	"github.com/ad/manticoresearch-go/internal/models"
)

// MockClient implements ClientInterface for testing
type MockClient struct {
	aiSearchResponse *manticore.SearchResponse
	aiSearchError    error
}

func (m *MockClient) WaitForReady(timeout time.Duration) error           { return nil }
func (m *MockClient) HealthCheck() error                                 { return nil }
func (m *MockClient) Close() error                                       { return nil }
func (m *MockClient) IsConnected() bool                                  { return true }
func (m *MockClient) CreateSchema(aiConfig *models.AISearchConfig) error { return nil }
func (m *MockClient) ResetDatabase() error                               { return nil }
func (m *MockClient) TruncateTables() error                              { return nil }
func (m *MockClient) IndexDocument(doc *models.Document, vector []float64) error {
	return nil
}
func (m *MockClient) IndexDocuments(documents []*models.Document, vectors [][]float64) error {
	return nil
}
func (m *MockClient) Search(query string, mode models.SearchMode, page, pageSize int) (*models.SearchResponse, error) {
	return nil, nil
}
func (m *MockClient) GetAllDocuments() ([]*models.Document, error) { return nil, nil }
func (m *MockClient) SearchWithRequest(request manticore.SearchRequest) (*manticore.SearchResponse, error) {
	return nil, nil
}

func (m *MockClient) AISearch(query string, model string, limit, offset int) (*manticore.SearchResponse, error) {
	return m.aiSearchResponse, m.aiSearchError
}

func (m *MockClient) GenerateEmbedding(text string, model string) ([]float64, error) {
	return []float64{0.1, 0.2, 0.3}, nil
}

func TestAISearch_Success(t *testing.T) {
	// Create mock response
	mockResponse := &manticore.SearchResponse{
		Took:     5,
		TimedOut: false,
		Hits: struct {
			Total         int32  `json:"total"`
			TotalRelation string `json:"total_relation"`
			Hits          []struct {
				Index  string                 `json:"_index"`
				ID     int64                  `json:"_id"`
				Score  float32                `json:"_score"`
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		}{
			Total:         2,
			TotalRelation: "eq",
			Hits: []struct {
				Index  string                 `json:"_index"`
				ID     int64                  `json:"_id"`
				Score  float32                `json:"_score"`
				Source map[string]interface{} `json:"_source"`
			}{
				{
					Index: "documents",
					ID:    1,
					Score: 0.95,
					Source: map[string]interface{}{
						"title":   "Test Document 1",
						"content": "This is test content",
						"url":     "http://example.com/1",
					},
				},
				{
					Index: "documents",
					ID:    2,
					Score: 0.85,
					Source: map[string]interface{}{
						"title":   "Test Document 2",
						"content": "Another test document",
						"url":     "http://example.com/2",
					},
				},
			},
		},
	}

	// Create mock client
	mockClient := &MockClient{
		aiSearchResponse: mockResponse,
		aiSearchError:    nil,
	}

	// Create AI config
	aiConfig := &models.AISearchConfig{
		Model:   "sentence-transformers/all-MiniLM-L6-v2",
		Enabled: true,
	}

	// Create search engine
	engine := NewSearchEngine(mockClient, nil, aiConfig)

	// Perform AI search
	result, err := engine.AISearch("test query", 1, 10)

	// Verify results
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if result.Mode != string(models.SearchModeAI) {
		t.Errorf("Expected mode %s, got %s", models.SearchModeAI, result.Mode)
	}

	if result.Total != 2 {
		t.Errorf("Expected total 2, got %d", result.Total)
	}

	if len(result.Documents) != 2 {
		t.Errorf("Expected 2 documents, got %d", len(result.Documents))
	}

	// Verify first document
	if result.Documents[0].Document.Title != "Test Document 1" {
		t.Errorf("Expected title 'Test Document 1', got '%s'", result.Documents[0].Document.Title)
	}

	expectedScore := 0.95
	if result.Documents[0].Score < expectedScore-0.001 || result.Documents[0].Score > expectedScore+0.001 {
		t.Errorf("Expected score approximately %f, got %f", expectedScore, result.Documents[0].Score)
	}
}

func TestAISearch_Disabled(t *testing.T) {
	// Create mock client
	mockClient := &MockClient{}

	// Create AI config with disabled AI search
	aiConfig := &models.AISearchConfig{
		Model:   "sentence-transformers/all-MiniLM-L6-v2",
		Enabled: false,
	}

	// Create search engine
	engine := NewSearchEngine(mockClient, nil, aiConfig)

	// Perform AI search - should return error when disabled
	result, err := engine.AISearch("test query", 1, 10)

	// AI search should return an error when disabled
	if err == nil {
		t.Fatalf("Expected error when AI search is disabled, got none")
	}

	if result != nil {
		t.Errorf("Expected nil result when AI search is disabled, got: %v", result)
	}

	// Check that the error message indicates AI search is disabled
	if !containsString(err.Error(), "disabled") {
		t.Errorf("Expected error message to contain 'disabled', got: %v", err)
	}
}

func TestAISearch_EmptyQuery(t *testing.T) {
	// Create mock client
	mockClient := &MockClient{}

	// Create AI config
	aiConfig := &models.AISearchConfig{
		Model:   "sentence-transformers/all-MiniLM-L6-v2",
		Enabled: true,
	}

	// Create search engine
	engine := NewSearchEngine(mockClient, nil, aiConfig)

	// Perform AI search with empty query
	result, err := engine.AISearch("", 1, 10)

	// Verify results
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if result.Mode != string(models.SearchModeAI) {
		t.Errorf("Expected mode %s, got %s", models.SearchModeAI, result.Mode)
	}

	if result.Total != 0 {
		t.Errorf("Expected total 0, got %d", result.Total)
	}

	if len(result.Documents) != 0 {
		t.Errorf("Expected 0 documents, got %d", len(result.Documents))
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
