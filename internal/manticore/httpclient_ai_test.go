package manticore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAISearch(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		model         string
		limit         int
		offset        int
		embeddingResp EmbeddingResponse
		searchResp    SearchResponse
		expectError   bool
		expectedHits  int32
	}{
		{
			name:   "successful AI search",
			query:  "test query",
			model:  "sentence-transformers/all-MiniLM-L6-v2",
			limit:  10,
			offset: 0,
			embeddingResp: EmbeddingResponse{
				Embedding: []float64{0.1, 0.2, 0.3},
				Model:     "sentence-transformers/all-MiniLM-L6-v2",
				Tokens:    2,
			},
			searchResp: SearchResponse{
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
			},
			expectError:  false,
			expectedHits: 2,
		},
		{
			name:        "empty query",
			query:       "",
			model:       "sentence-transformers/all-MiniLM-L6-v2",
			limit:       10,
			offset:      0,
			expectError: true,
		},
		{
			name:        "invalid model",
			query:       "test query",
			model:       "",
			limit:       10,
			offset:      0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/embedding":
					if tt.expectError && (tt.query == "" || tt.model == "") {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"error": "invalid request"}`))
						return
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(tt.embeddingResp)
				case "/search":
					if tt.expectError {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"error": "search failed"}`))
						return
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(tt.searchResp)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			// Create client
			config := DefaultHTTPClientConfig(server.URL)
			client := NewHTTPClient(config)

			// Execute AI search
			result, err := client.AISearch(tt.query, tt.model, tt.limit, tt.offset)

			// Validate results
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result == nil {
					t.Errorf("Expected result but got nil")
				} else if result.Hits.Total != tt.expectedHits {
					t.Errorf("Expected %d hits, got %d", tt.expectedHits, result.Hits.Total)
				}
			}
		})
	}
}

