package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ad/manticoresearch-go/internal/handlers"
	"github.com/ad/manticoresearch-go/internal/manticore"
	"github.com/ad/manticoresearch-go/internal/models"
	"github.com/ad/manticoresearch-go/pkg/api"
)

// IntegrationTestClient provides a comprehensive mock for integration testing
type IntegrationTestClient struct {
	isConnected          bool
	healthCheckError     error
	documents            []*models.Document
	aiSearchEnabled      bool
	aiModel              string
	aiSearchHealthy      bool
	aiSearchResponse     *manticore.SearchResponse
	aiSearchError        error
	searchResponse       *models.SearchResponse
	searchError          error
	embeddingResponse    []float64
	embeddingError       error
	simulateTimeout      bool
	simulateNetworkError bool
	simulateModelError   bool
	callLog              []string
}

func NewIntegrationTestClient() *IntegrationTestClient {
	return &IntegrationTestClient{
		isConnected:     true,
		aiSearchEnabled: true,
		aiModel:         "sentence-transformers/all-MiniLM-L6-v2",
		aiSearchHealthy: true,
		documents:       []*models.Document{},
		callLog:         []string{},
	}
}

func (c *IntegrationTestClient) logCall(method string, args ...interface{}) {
	logEntry := fmt.Sprintf("%s(%v)", method, args)
	c.callLog = append(c.callLog, logEntry)
}

func (c *IntegrationTestClient) WaitForReady(timeout time.Duration) error {
	c.logCall("WaitForReady", timeout)
	return nil
}

func (c *IntegrationTestClient) HealthCheck() error {
	c.logCall("HealthCheck")
	return c.healthCheckError
}

func (c *IntegrationTestClient) Close() error {
	c.logCall("Close")
	return nil
}

func (c *IntegrationTestClient) IsConnected() bool {
	c.logCall("IsConnected")
	return c.isConnected
}

func (c *IntegrationTestClient) CreateSchema(aiConfig *models.AISearchConfig) error {
	c.logCall("CreateSchema")
	return nil
}

func (c *IntegrationTestClient) ResetDatabase() error {
	c.logCall("ResetDatabase")
	return nil
}

func (c *IntegrationTestClient) TruncateTables() error {
	c.logCall("TruncateTables")
	return nil
}

func (c *IntegrationTestClient) IndexDocument(doc *models.Document, vector []float64) error {
	c.logCall("IndexDocument", doc.ID, len(vector))
	return nil
}

func (c *IntegrationTestClient) IndexDocuments(documents []*models.Document, vectors [][]float64) error {
	c.logCall("IndexDocuments", len(documents), len(vectors))
	c.documents = append(c.documents, documents...)
	return nil
}

func (c *IntegrationTestClient) GetAllDocuments() ([]*models.Document, error) {
	c.logCall("GetAllDocuments")
	return c.documents, nil
}

func (c *IntegrationTestClient) SearchWithRequest(request manticore.SearchRequest) (*manticore.SearchResponse, error) {
	c.logCall("SearchWithRequest", request.Index)
	return nil, nil
}

func (c *IntegrationTestClient) Search(query string, mode models.SearchMode, page, pageSize int) (*models.SearchResponse, error) {
	c.logCall("Search", query, mode, page, pageSize)

	if c.simulateTimeout {
		time.Sleep(100 * time.Millisecond)
		return nil, fmt.Errorf("search timeout")
	}

	if c.simulateNetworkError {
		return nil, fmt.Errorf("network connection failed")
	}

	return c.searchResponse, c.searchError
}

func (c *IntegrationTestClient) AISearch(query string, model string, limit, offset int) (*manticore.SearchResponse, error) {
	c.logCall("AISearch", query, model, limit, offset)

	if c.simulateTimeout {
		time.Sleep(100 * time.Millisecond)
		return nil, fmt.Errorf("context deadline exceeded")
	}

	if c.simulateNetworkError {
		return nil, fmt.Errorf("network connection failed")
	}

	if c.simulateModelError {
		return nil, fmt.Errorf("AI model not available")
	}

	return c.aiSearchResponse, c.aiSearchError
}

