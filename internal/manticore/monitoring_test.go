package manticore

import (
	"testing"
	"time"
)

func TestMetricsCollector_RecordRequest(t *testing.T) {
	collector := NewMetricsCollector()

	// Record some successful requests
	collector.RecordRequest("test_operation", 100*time.Millisecond, true, "")
	collector.RecordRequest("test_operation", 200*time.Millisecond, true, "")
	collector.RecordRequest("test_operation", 150*time.Millisecond, false, "timeout")

	metrics := collector.GetMetrics()

	if metrics.RequestCount != 3 {
		t.Errorf("Expected RequestCount=3, got %d", metrics.RequestCount)
	}

	if metrics.SuccessCount != 2 {
		t.Errorf("Expected SuccessCount=2, got %d", metrics.SuccessCount)
	}

	if metrics.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount=1, got %d", metrics.ErrorCount)
	}

	if metrics.SuccessRate != 66.66666666666666 {
		t.Errorf("Expected SuccessRate=66.67%%, got %.2f%%", metrics.SuccessRate)
	}

	if metrics.OperationTypes["test_operation"] != 3 {
		t.Errorf("Expected test_operation count=3, got %d", metrics.OperationTypes["test_operation"])
	}

	if metrics.ErrorTypes["timeout"] != 1 {
		t.Errorf("Expected timeout error count=1, got %d", metrics.ErrorTypes["timeout"])
	}
}

func TestMetricsCollector_CircuitBreakerMetrics(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordCircuitBreakerOpen()
	collector.RecordCircuitBreakerOpen()
	collector.RecordCircuitBreakerClose()

	metrics := collector.GetMetrics()

	if metrics.CircuitBreakerOpens != 2 {
		t.Errorf("Expected CircuitBreakerOpens=2, got %d", metrics.CircuitBreakerOpens)
	}

	if metrics.CircuitBreakerCloses != 1 {
		t.Errorf("Expected CircuitBreakerCloses=1, got %d", metrics.CircuitBreakerCloses)
	}
}

func TestMetricsCollector_BulkOperations(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordBulkOperation(100)
	collector.RecordBulkOperation(50)

	metrics := collector.GetMetrics()

	if metrics.BulkOperations != 2 {
		t.Errorf("Expected BulkOperations=2, got %d", metrics.BulkOperations)
	}

	if metrics.BulkDocumentsIndexed != 150 {
		t.Errorf("Expected BulkDocumentsIndexed=150, got %d", metrics.BulkDocumentsIndexed)
	}
}

func TestMetricsCollector_OperationTypes(t *testing.T) {
	collector := NewMetricsCollector()

	collector.RecordSearchOperation()
	collector.RecordSearchOperation()
	collector.RecordIndexOperation()
	collector.RecordSchemaOperation()

	metrics := collector.GetMetrics()

	if metrics.SearchOperations != 2 {
		t.Errorf("Expected SearchOperations=2, got %d", metrics.SearchOperations)
	}

	if metrics.IndexOperations != 1 {
		t.Errorf("Expected IndexOperations=1, got %d", metrics.IndexOperations)
	}

	if metrics.SchemaOperations != 1 {
		t.Errorf("Expected SchemaOperations=1, got %d", metrics.SchemaOperations)
	}
}