func TestGenerateEmbedding(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		model          string
		expectedVector []float64
		expectedTokens int
		expectError    bool
	}{
		{
			name:           "successful embedding generation",
			text:           "test text",
			model:          "sentence-transformers/all-MiniLM-L6-v2",
			expectedVector: []float64{0.1, 0.2, 0.3, 0.4},
			expectedTokens: 2,
			expectError:    false,
		},
		{
			name:        "empty text",
			text:        "",
			model:       "sentence-transformers/all-MiniLM-L6-v2",
			expectError: true,
		},
		{
			name:        "empty model",
			text:        "test text",
			model:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/embedding" {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				if tt.expectError {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error": "invalid request"}`))
					return
				}

				response := EmbeddingResponse{
					Embedding: tt.expectedVector,
					Model:     tt.model,
					Tokens:    tt.expectedTokens,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			// Create client
			config := DefaultHTTPClientConfig(server.URL)
			client := NewHTTPClient(config)

			// Execute embedding generation
			result, err := client.GenerateEmbedding(tt.text, tt.model)

			// Validate results
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if len(result) != len(tt.expectedVector) {
					t.Errorf("Expected vector length %d, got %d", len(tt.expectedVector), len(result))
				}
				for i, v := range tt.expectedVector {
					if i < len(result) && result[i] != v {
						t.Errorf("Expected vector[%d] = %f, got %f", i, v, result[i])
					}
				}
			}
		})
	}
}

func TestCreateKNNSearchRequest(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config).(*manticoreHTTPClient)

	tests := []struct {
		name        string
		index       string
		vectorField string
		queryVector []float64
		limit       int
		offset      int
	}{
		{
			name:        "basic KNN request",
			index:       "documents",
			vectorField: "content_vector",
			queryVector: []float64{0.1, 0.2, 0.3},
			limit:       10,
			offset:      0,
		},
		{
			name:        "KNN request with pagination",
			index:       "documents",
			vectorField: "content_vector",
			queryVector: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
			limit:       5,
			offset:      10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := client.CreateKNNSearchRequest(tt.index, tt.vectorField, tt.queryVector, tt.limit, tt.offset)

			// Validate request structure
			if request.Index != tt.index {
				t.Errorf("Expected index %s, got %s", tt.index, request.Index)
			}
			if request.Limit != int32(tt.limit) {
				t.Errorf("Expected limit %d, got %d", tt.limit, request.Limit)
			}
			if request.Offset != int32(tt.offset) {
				t.Errorf("Expected offset %d, got %d", tt.offset, request.Offset)
			}

			// Validate KNN query structure
			knnQuery, ok := request.Query["knn"].(map[string]interface{})
			if !ok {
				t.Errorf("Expected KNN query in request")
				return
			}

			if field, ok := knnQuery["field"].(string); !ok || field != tt.vectorField {
				t.Errorf("Expected field %s, got %v", tt.vectorField, knnQuery["field"])
			}

			if k, ok := knnQuery["k"].(int); !ok || k != tt.limit {
				t.Errorf("Expected k %d, got %v", tt.limit, knnQuery["k"])
			}

			if queryVector, ok := knnQuery["query_vector"].([]float64); !ok || len(queryVector) != len(tt.queryVector) {
				t.Errorf("Expected query_vector length %d, got %v", len(tt.queryVector), knnQuery["query_vector"])
			}
		})
	}
}

func TestCreateHybridAISearchRequest(t *testing.T) {
	config := DefaultHTTPClientConfig("http://localhost:9308")
	client := NewHTTPClient(config).(*manticoreHTTPClient)

	tests := []struct {
		name        string
		index       string
		textQuery   string
		queryVector []float64
		limit       int
		offset      int
	}{
		{
			name:        "basic hybrid request",
			index:       "documents",
			textQuery:   "test query",
			queryVector: []float64{0.1, 0.2, 0.3},
			limit:       10,
			offset:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := client.CreateHybridAISearchRequest(tt.index, tt.textQuery, tt.queryVector, tt.limit, tt.offset)

			// Validate request structure
			if request.Index != tt.index {
				t.Errorf("Expected index %s, got %s", tt.index, request.Index)
			}

			// Validate bool query structure
			boolQuery, ok := request.Query["bool"].(map[string]interface{})
			if !ok {
				t.Errorf("Expected bool query in request")
				return
			}

			should, ok := boolQuery["should"].([]map[string]interface{})
			if !ok || len(should) != 2 {
				t.Errorf("Expected 2 should clauses, got %v", boolQuery["should"])
				return
			}

			// Check for match clause
			hasMatch := false
			hasKNN := false
			for _, clause := range should {
				if _, ok := clause["match"]; ok {
					hasMatch = true
				}
				if _, ok := clause["knn"]; ok {
					hasKNN = true
				}
			}

			if !hasMatch {
				t.Errorf("Expected match clause in hybrid query")
			}
			if !hasKNN {
				t.Errorf("Expected KNN clause in hybrid query")
			}
		})
	}
}

func TestValidateAISearchCapability(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectError    bool
	}{
		{
			name: "AI search available",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/embedding" {
					response := EmbeddingResponse{
						Embedding: []float64{0.1, 0.2, 0.3},
						Model:     "sentence-transformers/all-MiniLM-L6-v2",
						Tokens:    1,
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(response)
				}
			},
			expectError: false,
		},
		{
			name: "AI search unavailable",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/embedding" {
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte(`{"error": "endpoint not found"}`))
				}
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			// Create client
			config := DefaultHTTPClientConfig(server.URL)
			client := NewHTTPClient(config).(*manticoreHTTPClient)

			// Test AI search capability
			err := client.ValidateAISearchCapability()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestGetAISearchStatus(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectHealthy  bool
	}{
		{
			name: "healthy AI search",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/embedding" {
					response := EmbeddingResponse{
						Embedding: []float64{0.1, 0.2, 0.3},
						Model:     "sentence-transformers/all-MiniLM-L6-v2",
						Tokens:    1,
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(response)
				}
			},
			expectHealthy: true,
		},
		{
			name: "unhealthy AI search",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/embedding" {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error": "internal error"}`))
				}
			},
			expectHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			// Create client
			config := DefaultHTTPClientConfig(server.URL)
			client := NewHTTPClient(config).(*manticoreHTTPClient)

			// Get AI search status
			status := client.GetAISearchStatus()

			// Validate status
			if available, ok := status["ai_search_available"].(bool); !ok {
				t.Errorf("Expected ai_search_available field in status")
			} else if available != tt.expectHealthy {
				t.Errorf("Expected ai_search_available %v, got %v", tt.expectHealthy, available)
			}

			if endpoint, ok := status["embedding_endpoint"].(string); !ok {
				t.Errorf("Expected embedding_endpoint field in status")
			} else if endpoint != server.URL+"/embedding" {
				t.Errorf("Expected embedding_endpoint %s, got %s", server.URL+"/embedding", endpoint)
			}

			if _, ok := status["last_check"].(string); !ok {
				t.Errorf("Expected last_check field in status")
			}
		})
	}
}

