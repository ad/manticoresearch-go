package manticore

import (
	"testing"
	"time"
)

func TestMetricsCollector_AISearchMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	// Test AI search operation recording
	model := "test-model"
	duration := 100 * time.Millisecond

	// Record successful AI search
	collector.RecordAISearchOperation(model, duration, true, "")

	// Record failed AI search
	collector.RecordAISearchOperation(model, 200*time.Millisecond, false, "timeout")

	// Record AI embedding operation
	collector.RecordAIEmbeddingOperation(model, 50*time.Millisecond, true, "")

	// Get metrics
	metrics := collector.GetMetrics()

	// Verify AI search metrics
	if metrics.AISearchOperations != 2 {
		t.Errorf("Expected 2 AI search operations, got %d", metrics.AISearchOperations)
	}

	if metrics.AISearchSuccessCount != 1 {
		t.Errorf("Expected 1 successful AI search, got %d", metrics.AISearchSuccessCount)
	}

	if metrics.AISearchErrorCount != 1 {
		t.Errorf("Expected 1 failed AI search, got %d", metrics.AISearchErrorCount)
	}

	if metrics.AISearchSuccessRate != 50.0 {
		t.Errorf("Expected 50%% success rate, got %.2f%%", metrics.AISearchSuccessRate)
	}

	if metrics.AIEmbeddingOperations != 1 {
		t.Errorf("Expected 1 AI embedding operation, got %d", metrics.AIEmbeddingOperations)
	}

	// Verify model usage tracking
	if metrics.AIModelUsage[model] != 2 {
		t.Errorf("Expected 2 uses of model %s, got %d", model, metrics.AIModelUsage[model])
	}

	// Verify error type tracking
	if metrics.AISearchErrorTypes["timeout"] != 1 {
		t.Errorf("Expected 1 timeout error, got %d", metrics.AISearchErrorTypes["timeout"])
	}

	// Verify average time calculation
	expectedAvgTime := (100*time.Millisecond + 200*time.Millisecond) / 2
	if metrics.AISearchAverageTime != expectedAvgTime {
		t.Errorf("Expected average time %v, got %v", expectedAvgTime, metrics.AISearchAverageTime)
	}
}

func TestMetricsCollector_LogAIMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	// Record some AI operations
	collector.RecordAISearchOperation("test-model", 100*time.Millisecond, true, "")
	collector.RecordAISearchOperation("test-model", 200*time.Millisecond, false, "network")
	collector.RecordAIEmbeddingOperation("embedding-model", 50*time.Millisecond, true, "")

	// This should log AI metrics without errors
	collector.LogMetrics()
}

func TestLogger_AISearchLogging(t *testing.T) {
	logger := NewLogger(LogLevelInfo)

	// Test AI search operation logging
	logger.LogAISearchOperation("test query", "test-model", 100*time.Millisecond, true, 5, "")
	logger.LogAISearchOperation("failed query", "test-model", 200*time.Millisecond, false, 0, "model not available")

	// Test AI embedding operation logging
	logger.LogAIEmbeddingOperation(100, "test-model", 50*time.Millisecond, true, 384, "")
	logger.LogAIEmbeddingOperation(200, "test-model", 100*time.Millisecond, false, 0, "timeout")

	// Test AI search health check logging
	logger.LogAISearchHealthCheck(true, "test-model", 10*time.Millisecond, "")
	logger.LogAISearchHealthCheck(false, "test-model", 20*time.Millisecond, "connection failed")

	// Test AI search fallback logging
	logger.LogAISearchFallback("test query", "hybrid", "AI model unavailable", 150*time.Millisecond)

	// Test AI search configuration logging
	logger.LogAISearchConfiguration("test-model", true, 30*time.Second)
}

func TestCategorizeAIError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "timeout error",
			err:      &testError{"request timeout"},
			expected: "timeout",
		},
		{
			name:     "network error",
			err:      &testError{"connection refused"},
			expected: "network",
		},
		{
			name:     "embedding error",
			err:      &testError{"embedding generation failed"},
			expected: "embedding",
		},
		{
			name:     "model error",
			err:      &testError{"model not found"},
			expected: "model",
		},
		{
			name:     "client error",
			err:      &testError{"HTTP 400 bad request"},
			expected: "client_error",
		},
		{
			name:     "server error",
			err:      &testError{"HTTP 500 internal server error"},
			expected: "server_error",
		},
		{
			name:     "parse error",
			err:      &testError{"failed to parse response"},
			expected: "parse_error",
		},
		{
			name:     "circuit breaker error",
			err:      &testError{"circuit breaker is open"},
			expected: "circuit_breaker",
		},
		{
			name:     "unknown error",
			err:      &testError{"some other error"},
			expected: "unknown",
		},
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := categorizeAIError(tt.err)
			if result != tt.expected {
				t.Errorf("categorizeAIError(%v) = %s, expected %s", tt.err, result, tt.expected)
			}
		})
	}
}

// testError is a simple error implementation for testing
type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}
