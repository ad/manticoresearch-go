package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ad/manticoresearch-go/internal/manticore"
	"github.com/ad/manticoresearch-go/internal/models"
	"github.com/ad/manticoresearch-go/pkg/api"
)

// MockAIErrorClient simulates various AI search error conditions
type MockAIErrorClient struct {
	isConnected          bool
	healthCheckError     error
	aiSearchResponse     *manticore.SearchResponse
	aiSearchError        error
	searchResponse       *models.SearchResponse
	searchError          error
	simulateTimeout      bool
	simulateNetworkError bool
	simulateModelError   bool
	callCount            int
}

func (m *MockAIErrorClient) WaitForReady(timeout time.Duration) error           { return nil }
func (m *MockAIErrorClient) HealthCheck() error                                 { return m.healthCheckError }
func (m *MockAIErrorClient) Close() error                                       { return nil }
func (m *MockAIErrorClient) IsConnected() bool                                  { return m.isConnected }
func (m *MockAIErrorClient) CreateSchema(aiConfig *models.AISearchConfig) error { return nil }
func (m *MockAIErrorClient) ResetDatabase() error                               { return nil }
func (m *MockAIErrorClient) TruncateTables() error                              { return nil }
func (m *MockAIErrorClient) IndexDocument(doc *models.Document, vector []float64) error {
	return nil
}
func (m *MockAIErrorClient) IndexDocuments(documents []*models.Document, vectors [][]float64) error {
	return nil
}
func (m *MockAIErrorClient) GetAllDocuments() ([]*models.Document, error) { return nil, nil }
func (m *MockAIErrorClient) SearchWithRequest(request manticore.SearchRequest) (*manticore.SearchResponse, error) {
	return nil, nil
}

func (m *MockAIErrorClient) Search(query string, mode models.SearchMode, page, pageSize int) (*models.SearchResponse, error) {
	m.callCount++

	if m.simulateTimeout {
		time.Sleep(100 * time.Millisecond)
		return nil, errors.New("search timeout")
	}

	if m.simulateNetworkError {
		return nil, errors.New("network connection failed")
	}

	return m.searchResponse, m.searchError
}

func (m *MockAIErrorClient) AISearch(query string, model string, limit, offset int) (*manticore.SearchResponse, error) {
	m.callCount++

	if m.simulateTimeout {
		time.Sleep(100 * time.Millisecond)
		return nil, errors.New("context deadline exceeded")
	}

	if m.simulateNetworkError {
		return nil, errors.New("network connection failed")
	}

	if m.simulateModelError {
		return nil, errors.New("AI model not available")
	}

	return m.aiSearchResponse, m.aiSearchError
}

func (m *MockAIErrorClient) GenerateEmbedding(text string, model string) ([]float64, error) {
	if m.simulateModelError {
		return nil, errors.New("embedding model error")
	}
	return []float64{0.1, 0.2, 0.3}, nil
}

// TestAISearchErrorHandlingComprehensive provides comprehensive testing for AI search error handling and fallback behavior
func TestAISearchErrorHandlingComprehensive(t *testing.T) {
	t.Run("AI Search Unavailable Scenarios", func(t *testing.T) {
		testAISearchUnavailableScenarios(t)
	})

	t.Run("AI Search Failure with Fallback", func(t *testing.T) {
		testAISearchFailureWithFallback(t)
	})

	t.Run("AI Search Complete Failure", func(t *testing.T) {
		testAISearchCompleteFailure(t)
	})

	t.Run("AI Search Error Categorization", func(t *testing.T) {
		testAISearchErrorCategorization(t)
	})

	t.Run("AI Search Status and Health Checks", func(t *testing.T) {
		testAISearchStatusAndHealthChecks(t)
	})
}

