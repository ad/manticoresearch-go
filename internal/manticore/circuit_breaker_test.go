package manticore

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_Execute(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 3
	config.RecoveryTimeout = 100 * time.Millisecond // Short timeout for testing
	config.HalfOpenMaxCalls = 2
	config.SuccessThreshold = 2

	cb := NewCircuitBreaker(config)
	defer cb.Close()

	t.Run("successful operation", func(t *testing.T) {
		attempts := 0
		operation := func(ctx context.Context) error {
			attempts++
			return nil
		}

		err := cb.Execute(context.Background(), operation)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
		if cb.GetState() != CircuitBreakerClosed {
			t.Errorf("Expected CLOSED state, got %v", cb.GetState())
		}
	})

	t.Run("circuit opens after failures", func(t *testing.T) {
		// Reset circuit breaker
		cb.Reset()

		attempts := 0
		operation := func(ctx context.Context) error {
			attempts++
			return errors.New("connection refused")
		}

		// Execute enough failures to open the circuit
		for i := 0; i < config.FailureThreshold; i++ {
			err := cb.Execute(context.Background(), operation)
			if err == nil {
				t.Errorf("Expected error on attempt %d", i+1)
			}
		}

		if cb.GetState() != CircuitBreakerOpen {
			t.Errorf("Expected OPEN state after %d failures, got %v", config.FailureThreshold, cb.GetState())
		}

		// Next request should be rejected immediately
		rejectedAttempts := attempts
		err := cb.Execute(context.Background(), operation)
		if err == nil {
			t.Error("Expected circuit breaker error when open")
		}

		// Should not have executed the operation
		if attempts != rejectedAttempts {
			t.Error("Operation should not have been executed when circuit is open")
		}

		// Check that it's a circuit breaker error
		if manticoreErr, ok := err.(*ManticoreError); ok {
			if manticoreErr.ErrorType != ErrorTypeCircuitBreaker {
				t.Errorf("Expected ErrorTypeCircuitBreaker, got %v", manticoreErr.ErrorType)
			}
		} else {
			t.Errorf("Expected ManticoreError, got %T", err)
		}
	})

	t.Run("circuit transitions to half-open after recovery timeout", func(t *testing.T) {
		// Ensure circuit is open
		cb.ForceOpen()

		// Wait for recovery timeout
		time.Sleep(config.RecoveryTimeout + 10*time.Millisecond)

		attempts := 0
		operation := func(ctx context.Context) error {
			attempts++
			return nil // Success
		}

		err := cb.Execute(context.Background(), operation)
		if err != nil {
			t.Errorf("Expected no error after recovery timeout, got: %v", err)
		}

		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("circuit closes after successful recovery", func(t *testing.T) {
		// Force circuit to half-open state
		cb.Reset()
		cb.ForceOpen()
		time.Sleep(config.RecoveryTimeout + 10*time.Millisecond)

		// Execute successful operations to close the circuit
		operation := func(ctx context.Context) error {
			return nil
		}

		for i := 0; i < config.SuccessThreshold; i++ {
			err := cb.Execute(context.Background(), operation)
			if err != nil {
				t.Errorf("Expected no error on recovery attempt %d, got: %v", i+1, err)
			}
		}

		if cb.GetState() != CircuitBreakerClosed {
			t.Errorf("Expected CLOSED state after %d successes, got %v", config.SuccessThreshold, cb.GetState())
		}
	})

	t.Run("circuit returns to open on failure during half-open", func(t *testing.T) {
		// Force circuit to half-open state
		cb.Reset()
		cb.ForceOpen()
		time.Sleep(config.RecoveryTimeout + 10*time.Millisecond)

		// First request succeeds (transitions to half-open)
		err := cb.Execute(context.Background(), func(ctx context.Context) error {
			return nil
		})
		if err != nil {
			t.Errorf("Expected no error on first half-open request, got: %v", err)
		}

		// Second request fails (should return to open)
		err = cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("connection refused")
		})
		if err == nil {
			t.Error("Expected error on failed half-open request")
		}

		if cb.GetState() != CircuitBreakerOpen {
			t.Errorf("Expected OPEN state after half-open failure, got %v", cb.GetState())
		}
	})
}

func TestCircuitBreaker_GetStats(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2

	cb := NewCircuitBreaker(config)
	defer cb.Close()

	// Initial stats
	stats := cb.GetStats()
	if stats.State != CircuitBreakerClosed {
		t.Errorf("Expected initial state CLOSED, got %v", stats.State)
	}
	if stats.TotalRequests != 0 {
		t.Errorf("Expected 0 total requests initially, got %d", stats.TotalRequests)
	}

	// Execute some operations
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil // Success
	})
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("failure") // Failure
	})

	stats = cb.GetStats()
	if stats.TotalRequests != 2 {
		t.Errorf("Expected 2 total requests, got %d", stats.TotalRequests)
	}
	if stats.TotalSuccesses != 1 {
		t.Errorf("Expected 1 success, got %d", stats.TotalSuccesses)
	}
	if stats.TotalFailures != 1 {
		t.Errorf("Expected 1 failure, got %d", stats.TotalFailures)
	}
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.RecoveryTimeout = 50 * time.Millisecond
	config.SuccessThreshold = 1

	cb := NewCircuitBreaker(config)
	defer cb.Close()

	// Test manual state transitions
	if !cb.IsClosed() {
		t.Error("Circuit breaker should initially be closed")
	}

	cb.ForceOpen()
	if !cb.IsOpen() {
		t.Error("Circuit breaker should be open after ForceOpen()")
	}

	cb.Reset()
	if !cb.IsClosed() {
		t.Error("Circuit breaker should be closed after Reset()")
	}
}