func TestUtilityFunctions(t *testing.T) {
	t.Run("formatVectorAsJSONArray", func(t *testing.T) {
		vector := []float64{0.1, 0.2, 0.3}
		result := formatVectorAsJSONArray(vector)
		// Go JSON marshaler uses full precision, so we need to check the parsed result
		var parsedVector []float64
		err := json.Unmarshal([]byte(result), &parsedVector)
		if err != nil {
			t.Errorf("Failed to parse result JSON: %v", err)
		}
		if len(parsedVector) != len(vector) {
			t.Errorf("Expected length %d, got %d", len(vector), len(parsedVector))
		}
		for i, v := range vector {
			if parsedVector[i] != v {
				t.Errorf("Expected parsedVector[%d] = %f, got %f", i, v, parsedVector[i])
			}
		}
	})

	t.Run("parseVectorFromJSONArray", func(t *testing.T) {
		jsonStr := "[0.1,0.2,0.3]"
		result, err := parseVectorFromJSONArray(jsonStr)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		expected := []float64{0.1, 0.2, 0.3}
		if len(result) != len(expected) {
			t.Errorf("Expected length %d, got %d", len(expected), len(result))
		}
		for i, v := range expected {
			if result[i] != v {
				t.Errorf("Expected result[%d] = %f, got %f", i, v, result[i])
			}
		}
	})

	t.Run("cosineSimilarity", func(t *testing.T) {
		config := DefaultHTTPClientConfig("http://localhost:9308")
		client := NewHTTPClient(config).(*manticoreHTTPClient)

		a := []float64{1.0, 0.0, 0.0}
		b := []float64{1.0, 0.0, 0.0}
		result := client.cosineSimilarity(a, b)
		expected := 1.0
		if result != expected {
			t.Errorf("Expected similarity %f, got %f", expected, result)
		}

		// Test orthogonal vectors
		c := []float64{1.0, 0.0}
		d := []float64{0.0, 1.0}
		result2 := client.cosineSimilarity(c, d)
		expected2 := 0.0
		if result2 != expected2 {
			t.Errorf("Expected similarity %f, got %f", expected2, result2)
		}
	})
}

// Benchmark tests
func BenchmarkAISearch(b *testing.B) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/embedding":
			response := EmbeddingResponse{
				Embedding: make([]float64, 384), // Typical embedding size
				Model:     "sentence-transformers/all-MiniLM-L6-v2",
				Tokens:    10,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		case "/search":
			response := SearchResponse{
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
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	// Create client
	config := DefaultHTTPClientConfig(server.URL)
	config.Timeout = 5 * time.Second
	client := NewHTTPClient(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.AISearch("benchmark query", "sentence-transformers/all-MiniLM-L6-v2", 10, 0)
		if err != nil {
			b.Errorf("Benchmark failed: %v", err)
		}
	}
}

func BenchmarkGenerateEmbedding(b *testing.B) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/embedding" {
			response := EmbeddingResponse{
				Embedding: make([]float64, 384), // Typical embedding size
				Model:     "sentence-transformers/all-MiniLM-L6-v2",
				Tokens:    10,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	// Create client
	config := DefaultHTTPClientConfig(server.URL)
	config.Timeout = 5 * time.Second
	client := NewHTTPClient(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.GenerateEmbedding("benchmark text", "sentence-transformers/all-MiniLM-L6-v2")
		if err != nil {
			b.Errorf("Benchmark failed: %v", err)
		}
	}
}