func testAISearchUnavailableScenarios(t *testing.T) {
	tests := []struct {
		name                string
		aiConfig            *models.AISearchConfig
		clientConnected     bool
		clientHealthError   error
		expectedStatusCode  int
		expectedErrorType   string
		expectedSuggestions []string
	}{
		{
			name:                "AI search disabled in config",
			aiConfig:            &models.AISearchConfig{Enabled: false},
			clientConnected:     true,
			expectedStatusCode:  http.StatusServiceUnavailable,
			expectedErrorType:   "ai_search_unavailable",
			expectedSuggestions: []string{"hybrid", "fulltext", "vector"},
		},
		{
			name:                "nil AI config",
			aiConfig:            nil,
			clientConnected:     true,
			expectedStatusCode:  http.StatusServiceUnavailable,
			expectedErrorType:   "ai_search_unavailable",
			expectedSuggestions: []string{"hybrid", "fulltext", "vector"},
		},
		{
			name: "client not connected",
			aiConfig: &models.AISearchConfig{
				Model:   "test-model",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
			clientConnected:     false,
			expectedStatusCode:  http.StatusServiceUnavailable,
			expectedErrorType:   "ai_search_unavailable",
			expectedSuggestions: []string{"hybrid", "fulltext", "vector"},
		},
		{
			name: "client health check failed",
			aiConfig: &models.AISearchConfig{
				Model:   "test-model",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
			clientConnected:     true,
			clientHealthError:   errors.New("health check failed"),
			expectedStatusCode:  http.StatusServiceUnavailable,
			expectedErrorType:   "ai_search_unavailable",
			expectedSuggestions: []string{"hybrid", "fulltext", "vector"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			mockClient := &MockAIErrorClient{
				isConnected:      tt.clientConnected,
				healthCheckError: tt.clientHealthError,
			}

			// Create app state
			app := &AppState{
				Documents:  []*models.Document{},
				Vectorizer: nil,
				Manticore:  mockClient,
				Vectors:    [][]float64{},
				AIConfig:   tt.aiConfig,
			}

			// Create request
			req := httptest.NewRequest("GET", "/api/search?query=test&mode=ai", nil)
			w := httptest.NewRecorder()

			// Handle request
			app.SearchHandler(w, req)

			// Verify response
			if w.Code != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatusCode, w.Code)
			}

			var response api.APIResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if response.Success {
				t.Errorf("Expected unsuccessful response")
			}

			// Check error type in response data
			if data, ok := response.Data.(map[string]interface{}); ok {
				if errorType, exists := data["error_type"]; exists {
					if errorType != tt.expectedErrorType {
						t.Errorf("Expected error type %s, got %v", tt.expectedErrorType, errorType)
					}
				} else {
					t.Errorf("Expected error_type in response data")
				}

				// Check suggested modes
				if suggestions, exists := data["suggested_modes"]; exists {
					if suggestionsSlice, ok := suggestions.([]interface{}); ok {
						for _, expectedSuggestion := range tt.expectedSuggestions {
							found := false
							for _, suggestion := range suggestionsSlice {
								if suggestion == expectedSuggestion {
									found = true
									break
								}
							}
							if !found {
								t.Errorf("Expected suggestion %s not found in %v", expectedSuggestion, suggestionsSlice)
							}
						}
					}
				}
			}
		})
	}
}

func testAISearchFailureWithFallback(t *testing.T) {
	tests := []struct {
		name               string
		aiError            error
		fallbackResponse   *models.SearchResponse
		fallbackError      error
		expectedStatusCode int
		expectedMode       string
		expectedFallback   bool
	}{
		{
			name:    "AI search fails, fallback succeeds",
			aiError: errors.New("AI search timeout"),
			fallbackResponse: &models.SearchResponse{
				Documents: []models.SearchResult{
					{
						Document: &models.Document{
							ID:      1,
							Title:   "Fallback Document",
							Content: "Fallback content",
							URL:     "http://example.com/fallback",
						},
						Score: 0.8,
					},
				},
				Total: 1,
				Page:  1,
				Mode:  string(models.SearchModeHybrid),
			},
			fallbackError:      nil,
			expectedStatusCode: http.StatusOK,
			expectedMode:       "hybrid (AI fallback)",
			expectedFallback:   true,
		},
		{
			name:    "AI search network error, fallback succeeds",
			aiError: errors.New("network connection failed"),
			fallbackResponse: &models.SearchResponse{
				Documents: []models.SearchResult{},
				Total:     0,
				Page:      1,
				Mode:      string(models.SearchModeHybrid),
			},
			fallbackError:      nil,
			expectedStatusCode: http.StatusOK,
			expectedMode:       "hybrid (AI fallback)",
			expectedFallback:   true,
		},
		{
			name:    "AI search model error, fallback succeeds",
			aiError: errors.New("AI model not available"),
			fallbackResponse: &models.SearchResponse{
				Documents: []models.SearchResult{
					{
						Document: &models.Document{
							ID:      2,
							Title:   "Model Fallback Document",
							Content: "Model fallback content",
							URL:     "http://example.com/model-fallback",
						},
						Score: 0.7,
					},
				},
				Total: 1,
				Page:  1,
				Mode:  string(models.SearchModeHybrid),
			},
			fallbackError:      nil,
			expectedStatusCode: http.StatusOK,
			expectedMode:       "hybrid (AI fallback)",
			expectedFallback:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client that fails AI search but succeeds fallback
			mockClient := &MockAIErrorClient{
				isConnected:    true,
				aiSearchError:  tt.aiError,
				searchResponse: tt.fallbackResponse,
				searchError:    tt.fallbackError,
			}

			// Create app state with AI enabled
			app := &AppState{
				Documents:  []*models.Document{},
				Vectorizer: nil,
				Manticore:  mockClient,
				Vectors:    [][]float64{},
				AIConfig: &models.AISearchConfig{
					Model:   "test-model",
					Enabled: true,
					Timeout: 30 * time.Second,
				},
			}

			// Create request
			req := httptest.NewRequest("GET", "/api/search?query=test&mode=ai", nil)
			w := httptest.NewRecorder()

			// Handle request
			app.SearchHandler(w, req)

			// Verify response
			if w.Code != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatusCode, w.Code)
			}

			var response api.APIResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if tt.expectedFallback {
				if !response.Success {
					t.Errorf("Expected successful fallback response")
				}

				// Check that the response contains fallback data
				if searchResponse, ok := response.Data.(*models.SearchResponse); ok {
					if searchResponse.Mode != tt.expectedMode {
						t.Errorf("Expected mode %s, got %s", tt.expectedMode, searchResponse.Mode)
					}
				} else {
					t.Errorf("Expected SearchResponse in successful fallback")
				}
			}

			// Verify that both AI search and fallback were attempted
			if mockClient.callCount < 2 {
				t.Errorf("Expected at least 2 calls (AI + fallback), got %d", mockClient.callCount)
			}
		})
	}
}