func TestCircuitBreaker_FailureRateCalculation(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 100 // High threshold to test failure rate
	config.MinRequestThreshold = 5
	config.FailureRateThreshold = 0.6 // 60% failure rate
	config.SlidingWindowSize = 10

	cb := NewCircuitBreaker(config)
	defer cb.Close()

	// Execute operations with 70% failure rate (7 failures out of 10)
	for i := 0; i < 10; i++ {
		var err error
		if i < 7 {
			err = errors.New("failure")
		}

		cb.Execute(context.Background(), func(ctx context.Context) error {
			return err
		})
	}

	// Circuit should be open due to high failure rate
	if cb.GetState() != CircuitBreakerOpen {
		t.Errorf("Expected OPEN state due to high failure rate, got %v", cb.GetState())
	}

	stats := cb.GetStats()
	if stats.CurrentFailureRate < config.FailureRateThreshold {
		t.Errorf("Expected failure rate >= %v, got %v", config.FailureRateThreshold, stats.CurrentFailureRate)
	}
}

func TestCircuitBreakerWithRetry_Execute(t *testing.T) {
	cbConfig := DefaultCircuitBreakerConfig()
	cbConfig.FailureThreshold = 3
	cbConfig.RecoveryTimeout = 100 * time.Millisecond

	retryConfig := DefaultRetryConfig()
	retryConfig.MaxAttempts = 2
	retryConfig.BaseDelay = 10 * time.Millisecond

	cbr := NewCircuitBreakerWithRetry(cbConfig, retryConfig)
	defer cbr.Close()

	t.Run("successful operation", func(t *testing.T) {
		attempts := 0
		operation := func(ctx context.Context) error {
			attempts++
			return nil
		}

		err := cbr.Execute(context.Background(), "/test", "GET", operation)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("operation with retries", func(t *testing.T) {
		attempts := 0
		operation := func(ctx context.Context) error {
			attempts++
			if attempts == 1 {
				return errors.New("temporary failure")
			}
			return nil
		}

		err := cbr.Execute(context.Background(), "/test", "GET", operation)
		if err != nil {
			t.Errorf("Expected no error after retry, got: %v", err)
		}
		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}
	})

	t.Run("circuit breaker integration", func(t *testing.T) {
		// Force circuit breaker open
		stats := cbr.GetCircuitBreakerStats()
		if stats.State != CircuitBreakerClosed {
			t.Errorf("Expected initial CLOSED state, got %v", stats.State)
		}

		// Execute enough failures to open circuit
		for i := 0; i < cbConfig.FailureThreshold; i++ {
			cbr.Execute(context.Background(), "/test", "GET", func(ctx context.Context) error {
				return errors.New("connection refused")
			})
		}

		stats = cbr.GetCircuitBreakerStats()
		if stats.State != CircuitBreakerOpen {
			t.Errorf("Expected OPEN state after failures, got %v", stats.State)
		}

		// Next request should fail immediately due to circuit breaker
		attempts := 0
		err := cbr.Execute(context.Background(), "/test", "GET", func(ctx context.Context) error {
			attempts++
			return nil
		})

		if err == nil {
			t.Error("Expected circuit breaker error")
		}
		if attempts > 0 {
			t.Error("Operation should not have been executed when circuit is open")
		}
	})
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	if config.FailureThreshold <= 0 {
		t.Error("FailureThreshold should be positive")
	}
	if config.RecoveryTimeout <= 0 {
		t.Error("RecoveryTimeout should be positive")
	}
	if config.HalfOpenMaxCalls <= 0 {
		t.Error("HalfOpenMaxCalls should be positive")
	}
	if config.SuccessThreshold <= 0 {
		t.Error("SuccessThreshold should be positive")
	}
	if config.MinRequestThreshold <= 0 {
		t.Error("MinRequestThreshold should be positive")
	}
	if config.FailureRateThreshold <= 0 || config.FailureRateThreshold >= 1 {
		t.Error("FailureRateThreshold should be between 0 and 1")
	}
	if config.SlidingWindowSize <= 0 {
		t.Error("SlidingWindowSize should be positive")
	}
	if config.MonitoringInterval <= 0 {
		t.Error("MonitoringInterval should be positive")
	}
}

func TestCircuitBreakerState_String(t *testing.T) {
	tests := []struct {
		state    CircuitBreakerState
		expected string
	}{
		{CircuitBreakerClosed, "CLOSED"},
		{CircuitBreakerOpen, "OPEN"},
		{CircuitBreakerHalfOpen, "HALF-OPEN"},
		{CircuitBreakerState(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