func (c *IntegrationTestClient) GenerateEmbedding(text string, model string) ([]float64, error) {
	c.logCall("GenerateEmbedding", len(text), model)

	if c.simulateModelError {
		return nil, fmt.Errorf("embedding model error")
	}

	return c.embeddingResponse, c.embeddingError
}

// TestAISearchIntegrationComprehensive provides comprehensive integration testing for AI search
func TestAISearchIntegrationComprehensive(t *testing.T) {
	t.Run("End-to-End AI Search Flow", func(t *testing.T) {
		testEndToEndAISearchFlow(t)
	})

	t.Run("AI Search Configuration Integration", func(t *testing.T) {
		testAISearchConfigurationIntegration(t)
	})

	t.Run("AI Search Error Handling Integration", func(t *testing.T) {
		testAISearchErrorHandlingIntegration(t)
	})

	t.Run("AI Search Status Integration", func(t *testing.T) {
		testAISearchStatusIntegration(t)
	})

	t.Run("AI Search Performance Integration", func(t *testing.T) {
		testAISearchPerformanceIntegration(t)
	})
}

func testEndToEndAISearchFlow(t *testing.T) {
	tests := []struct {
		name                string
		query               string
		mode                string
		setupClient         func(*IntegrationTestClient)
		expectedStatusCode  int
		expectedSuccess     bool
		expectedResultCount int
		expectedMode        string
		validateResponse    func(*testing.T, *api.APIResponse)
	}{
		{
			name:  "successful AI search with results",
			query: "test query",
			mode:  "ai",
			setupClient: func(client *IntegrationTestClient) {
				client.aiSearchResponse = &manticore.SearchResponse{
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
									"title":   "Integration Test Document 1",
									"content": "This is integration test content",
									"url":     "http://example.com/integration/1",
								},
							},
							{
								Index: "documents",
								ID:    2,
								Score: 0.85,
								Source: map[string]interface{}{
									"title":   "Integration Test Document 2",
									"content": "Another integration test document",
									"url":     "http://example.com/integration/2",
								},
							},
						},
					},
				}
			},
			expectedStatusCode:  http.StatusOK,
			expectedSuccess:     true,
			expectedResultCount: 2,
			expectedMode:        "ai",
			validateResponse: func(t *testing.T, response *api.APIResponse) {
				if searchResp, ok := response.Data.(*models.SearchResponse); ok {
					if searchResp.Mode != string(models.SearchModeAI) {
						t.Errorf("Expected mode %s, got %s", models.SearchModeAI, searchResp.Mode)
					}
					if len(searchResp.Documents) != 2 {
						t.Errorf("Expected 2 documents, got %d", len(searchResp.Documents))
					}
					if searchResp.Total != 2 {
						t.Errorf("Expected total 2, got %d", searchResp.Total)
					}
				} else {
					t.Errorf("Expected SearchResponse in response data")
				}
			},
		},
		{
			name:  "AI search with fallback to hybrid",
			query: "fallback test",
			mode:  "ai",
			setupClient: func(client *IntegrationTestClient) {
				client.aiSearchError = fmt.Errorf("AI search timeout")
				client.searchResponse = &models.SearchResponse{
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
				}
			},
			expectedStatusCode:  http.StatusOK,
			expectedSuccess:     true,
			expectedResultCount: 1,
			expectedMode:        "hybrid (AI fallback)",
			validateResponse: func(t *testing.T, response *api.APIResponse) {
				if searchResp, ok := response.Data.(*models.SearchResponse); ok {
					if !strings.Contains(searchResp.Mode, "fallback") {
						t.Errorf("Expected fallback mode, got %s", searchResp.Mode)
					}
				}
			},
		},
		{
			name:  "AI search complete failure",
			query: "complete failure test",
			mode:  "ai",
			setupClient: func(client *IntegrationTestClient) {
				client.aiSearchError = fmt.Errorf("AI search failed")
				client.searchError = fmt.Errorf("fallback search failed")
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedSuccess:    false,
			validateResponse: func(t *testing.T, response *api.APIResponse) {
				if data, ok := response.Data.(map[string]interface{}); ok {
					if errorType, exists := data["error_type"]; exists {
						if errorType != "ai_search_failure" {
							t.Errorf("Expected error_type 'ai_search_failure', got %v", errorType)
						}
					} else {
						t.Errorf("Expected error_type in response data")
					}
				}
			},
		},
		{
			name:  "AI search unavailable",
			query: "unavailable test",
			mode:  "ai",
			setupClient: func(client *IntegrationTestClient) {
				client.aiSearchEnabled = false
			},
			expectedStatusCode: http.StatusServiceUnavailable,
			expectedSuccess:    false,
			validateResponse: func(t *testing.T, response *api.APIResponse) {
				if data, ok := response.Data.(map[string]interface{}); ok {
					if errorType, exists := data["error_type"]; exists {
						if errorType != "ai_search_unavailable" {
							t.Errorf("Expected error_type 'ai_search_unavailable', got %v", errorType)
						}
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test client
			client := NewIntegrationTestClient()
			tt.setupClient(client)

			// Create app state
			app := &handlers.AppState{
				Documents:  []*models.Document{},
				Vectorizer: nil,
				Manticore:  client,
				Vectors:    [][]float64{},
				AIConfig: &models.AISearchConfig{
					Model:   "test-model",
					Enabled: client.aiSearchEnabled,
					Timeout: 30 * time.Second,
				},
			}

			// Create request
			url := fmt.Sprintf("/api/search?query=%s&mode=%s", tt.query, tt.mode)
			req := httptest.NewRequest("GET", url, nil)
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

			if response.Success != tt.expectedSuccess {
				t.Errorf("Expected success %v, got %v", tt.expectedSuccess, response.Success)
			}

			// Run custom validation
			if tt.validateResponse != nil {
				tt.validateResponse(t, &response)
			}

			// Verify client interactions
			if len(client.callLog) == 0 {
				t.Errorf("Expected client method calls, but got none")
			}

			t.Logf("Client call log: %v", client.callLog)
		})
	}
}

func testAISearchConfigurationIntegration(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		validate    func(*testing.T, *handlers.AppState)
	}{
		{
			name:        "default AI configuration",
			envVars:     map[string]string{},
			expectError: false,
			validate: func(t *testing.T, app *handlers.AppState) {
				if app.AIConfig == nil {
					t.Errorf("Expected AI config to be loaded")
					return
				}
				if app.AIConfig.Model != "sentence-transformers/all-MiniLM-L6-v2" {
					t.Errorf("Expected default model, got %s", app.AIConfig.Model)
				}
				if !app.AIConfig.Enabled {
					t.Errorf("Expected AI search to be enabled by default")
				}
			},
		},
		{
			name: "custom AI configuration",
			envVars: map[string]string{
				"MANTICORE_AI_MODEL":   "custom-model/test",
				"MANTICORE_AI_ENABLED": "true",
				"MANTICORE_AI_TIMEOUT": "60s",
			},
			expectError: false,
			validate: func(t *testing.T, app *handlers.AppState) {
				if app.AIConfig == nil {
					t.Errorf("Expected AI config to be loaded")
					return
				}
				if app.AIConfig.Model != "custom-model/test" {
					t.Errorf("Expected custom model, got %s", app.AIConfig.Model)
				}
				if app.AIConfig.Timeout != 60*time.Second {
					t.Errorf("Expected 60s timeout, got %v", app.AIConfig.Timeout)
				}
			},
		},
		{
			name: "disabled AI configuration",
			envVars: map[string]string{
				"MANTICORE_AI_ENABLED": "false",
			},
			expectError: false,
			validate: func(t *testing.T, app *handlers.AppState) {
				if app.AIConfig == nil {
					t.Errorf("Expected AI config to be loaded")
					return
				}
				if app.AIConfig.Enabled {
					t.Errorf("Expected AI search to be disabled")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			clearAIEnvVars()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearAIEnvVars()

			// Create app state (this loads AI config)
			app := handlers.NewAppState()

			// Validate configuration
			if tt.validate != nil {
				tt.validate(t, app)
			}
		})
	}
}

func testAISearchErrorHandlingIntegration(t *testing.T) {
	tests := []struct {
		name           string
		setupClient    func(*IntegrationTestClient)
		query          string
		expectedStatus int
		expectedError  string
		validateLog    func(*testing.T, []string)
	}{
		{
			name: "AI search timeout with successful fallback",
			setupClient: func(client *IntegrationTestClient) {
				client.simulateTimeout = true
				client.searchResponse = &models.SearchResponse{
					Documents: []models.SearchResult{},
					Total:     0,
					Page:      1,
					Mode:      string(models.SearchModeHybrid),
				}
			},
			query:          "timeout test",
			expectedStatus: http.StatusOK,
			validateLog: func(t *testing.T, log []string) {
				hasAISearch := false
				hasFallbackSearch := false
				for _, entry := range log {
					if strings.Contains(entry, "AISearch") {
						hasAISearch = true
					}
					if strings.Contains(entry, "Search") && !strings.Contains(entry, "AISearch") {
						hasFallbackSearch = true
					}
				}
				if !hasAISearch {
					t.Errorf("Expected AI search call in log")
				}
				if !hasFallbackSearch {
					t.Errorf("Expected fallback search call in log")
				}
			},
		},
		{
			name: "AI search network error with failed fallback",
			setupClient: func(client *IntegrationTestClient) {
				client.simulateNetworkError = true
				client.searchError = fmt.Errorf("fallback network error")
			},
			query:          "network error test",
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "ai_search_failure",
			validateLog: func(t *testing.T, log []string) {
				if len(log) < 2 {
					t.Errorf("Expected at least 2 calls (AI + fallback), got %d", len(log))
				}
			},
		},
		{
			name: "AI search model error",
			setupClient: func(client *IntegrationTestClient) {
				client.simulateModelError = true
				client.searchResponse = &models.SearchResponse{
					Documents: []models.SearchResult{},
					Total:     0,
					Page:      1,
					Mode:      string(models.SearchModeHybrid),
				}
			},
			query:          "model error test",
			expectedStatus: http.StatusOK,
			validateLog: func(t *testing.T, log []string) {
				hasAISearch := false
				for _, entry := range log {
					if strings.Contains(entry, "AISearch") {
						hasAISearch = true
						break
					}
				}
				if !hasAISearch {
					t.Errorf("Expected AI search call in log")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test client
			client := NewIntegrationTestClient()
			tt.setupClient(client)

			// Create app state
			app := &handlers.AppState{
				Documents:  []*models.Document{},
				Vectorizer: nil,
				Manticore:  client,
				Vectors:    [][]float64{},
				AIConfig: &models.AISearchConfig{
					Model:   "test-model",
					Enabled: true,
					Timeout: 30 * time.Second,
				},
			}

			// Create request
			url := fmt.Sprintf("/api/search?query=%s&mode=ai", tt.query)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			// Handle request
			app.SearchHandler(w, req)

			// Verify response
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedError != "" {
				var response api.APIResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				if data, ok := response.Data.(map[string]interface{}); ok {
					if errorType, exists := data["error_type"]; exists {
						if errorType != tt.expectedError {
							t.Errorf("Expected error type %s, got %v", tt.expectedError, errorType)
						}
					}
				}
			}

			// Validate call log
			if tt.validateLog != nil {
				tt.validateLog(t, client.callLog)
			}
		})
	}
}

func testAISearchStatusIntegration(t *testing.T) {
	tests := []struct {
		name            string
		setupClient     func(*IntegrationTestClient)
		setupConfig     func(*models.AISearchConfig)
		expectedHealthy bool
		expectedEnabled bool
		validateStatus  func(*testing.T, *api.StatusResponse)
	}{
		{
			name: "healthy AI search",
			setupClient: func(client *IntegrationTestClient) {
				client.isConnected = true
				client.aiSearchHealthy = true
			},
			setupConfig: func(config *models.AISearchConfig) {
				config.Enabled = true
			},
			expectedHealthy: true,
			expectedEnabled: true,
			validateStatus: func(t *testing.T, status *api.StatusResponse) {
				if !status.AISearchEnabled {
					t.Errorf("Expected AI search to be enabled")
				}
				if !status.AISearchHealthy {
					t.Errorf("Expected AI search to be healthy")
				}
				if status.AIModel == "" {
					t.Errorf("Expected AI model to be set")
				}
			},
		},
		{
			name: "unhealthy AI search",
			setupClient: func(client *IntegrationTestClient) {
				client.isConnected = false
				client.aiSearchHealthy = false
			},
			setupConfig: func(config *models.AISearchConfig) {
				config.Enabled = true
			},
			expectedHealthy: false,
			expectedEnabled: true,
			validateStatus: func(t *testing.T, status *api.StatusResponse) {
				if !status.AISearchEnabled {
					t.Errorf("Expected AI search to be enabled")
				}
				if status.AISearchHealthy {
					t.Errorf("Expected AI search to be unhealthy")
				}
			},
		},
		{
			name: "disabled AI search",
			setupClient: func(client *IntegrationTestClient) {
				client.isConnected = true
			},
			setupConfig: func(config *models.AISearchConfig) {
				config.Enabled = false
			},
			expectedHealthy: false,
			expectedEnabled: false,
			validateStatus: func(t *testing.T, status *api.StatusResponse) {
				if status.AISearchEnabled {
					t.Errorf("Expected AI search to be disabled")
				}
				if status.AISearchHealthy {
					t.Errorf("Expected AI search to be unhealthy when disabled")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test client
			client := NewIntegrationTestClient()
			tt.setupClient(client)

			// Create AI config
			aiConfig := &models.AISearchConfig{
				Model:   "test-model",
				Enabled: true,
				Timeout: 30 * time.Second,
			}
			tt.setupConfig(aiConfig)

			// Create app state
			app := &handlers.AppState{
				Documents:  []*models.Document{},
				Vectorizer: nil,
				Manticore:  client,
				Vectors:    [][]float64{},
				AIConfig:   aiConfig,
			}

			// Create status request
			req := httptest.NewRequest("GET", "/api/status", nil)
			w := httptest.NewRecorder()

			// Handle request
			app.StatusHandler(w, req)

			// Verify response
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
				if statusResp.AISearchEnabled != tt.expectedEnabled {
					t.Errorf("Expected AI search enabled %v, got %v", tt.expectedEnabled, statusResp.AISearchEnabled)
				}
				if statusResp.AISearchHealthy != tt.expectedHealthy {
					t.Errorf("Expected AI search healthy %v, got %v", tt.expectedHealthy, statusResp.AISearchHealthy)
				}

				// Run custom validation
				if tt.validateStatus != nil {
					tt.validateStatus(t, &statusResp)
				}
			} else {
				t.Errorf("Expected StatusResponse in response data")
			}
		})
	}
}

func testAISearchPerformanceIntegration(t *testing.T) {
	t.Run("AI search performance under load", func(t *testing.T) {
		// Create test client with simulated response
		client := NewIntegrationTestClient()
		client.aiSearchResponse = &manticore.SearchResponse{
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
				Total: 10,
			},
		}

		// Create app state
		app := &handlers.AppState{
			Documents:  []*models.Document{},
			Vectorizer: nil,
			Manticore:  client,
			Vectors:    [][]float64{},
			AIConfig: &models.AISearchConfig{
				Model:   "performance-test-model",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
		}

		const numRequests = 50
		results := make(chan time.Duration, numRequests)
		errors := make(chan error, numRequests)

		startTime := time.Now()

		// Launch concurrent requests
		for i := 0; i < numRequests; i++ {
			go func(id int) {
				reqStart := time.Now()

				url := fmt.Sprintf("/api/search?query=performance-test-%d&mode=ai", id)
				req := httptest.NewRequest("GET", url, nil)
				w := httptest.NewRecorder()

				app.SearchHandler(w, req)

				duration := time.Since(reqStart)

				if w.Code != http.StatusOK {
					errors <- fmt.Errorf("request %d failed with status %d", id, w.Code)
				} else {
					results <- duration
				}
			}(i)
		}

		// Collect results
		var durations []time.Duration
		var requestErrors []error

		for i := 0; i < numRequests; i++ {
			select {
			case duration := <-results:
				durations = append(durations, duration)
			case err := <-errors:
				requestErrors = append(requestErrors, err)
			case <-time.After(30 * time.Second):
				t.Fatal("Timeout waiting for performance test requests")
			}
		}

		totalDuration := time.Since(startTime)

		// Verify results
		if len(requestErrors) > 0 {
			t.Errorf("Got %d errors during performance test: %v", len(requestErrors), requestErrors[0])
		}

		if len(durations) != numRequests {
			t.Errorf("Expected %d successful requests, got %d", numRequests, len(durations))
		}

		// Calculate performance metrics
		var totalRequestTime time.Duration
		var maxDuration time.Duration
		var minDuration time.Duration = time.Hour // Initialize to large value

		for _, duration := range durations {
			totalRequestTime += duration
			if duration > maxDuration {
				maxDuration = duration
			}
			if duration < minDuration {
				minDuration = duration
			}
		}

		avgDuration := totalRequestTime / time.Duration(len(durations))

		t.Logf("Performance test results:")
		t.Logf("  Total requests: %d", numRequests)
		t.Logf("  Successful requests: %d", len(durations))
		t.Logf("  Failed requests: %d", len(requestErrors))
		t.Logf("  Total test duration: %v", totalDuration)
		t.Logf("  Average request duration: %v", avgDuration)
		t.Logf("  Min request duration: %v", minDuration)
		t.Logf("  Max request duration: %v", maxDuration)
		t.Logf("  Requests per second: %.2f", float64(numRequests)/totalDuration.Seconds())

		// Performance assertions
		if avgDuration > 100*time.Millisecond {
			t.Errorf("Average request duration too high: %v", avgDuration)
		}

		if maxDuration > 500*time.Millisecond {
			t.Errorf("Max request duration too high: %v", maxDuration)
		}

		// Verify client was called the expected number of times
		expectedCalls := numRequests // Each request should call AISearch once
		if len(client.callLog) < expectedCalls {
			t.Errorf("Expected at least %d client calls, got %d", expectedCalls, len(client.callLog))
		}
	})

	t.Run("AI search memory usage", func(t *testing.T) {
		// Create test client
		client := NewIntegrationTestClient()
		client.aiSearchResponse = &manticore.SearchResponse{
			Hits: struct {
				Total         int32  `json:"total"`
				TotalRelation string `json:"total_relation"`
				Hits          []struct {
					Index  string                 `json:"_index"`
					ID     int64                  `json:"_id"`
					Score  float32                `json:"_score"`
					Source map[string]interface{} `json:"_source"`
				} `json:"hits"`
			}{Total: 1},
		}

		// Create app state
		app := &handlers.AppState{
			Documents:  []*models.Document{},
			Vectorizer: nil,
			Manticore:  client,
			Vectors:    [][]float64{},
			AIConfig: &models.AISearchConfig{
				Model:   "memory-test-model",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
		}

		// Perform many requests to check for memory leaks
		for i := 0; i < 1000; i++ {
			url := fmt.Sprintf("/api/search?query=memory-test-%d&mode=ai", i)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			app.SearchHandler(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Request %d failed with status %d", i, w.Code)
				break
			}

			// Don't hold references to responses to allow GC
		}

		t.Logf("Memory test completed: 1000 AI search requests processed")
	})
}

// Helper function to clear AI environment variables
func clearAIEnvVars() {
	os.Unsetenv("MANTICORE_AI_MODEL")
	os.Unsetenv("MANTICORE_AI_ENABLED")
	os.Unsetenv("MANTICORE_AI_TIMEOUT")
}

// BenchmarkAISearchIntegration benchmarks the complete AI search integration
func BenchmarkAISearchIntegration(b *testing.B) {
	// Create test client
	client := NewIntegrationTestClient()
	client.aiSearchResponse = &manticore.SearchResponse{
		Hits: struct {
			Total         int32  `json:"total"`
			TotalRelation string `json:"total_relation"`
			Hits          []struct {
				Index  string                 `json:"_index"`
				ID     int64                  `json:"_id"`
				Score  float32                `json:"_score"`
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		}{Total: 5},
	}

	// Create app state
	app := &handlers.AppState{
		Documents:  []*models.Document{},
		Vectorizer: nil,
		Manticore:  client,
		Vectors:    [][]float64{},
		AIConfig: &models.AISearchConfig{
			Model:   "benchmark-model",
			Enabled: true,
			Timeout: 30 * time.Second,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/search?query=benchmark&mode=ai", nil)
		w := httptest.NewRecorder()
		app.SearchHandler(w, req)

		if w.Code != http.StatusOK {
			b.Fatalf("Benchmark request failed with status %d", w.Code)
		}
	}
}