func testAISearchCompleteFailure(t *testing.T) {
	tests := []struct {
		name               string
		aiError            error
		fallbackError      error
		expectedStatusCode int
		expectedErrorType  string
		expectedCategory   string
		retrySuggested     bool
	}{
		{
			name:               "AI timeout, fallback timeout",
			aiError:            errors.New("context deadline exceeded"),
			fallbackError:      errors.New("search timeout"),
			expectedStatusCode: http.StatusInternalServerError,
			expectedErrorType:  "ai_search_failure",
			expectedCategory:   "timeout",
			retrySuggested:     true,
		},
		{
			name:               "AI network error, fallback network error",
			aiError:            errors.New("network connection failed"),
			fallbackError:      errors.New("network connection failed"),
			expectedStatusCode: http.StatusInternalServerError,
			expectedErrorType:  "ai_search_failure",
			expectedCategory:   "network",
			retrySuggested:     true,
		},
		{
			name:               "AI model error, fallback fails",
			aiError:            errors.New("AI model not available"),
			fallbackError:      errors.New("search index corrupted"),
			expectedStatusCode: http.StatusInternalServerError,
			expectedErrorType:  "ai_search_failure",
			expectedCategory:   "model",
			retrySuggested:     false,
		},
		{
			name:               "AI server error, fallback server error",
			aiError:            errors.New("HTTP 500 internal server error"),
			fallbackError:      errors.New("HTTP 503 service unavailable"),
			expectedStatusCode: http.StatusInternalServerError,
			expectedErrorType:  "ai_search_failure",
			expectedCategory:   "server_error",
			retrySuggested:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client that fails both AI search and fallback
			mockClient := &MockAIErrorClient{
				isConnected:   true,
				aiSearchError: tt.aiError,
				searchError:   tt.fallbackError,
			}

			// Create app state with AI enabled
			app := &AppState{
				Documents:  []*models.Document{},
				Vectorizer: nil,
				Manticore:  mockClient,
				Vectors:    [][]float64{},
				AIConfig: &models.AISearchConfig{
					Model:   "test-model",
					Enabled: true,
					Timeout: 30 * time.Second,
				},
			}

			// Create request
			req := httptest.NewRequest("GET", "/api/search?query=test&mode=ai", nil)
			w := httptest.NewRecorder()

			// Handle request
			app.SearchHandler(w, req)

			// Verify response
			if w.Code != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatusCode, w.Code)
			}

			var response api.APIResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if response.Success {
				t.Errorf("Expected unsuccessful response for complete failure")
			}

			// Check error details in response data
			if data, ok := response.Data.(map[string]interface{}); ok {
				if errorType, exists := data["error_type"]; exists {
					if errorType != tt.expectedErrorType {
						t.Errorf("Expected error type %s, got %v", tt.expectedErrorType, errorType)
					}
				}

				if category, exists := data["error_category"]; exists {
					if category != tt.expectedCategory {
						t.Errorf("Expected error category %s, got %v", tt.expectedCategory, category)
					}
				}

				if retrySuggested, exists := data["retry_suggested"]; exists {
					if retrySuggested != tt.retrySuggested {
						t.Errorf("Expected retry_suggested %v, got %v", tt.retrySuggested, retrySuggested)
					}
				}

				// Check that both AI and fallback errors are included
				if aiError, exists := data["ai_error"]; exists {
					if !strings.Contains(aiError.(string), tt.aiError.Error()) {
						t.Errorf("Expected AI error to contain %s, got %v", tt.aiError.Error(), aiError)
					}
				}

				if fallbackError, exists := data["fallback_error"]; exists {
					if !strings.Contains(fallbackError.(string), tt.fallbackError.Error()) {
						t.Errorf("Expected fallback error to contain %s, got %v", tt.fallbackError.Error(), fallbackError)
					}
				}
			}
		})
	}
}

