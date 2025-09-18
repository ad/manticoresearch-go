package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ad/manticoresearch-go/internal/models"
	"github.com/ad/manticoresearch-go/pkg/api"
)

func TestAISearchErrorHandling(t *testing.T) {
	// Test AI search unavailable scenario
	t.Run("AI search unavailable", func(t *testing.T) {
		app := NewAppState()
		app.AIConfig = &models.AISearchConfig{
			Model:   "test-model",
			Enabled: false, // Disabled to test error handling
		}

		req := httptest.NewRequest("GET", "/api/search?query=test&mode=ai", nil)
		w := httptest.NewRecorder()

		app.SearchHandler(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status 503, got %d", w.Code)
		}

		var response api.APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if response.Success {
			t.Error("Expected success to be false")
		}

		if !strings.Contains(response.Error, "AI search is currently unavailable") {
			t.Errorf("Expected unavailable error message, got: %s", response.Error)
		}

		// Check error data
		data, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Expected data to be a map")
		}

		if data["error_type"] != "ai_search_unavailable" {
			t.Errorf("Expected error_type to be ai_search_unavailable, got: %v", data["error_type"])
		}
	})

	// Test error categorization
	t.Run("Error categorization", func(t *testing.T) {
		app := NewAppState()

		tests := []struct {
			error    string
			expected string
		}{
			{"connection timeout", "timeout"},
			{"network error", "network"},
			{"embedding generation failed", "embedding"},
			{"model not found", "model"},
			{"HTTP 404 error", "client_error"},
			{"HTTP 500 error", "server_error"},
			{"unknown error", "unknown"},
		}

		for _, test := range tests {
			category := app.categorizeAISearchError(mockError(test.error))
			if category != test.expected {
				t.Errorf("For error '%s', expected category '%s', got '%s'",
					test.error, test.expected, category)
			}
		}
	})
}

// Helper function to create a mock error
func mockError(message string) error {
	return &mockErr{message: message}
}

type mockErr struct {
	message string
}

func (e *mockErr) Error() string {
	return e.message
}
