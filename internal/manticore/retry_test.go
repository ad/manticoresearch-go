package manticore

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryManager_Execute(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 3
	config.BaseDelay = 10 * time.Millisecond // Short delay for testing

	retryManager := NewRetryManager(config)

	t.Run("successful operation", func(t *testing.T) {
		attempts := 0
		operation := func(ctx context.Context, retryCtx *RetryContext) error {
			attempts++
			return nil // Success on first attempt
		}

		err := retryManager.Execute(context.Background(), "/test", "GET", operation)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("operation succeeds after retries", func(t *testing.T) {
		attempts := 0
		operation := func(ctx context.Context, retryCtx *RetryContext) error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary failure")
			}
			return nil // Success on third attempt
		}

		err := retryManager.Execute(context.Background(), "/test", "GET", operation)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("operation fails with non-retryable error", func(t *testing.T) {
		attempts := 0
		operation := func(ctx context.Context, retryCtx *RetryContext) error {
			attempts++
			return errors.New("unauthorized access") // Non-retryable
		}

		err := retryManager.Execute(context.Background(), "/test", "GET", operation)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt for non-retryable error, got %d", attempts)
		}
	})

	t.Run("operation exhausts max attempts", func(t *testing.T) {
		attempts := 0
		operation := func(ctx context.Context, retryCtx *RetryContext) error {
			attempts++
			return errors.New("connection refused") // Retryable error
		}

		err := retryManager.Execute(context.Background(), "/test", "GET", operation)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if attempts != config.MaxAttempts {
			t.Errorf("Expected %d attempts, got %d", config.MaxAttempts, attempts)
		}

		// Check that it's a retry exhausted error
		if manticoreErr, ok := err.(*ManticoreError); ok {
			if manticoreErr.ErrorType != ErrorTypeRetryExhausted {
				t.Errorf("Expected ErrorTypeRetryExhausted, got %v", manticoreErr.ErrorType)
			}
		} else {
			t.Errorf("Expected ManticoreError, got %T", err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		attempts := 0
		operation := func(ctx context.Context, retryCtx *RetryContext) error {
			attempts++
			if attempts == 1 {
				cancel() // Cancel after first attempt
			}
			return errors.New("connection refused")
		}

		err := retryManager.Execute(ctx, "/test", "GET", operation)
		if err == nil {
			t.Error("Expected error due to context cancellation, got nil")
		}

		// Should stop retrying after context cancellation
		if attempts > 2 {
			t.Errorf("Expected at most 2 attempts due to cancellation, got %d", attempts)
		}
	})
}

func TestRetryManager_CalculateBackoffDelay(t *testing.T) {
	config := DefaultRetryConfig()
	config.BaseDelay = 100 * time.Millisecond
	config.MaxDelay = 5 * time.Second

	retryManager := NewRetryManager(config)

	tests := []struct {
		name        string
		err         error
		attempt     int
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{
			name:        "timeout error attempt 1",
			err:         &ManticoreError{ErrorType: ErrorTypeTimeout},
			attempt:     1,
			minExpected: 150 * time.Millisecond, // 100ms * 2.0 * 2^0 = 200ms, but with jitter could be less
			maxExpected: 250 * time.Millisecond,
		},
		{
			name:        "connection refused attempt 2",
			err:         &ManticoreError{ErrorType: ErrorTypeConnectionRefused},
			attempt:     2,
			minExpected: 700 * time.Millisecond, // 100ms * 4.0 * 2^1 = 800ms, but with jitter could be less
			maxExpected: 900 * time.Millisecond,
		},
		{
			name:        "max delay cap",
			err:         &ManticoreError{ErrorType: ErrorTypeConnectionRefused},
			attempt:     10,              // Very high attempt should be capped
			minExpected: 5 * time.Second, // Should be capped at MaxDelay
			maxExpected: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := retryManager.calculateBackoffDelay(tt.err, tt.attempt)

			if delay < tt.minExpected || delay > tt.maxExpected {
				t.Errorf("Delay %v not in expected range [%v, %v]", delay, tt.minExpected, tt.maxExpected)
			}
		})
	}
}

func TestRetryManager_GetErrorMultiplier(t *testing.T) {
	config := DefaultRetryConfig()
	retryManager := NewRetryManager(config)

	tests := []struct {
		name     string
		err      error
		expected float64
	}{
		{
			name:     "timeout error",
			err:      &ManticoreError{ErrorType: ErrorTypeTimeout},
			expected: config.TimeoutMultiplier,
		},
		{
			name:     "connection refused",
			err:      &ManticoreError{ErrorType: ErrorTypeConnectionRefused},
			expected: config.ServiceUnavailableMultiplier,
		},
		{
			name:     "network error",
			err:      &ConnectionError{ErrorType: ErrorTypeNetwork},
			expected: config.ConnectionMultiplier,
		},
		{
			name:     "rate limit",
			err:      &ManticoreError{ErrorType: ErrorTypeRateLimit},
			expected: config.RateLimitMultiplier,
		},
		{
			name:     "unknown error",
			err:      errors.New("some unknown error"),
			expected: 1.0,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := retryManager.getErrorMultiplier(tt.err)
			if result != tt.expected {
				t.Errorf("Expected multiplier %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRetryManager_CalculateJitter(t *testing.T) {
	config := DefaultRetryConfig()
	config.JitterPercent = 0.1 // 10% jitter

	retryManager := NewRetryManager(config)

	delay := 1 * time.Second
	jitter := retryManager.calculateJitter(delay)

	// Jitter should be between 0 and 10% of delay
	maxJitter := time.Duration(float64(delay) * config.JitterPercent)

	if jitter < 0 || jitter > maxJitter {
		t.Errorf("Jitter %v not in expected range [0, %v]", jitter, maxJitter)
	}
}

func TestRetryManager_GetRetryStats(t *testing.T) {
	config := DefaultRetryConfig()
	retryManager := NewRetryManager(config)

	stats := retryManager.GetRetryStats()

	if stats.MaxAttempts != config.MaxAttempts {
		t.Errorf("Expected MaxAttempts %d, got %d", config.MaxAttempts, stats.MaxAttempts)
	}
	if stats.BaseDelay != config.BaseDelay {
		t.Errorf("Expected BaseDelay %v, got %v", config.BaseDelay, stats.BaseDelay)
	}
	if stats.MaxDelay != config.MaxDelay {
		t.Errorf("Expected MaxDelay %v, got %v", config.MaxDelay, stats.MaxDelay)
	}
	if stats.JitterPercent != config.JitterPercent {
		t.Errorf("Expected JitterPercent %v, got %v", config.JitterPercent, stats.JitterPercent)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxAttempts <= 0 {
		t.Error("MaxAttempts should be positive")
	}
	if config.BaseDelay <= 0 {
		t.Error("BaseDelay should be positive")
	}
	if config.MaxDelay <= config.BaseDelay {
		t.Error("MaxDelay should be greater than BaseDelay")
	}
	if config.JitterPercent < 0 || config.JitterPercent > 1 {
		t.Error("JitterPercent should be between 0 and 1")
	}
	if config.TimeoutMultiplier <= 0 {
		t.Error("TimeoutMultiplier should be positive")
	}
	if config.ConnectionMultiplier <= 0 {
		t.Error("ConnectionMultiplier should be positive")
	}
	if config.ServiceUnavailableMultiplier <= 0 {
		t.Error("ServiceUnavailableMultiplier should be positive")
	}
	if config.RateLimitMultiplier <= 0 {
		t.Error("RateLimitMultiplier should be positive")
	}
}
