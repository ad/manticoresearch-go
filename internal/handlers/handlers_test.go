package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ad/manticoresearch-go/internal/manticore"
	"github.com/ad/manticoresearch-go/internal/models"
)

// MockManticoreClient for testing
type MockManticoreClient struct {
	connected bool
	healthy   bool
}

func (m *MockManticoreClient) IsConnected() bool {
	return m.connected
}

func (m *MockManticoreClient) HealthCheck() error {
	if !m.healthy {
		return fmt.Errorf("health check failed")
	}
	return nil
}

func (m *MockManticoreClient) WaitForReady(timeout time.Duration) error {
	return nil
}

func (m *MockManticoreClient) Close() error {
	return nil
}

func (m *MockManticoreClient) CreateSchema(aiConfig *models.AISearchConfig) error {
	return nil
}

func (m *MockManticoreClient) ResetDatabase() error {
	return nil
}

func (m *MockManticoreClient) TruncateTables() error {
	return nil
}

func (m *MockManticoreClient) IndexDocument(doc *models.Document, vector []float64) error {
	return nil
}

func (m *MockManticoreClient) IndexDocuments(docs []*models.Document, vectors [][]float64) error {
	return nil
}

func (m *MockManticoreClient) Search(query string, mode models.SearchMode, page, pageSize int) (*models.SearchResponse, error) {
	return &models.SearchResponse{
		Documents: []models.SearchResult{},
		Total:     0,
		Page:      page,
		Mode:      string(mode),
	}, nil
}

func (m *MockManticoreClient) GetAllDocuments() ([]*models.Document, error) {
	return []*models.Document{}, nil
}

func (m *MockManticoreClient) SearchWithRequest(request manticore.SearchRequest) (*manticore.SearchResponse, error) {
	return &manticore.SearchResponse{}, nil
}

func (m *MockManticoreClient) AISearch(query, model string, limit, offset int) (*manticore.SearchResponse, error) {
	return &manticore.SearchResponse{
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
			Total: 0,
			Hits: []struct {
				Index  string                 `json:"_index"`
				ID     int64                  `json:"_id"`
				Score  float32                `json:"_score"`
				Source map[string]interface{} `json:"_source"`
			}{},
		},
	}, nil
}

func (m *MockManticoreClient) GenerateEmbedding(text string, model string) ([]float64, error) {
	return []float64{0.1, 0.2, 0.3}, nil
}

func TestSearchHandler_AISearchValidation(t *testing.T) {
	// Test AI search validation when AI is disabled
	app := &AppState{
		AIConfig: &models.AISearchConfig{
			Model:   "test-model",
			Enabled: false,
			Timeout: 30,
		},
		Manticore: &MockManticoreClient{connected: true, healthy: true},
	}

	req := httptest.NewRequest("GET", "/api/search?query=test&mode=ai", nil)
	w := httptest.NewRecorder()

	app.SearchHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestSearchHandler_AISearchSuccess(t *testing.T) {
	// Test successful AI search
	app := &AppState{
		AIConfig: &models.AISearchConfig{
			Model:   "test-model",
			Enabled: true,
			Timeout: 30,
		},
		Manticore: &MockManticoreClient{connected: true, healthy: true},
	}

	req := httptest.NewRequest("GET", "/api/search?query=test&mode=ai", nil)
	w := httptest.NewRecorder()

	app.SearchHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestStatusHandler_AISearchInfo(t *testing.T) {
	// Test status handler includes AI search information
	app := &AppState{
		AIConfig: &models.AISearchConfig{
			Model:   "test-model",
			Enabled: true,
			Timeout: 30,
		},
		Manticore: &MockManticoreClient{connected: true, healthy: true},
	}

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()

	app.StatusHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check that response contains AI search information
	body := w.Body.String()
	if body == "" {
		t.Error("Expected non-empty response body")
	}
}

func TestValidateAISearchAvailability(t *testing.T) {
	tests := []struct {
		name      string
		app       *AppState
		expectErr bool
	}{
		{
			name: "AI search available",
			app: &AppState{
				AIConfig:  &models.AISearchConfig{Enabled: true},
				Manticore: &MockManticoreClient{connected: true},
			},
			expectErr: false,
		},
		{
			name: "AI search disabled",
			app: &AppState{
				AIConfig:  &models.AISearchConfig{Enabled: false},
				Manticore: &MockManticoreClient{connected: true},
			},
			expectErr: true,
		},
		{
			name: "No AI config",
			app: &AppState{
				AIConfig:  nil,
				Manticore: &MockManticoreClient{connected: true},
			},
			expectErr: true,
		},
		{
			name: "Manticore not connected",
			app: &AppState{
				AIConfig:  &models.AISearchConfig{Enabled: true},
				Manticore: &MockManticoreClient{connected: false},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.app.validateAISearchAvailability()
			if (err != nil) != tt.expectErr {
				t.Errorf("validateAISearchAvailability() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestCheckAISearchHealth(t *testing.T) {
	tests := []struct {
		name     string
		app      *AppState
		expected bool
	}{
		{
			name: "AI search healthy",
			app: &AppState{
				AIConfig:  &models.AISearchConfig{Enabled: true},
				Manticore: &MockManticoreClient{connected: true},
			},
			expected: true,
		},
		{
			name: "AI search disabled",
			app: &AppState{
				AIConfig:  &models.AISearchConfig{Enabled: false},
				Manticore: &MockManticoreClient{connected: true},
			},
			expected: false,
		},
		{
			name: "No AI config",
			app: &AppState{
				AIConfig:  nil,
				Manticore: &MockManticoreClient{connected: true},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.app.checkAISearchHealth()
			if result != tt.expected {
				t.Errorf("checkAISearchHealth() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