func testAISearchErrorCategorization(t *testing.T) {
	tests := []struct {
		name             string
		error            error
		expectedCategory string
	}{
		{
			name:             "timeout error",
			error:            errors.New("context deadline exceeded"),
			expectedCategory: "timeout",
		},
		{
			name:             "timeout error variant",
			error:            errors.New("request timeout"),
			expectedCategory: "timeout",
		},
		{
			name:             "network error",
			error:            errors.New("network connection failed"),
			expectedCategory: "network",
		},
		{
			name:             "connection error",
			error:            errors.New("connection refused"),
			expectedCategory: "network",
		},
		{
			name:             "embedding error",
			error:            errors.New("embedding generation failed"),
			expectedCategory: "embedding",
		},
		{
			name:             "model error",
			error:            errors.New("AI model not available"),
			expectedCategory: "model",
		},
		{
			name:             "client error",
			error:            errors.New("HTTP 400 bad request"),
			expectedCategory: "client_error",
		},
		{
			name:             "server error",
			error:            errors.New("HTTP 500 internal server error"),
			expectedCategory: "server_error",
		},
		{
			name:             "unknown error",
			error:            errors.New("some unknown error"),
			expectedCategory: "unknown",
		},
	}

	app := &AppState{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := app.categorizeAISearchError(tt.error)
			if category != tt.expectedCategory {
				t.Errorf("Expected category %s, got %s", tt.expectedCategory, category)
			}
		})
	}
}