func TestLogger_LogLevels(t *testing.T) {
	// Test different log levels
	debugLogger := NewLogger(LogLevelDebug)
	infoLogger := NewLogger(LogLevelInfo)
	warnLogger := NewLogger(LogLevelWarn)
	errorLogger := NewLogger(LogLevelError)

	// These should not panic
	debugLogger.Debug("Debug message")
	debugLogger.Info("Info message")
	debugLogger.Warn("Warn message")
	debugLogger.Error("Error message")

	infoLogger.Debug("Debug message") // Should not log
	infoLogger.Info("Info message")
	infoLogger.Warn("Warn message")
	infoLogger.Error("Error message")

	warnLogger.Debug("Debug message") // Should not log
	warnLogger.Info("Info message")   // Should not log
	warnLogger.Warn("Warn message")
	warnLogger.Error("Error message")

	errorLogger.Debug("Debug message") // Should not log
	errorLogger.Info("Info message")   // Should not log
	errorLogger.Warn("Warn message")   // Should not log
	errorLogger.Error("Error message")
}

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LogLevelDebug, "DEBUG"},
		{LogLevelInfo, "INFO"},
		{LogLevelWarn, "WARN"},
		{LogLevelError, "ERROR"},
		{LogLevel(999), "UNKNOWN"},
	}

	for _, test := range tests {
		result := test.level.String()
		if result != test.expected {
			t.Errorf("LogLevel(%d).String() = %s, want %s", test.level, result, test.expected)
		}
	}
}

func TestMetricsCircuitBreakerCallback(t *testing.T) {
	collector := NewMetricsCollector()
	logger := NewLogger(LogLevelInfo)
	callback := NewMetricsCircuitBreakerCallback(collector, logger)

	// Test state change to OPEN
	callback.OnStateChange(CircuitBreakerClosed, CircuitBreakerOpen, "too many failures")

	metrics := collector.GetMetrics()
	if metrics.CircuitBreakerOpens != 1 {
		t.Errorf("Expected CircuitBreakerOpens=1, got %d", metrics.CircuitBreakerOpens)
	}

	// Test state change to CLOSED
	callback.OnStateChange(CircuitBreakerOpen, CircuitBreakerClosed, "recovery successful")

	metrics = collector.GetMetrics()
	if metrics.CircuitBreakerCloses != 1 {
		t.Errorf("Expected CircuitBreakerCloses=1, got %d", metrics.CircuitBreakerCloses)
	}

	// Test state change to HALF-OPEN (should not increment counters)
	callback.OnStateChange(CircuitBreakerClosed, CircuitBreakerHalfOpen, "testing recovery")

	metrics = collector.GetMetrics()
	if metrics.CircuitBreakerOpens != 1 {
		t.Errorf("Expected CircuitBreakerOpens to remain 1, got %d", metrics.CircuitBreakerOpens)
	}
	if metrics.CircuitBreakerCloses != 1 {
		t.Errorf("Expected CircuitBreakerCloses to remain 1, got %d", metrics.CircuitBreakerCloses)
	}
}

func TestCalculatePercentiles(t *testing.T) {
	// Test with empty slice
	result := calculatePercentiles([]time.Duration{})
	if result.P50 != 0 || result.P95 != 0 || result.P99 != 0 {
		t.Errorf("Expected all percentiles to be 0 for empty slice")
	}

	// Test with single value
	times := []time.Duration{100 * time.Millisecond}
	result = calculatePercentiles(times)
	if result.P50 != 100*time.Millisecond {
		t.Errorf("Expected P50=100ms, got %v", result.P50)
	}

	// Test with multiple values
	times = []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}
	result = calculatePercentiles(times)

	// P50 should be around 60ms (50th percentile of 10 values is index 5, which is 60ms)
	if result.P50 != 60*time.Millisecond {
		t.Errorf("Expected P50=60ms, got %v", result.P50)
	}

	// P95 should be around 95ms
	if result.P95 != 100*time.Millisecond {
		t.Errorf("Expected P95=100ms, got %v", result.P95)
	}

	// P99 should be around 99ms
	if result.P99 != 100*time.Millisecond {
		t.Errorf("Expected P99=100ms, got %v", result.P99)
	}
}

func TestPeriodicMetricsLogger(t *testing.T) {
	collector := NewMetricsCollector()
	logger := NewPeriodicMetricsLogger(collector, 100*time.Millisecond)

	// Start logging
	logger.Start()

	// Add some metrics
	collector.RecordRequest("test", 50*time.Millisecond, true, "")

	// Wait a bit for logging to occur
	time.Sleep(150 * time.Millisecond)

	// Stop logging
	logger.Stop()

	// Test that we can stop multiple times without panic
	logger.Stop()
}