func testAISearchStatusAndHealthChecks(t *testing.T) {
	t.Run("AI Search Health Check Success", func(t *testing.T) {
		mockClient := &MockAIErrorClient{
			isConnected: true,
		}

		app := &AppState{
			Manticore: mockClient,
			AIConfig: &models.AISearchConfig{
				Model:   "test-model",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
		}

		req := httptest.NewRequest("GET", "/api/status", nil)
		w := httptest.NewRecorder()

		app.StatusHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response api.APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if !response.Success {
			t.Errorf("Expected successful status response")
		}

		if statusResp, ok := response.Data.(api.StatusResponse); ok {
			if !statusResp.AISearchEnabled {
				t.Errorf("Expected AI search to be enabled")
			}
			if statusResp.AIModel != "test-model" {
				t.Errorf("Expected AI model 'test-model', got %s", statusResp.AIModel)
			}
			if !statusResp.AISearchHealthy {
				t.Errorf("Expected AI search to be healthy")
			}
		} else {
			t.Errorf("Expected StatusResponse in response data")
		}
	})

	t.Run("AI Search Health Check Failure", func(t *testing.T) {
		mockClient := &MockAIErrorClient{
			isConnected: false, // Not connected
		}

		app := &AppState{
			Manticore: mockClient,
			AIConfig: &models.AISearchConfig{
				Model:   "test-model",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
		}

		req := httptest.NewRequest("GET", "/api/status", nil)
		w := httptest.NewRecorder()

		app.StatusHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response api.APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if statusResp, ok := response.Data.(api.StatusResponse); ok {
			if statusResp.AISearchHealthy {
				t.Errorf("Expected AI search to be unhealthy when client not connected")
			}
		}
	})

	t.Run("AI Search Disabled in Config", func(t *testing.T) {
		mockClient := &MockAIErrorClient{
			isConnected: true,
		}

		app := &AppState{
			Manticore: mockClient,
			AIConfig: &models.AISearchConfig{
				Model:   "test-model",
				Enabled: false, // Disabled
				Timeout: 30 * time.Second,
			},
		}

		req := httptest.NewRequest("GET", "/api/status", nil)
		w := httptest.NewRecorder()

		app.StatusHandler(w, req)

		var response api.APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if statusResp, ok := response.Data.(api.StatusResponse); ok {
			if statusResp.AISearchEnabled {
				t.Errorf("Expected AI search to be disabled")
			}
			if statusResp.AISearchHealthy {
				t.Errorf("Expected AI search to be unhealthy when disabled")
			}
		}
	})
}

// TestAISearchErrorResponseFormats tests that error responses are properly formatted
func TestAISearchErrorResponseFormats(t *testing.T) {
	t.Run("AI Search Unavailable Response Format", func(t *testing.T) {
		app := &AppState{
			AIConfig: &models.AISearchConfig{Enabled: false},
		}

		w := httptest.NewRecorder()
		app.sendAISearchUnavailableResponse(w, "AI search is disabled")

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, w.Code)
		}

		var response api.APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if response.Success {
			t.Errorf("Expected unsuccessful response")
		}

		if !strings.Contains(response.Error, "AI search is currently unavailable") {
			t.Errorf("Expected error message about AI search unavailability")
		}

		// Check response data structure
		if data, ok := response.Data.(map[string]interface{}); ok {
			requiredFields := []string{"error_type", "reason", "suggested_modes", "ai_enabled"}
			for _, field := range requiredFields {
				if _, exists := data[field]; !exists {
					t.Errorf("Expected field %s in response data", field)
				}
			}
		} else {
			t.Errorf("Expected map in response data")
		}
	})

	t.Run("AI Search Error Response Format", func(t *testing.T) {
		app := &AppState{}

		aiError := errors.New("AI search timeout")
		fallbackError := errors.New("fallback search failed")

		w := httptest.NewRecorder()
		app.sendAISearchErrorResponse(w, aiError, fallbackError)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, w.Code)
		}

		var response api.APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if response.Success {
			t.Errorf("Expected unsuccessful response")
		}

		// Check response data structure
		if data, ok := response.Data.(map[string]interface{}); ok {
			requiredFields := []string{"error_type", "error_category", "ai_error", "fallback_error", "suggested_modes", "retry_suggested"}
			for _, field := range requiredFields {
				if _, exists := data[field]; !exists {
					t.Errorf("Expected field %s in response data", field)
				}
			}

			// Verify error type
			if errorType, exists := data["error_type"]; exists {
				if errorType != "ai_search_failure" {
					t.Errorf("Expected error_type 'ai_search_failure', got %v", errorType)
				}
			}

			// Verify error messages are included
			if aiErrorMsg, exists := data["ai_error"]; exists {
				if !strings.Contains(aiErrorMsg.(string), "AI search timeout") {
					t.Errorf("Expected AI error message to contain 'AI search timeout'")
				}
			}

			if fallbackErrorMsg, exists := data["fallback_error"]; exists {
				if !strings.Contains(fallbackErrorMsg.(string), "fallback search failed") {
					t.Errorf("Expected fallback error message to contain 'fallback search failed'")
				}
			}
		} else {
			t.Errorf("Expected map in response data")
		}
	})
}

// TestAISearchConcurrentErrorHandling tests error handling under concurrent load
func TestAISearchConcurrentErrorHandling(t *testing.T) {
	mockClient := &MockAIErrorClient{
		isConnected:   true,
		aiSearchError: errors.New("concurrent AI search error"),
		searchResponse: &models.SearchResponse{
			Documents: []models.SearchResult{},
			Total:     0,
			Page:      1,
			Mode:      string(models.SearchModeHybrid),
		},
	}

	app := &AppState{
		Manticore: mockClient,
		AIConfig: &models.AISearchConfig{
			Model:   "test-model",
			Enabled: true,
			Timeout: 30 * time.Second,
		},
	}

	const numRequests = 10
	results := make(chan int, numRequests)

	// Launch concurrent requests
	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/api/search?query=concurrent&mode=ai", nil)
			w := httptest.NewRecorder()
			app.SearchHandler(w, req)
			results <- w.Code
		}()
	}

	// Collect results
	for i := 0; i < numRequests; i++ {
		select {
		case statusCode := <-results:
			if statusCode != http.StatusOK {
				t.Errorf("Expected status code %d for concurrent request, got %d", http.StatusOK, statusCode)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent requests")
		}
	}

	// Verify that all requests were handled (AI search + fallback for each)
	if mockClient.callCount != numRequests*2 {
		t.Errorf("Expected %d total calls, got %d", numRequests*2, mockClient.callCount)
	}
}
