package manticore

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"
)

// RetryManager handles retry logic with exponential backoff and jitter
type RetryManager struct {
	config          RetryConfig
	errorClassifier *ErrorClassifier
}

// RetryConfig defines retry behavior with enhanced options
type RetryConfig struct {
	MaxAttempts   int           `json:"max_attempts"`
	BaseDelay     time.Duration `json:"base_delay"`
	MaxDelay      time.Duration `json:"max_delay"`
	JitterPercent float64       `json:"jitter_percent"`

	// Error-specific backoff multipliers
	TimeoutMultiplier            float64 `json:"timeout_multiplier"`
	ConnectionMultiplier         float64 `json:"connection_multiplier"`
	ServiceUnavailableMultiplier float64 `json:"service_unavailable_multiplier"`
	RateLimitMultiplier          float64 `json:"rate_limit_multiplier"`

	// Timeout handling
	PerAttemptTimeout time.Duration `json:"per_attempt_timeout"`
	TotalTimeout      time.Duration `json:"total_timeout"`
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:                  5,
		BaseDelay:                    500 * time.Millisecond,
		MaxDelay:                     30 * time.Second,
		JitterPercent:                0.1,
		TimeoutMultiplier:            2.0,
		ConnectionMultiplier:         3.0,
		ServiceUnavailableMultiplier: 4.0,
		RateLimitMultiplier:          5.0,
		PerAttemptTimeout:            30 * time.Second,
		TotalTimeout:                 5 * time.Minute,
	}
}

// NewRetryManager creates a new retry manager with enhanced capabilities
func NewRetryManager(config RetryConfig) *RetryManager {
	return &RetryManager{
		config:          config,
		errorClassifier: NewErrorClassifier(),
	}
}

// RetryContext holds context information for retry operations
type RetryContext struct {
	Attempt       int           `json:"attempt"`
	TotalDuration time.Duration `json:"total_duration"`
	LastError     error         `json:"last_error"`
	StartTime     time.Time     `json:"start_time"`
	Endpoint      string        `json:"endpoint"`
	Method        string        `json:"method"`
}

// RetryableOperation represents an operation that can be retried
type RetryableOperation func(ctx context.Context, retryCtx *RetryContext) error

// Execute performs the operation with retry logic and exponential backoff
func (rm *RetryManager) Execute(ctx context.Context, endpoint, method string, operation RetryableOperation) error {
	retryCtx := &RetryContext{
		Attempt:   0,
		StartTime: time.Now(),
		Endpoint:  endpoint,
		Method:    method,
	}

	// Create context with total timeout if specified
	var operationCtx context.Context
	var cancel context.CancelFunc

	if rm.config.TotalTimeout > 0 {
		operationCtx, cancel = context.WithTimeout(ctx, rm.config.TotalTimeout)
		defer cancel()
	} else {
		operationCtx = ctx
	}

	for retryCtx.Attempt < rm.config.MaxAttempts {
		retryCtx.Attempt++
		retryCtx.TotalDuration = time.Since(retryCtx.StartTime)

		// Check if total timeout exceeded
		if rm.config.TotalTimeout > 0 && retryCtx.TotalDuration >= rm.config.TotalTimeout {
			return &ManticoreError{
				StatusCode: 0,
				Message:    fmt.Sprintf("total timeout exceeded after %v", retryCtx.TotalDuration),
				Endpoint:   endpoint,
				Method:     method,
				Retryable:  false,
				ErrorType:  ErrorTypeTimeout,
			}
		}

		// Create per-attempt context with timeout
		var attemptCtx context.Context
		var attemptCancel context.CancelFunc

		if rm.config.PerAttemptTimeout > 0 {
			attemptCtx, attemptCancel = context.WithTimeout(operationCtx, rm.config.PerAttemptTimeout)
		} else {
			attemptCtx = operationCtx
			attemptCancel = func() {} // No-op cancel function
		}

		// Execute the operation
		err := operation(attemptCtx, retryCtx)
		attemptCancel()

		if err == nil {
			// Success
			if retryCtx.Attempt > 1 {
				log.Printf("Operation succeeded after %d attempts (total duration: %v) for %s %s",
					retryCtx.Attempt, retryCtx.TotalDuration, method, endpoint)
			}
			return nil
		}

		retryCtx.LastError = err

		// Classify the error
		classifiedErr := rm.errorClassifier.ClassifyError(err, endpoint, method)

		// Check if error is retryable
		if !IsRetryableError(classifiedErr) {
			log.Printf("Non-retryable error on attempt %d for %s %s: %v",
				retryCtx.Attempt, method, endpoint, classifiedErr)
			return classifiedErr
		}

		// Check if we've exhausted all attempts
		if retryCtx.Attempt >= rm.config.MaxAttempts {
			log.Printf("Max attempts (%d) exceeded for %s %s, last error: %v",
				rm.config.MaxAttempts, method, endpoint, classifiedErr)

			return &ManticoreError{
				StatusCode: 0,
				Message:    fmt.Sprintf("max retry attempts (%d) exceeded, last error: %v", rm.config.MaxAttempts, classifiedErr),
				Endpoint:   endpoint,
				Method:     method,
				Retryable:  false,
				ErrorType:  ErrorTypeRetryExhausted,
			}
		}

		// Calculate backoff delay
		delay := rm.calculateBackoffDelay(classifiedErr, retryCtx.Attempt)

		log.Printf("Retrying operation (attempt %d/%d) after %v delay for %s %s due to error: %v",
			retryCtx.Attempt+1, rm.config.MaxAttempts, delay, method, endpoint, classifiedErr)

		// Wait for backoff delay (respecting context cancellation)
		select {
		case <-operationCtx.Done():
			return operationCtx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	// This should not be reached, but just in case
	return &ManticoreError{
		StatusCode: 0,
		Message:    fmt.Sprintf("unexpected retry loop exit after %d attempts", retryCtx.Attempt),
		Endpoint:   endpoint,
		Method:     method,
		Retryable:  false,
		ErrorType:  ErrorTypeRetryExhausted,
	}
}

// calculateBackoffDelay calculates the delay for the next retry attempt
func (rm *RetryManager) calculateBackoffDelay(err error, attempt int) time.Duration {
	// Start with base exponential backoff: baseDelay * (2^(attempt-1))
	exponentialDelay := rm.config.BaseDelay * time.Duration(1<<(attempt-1))

	// Apply error-specific multipliers
	multiplier := rm.getErrorMultiplier(err)
	adjustedDelay := time.Duration(float64(exponentialDelay) * multiplier)

	// Apply jitter to prevent thundering herd
	jitter := rm.calculateJitter(adjustedDelay)
	finalDelay := adjustedDelay + jitter

	// Respect maximum delay
	if finalDelay > rm.config.MaxDelay {
		finalDelay = rm.config.MaxDelay
	}

	// Ensure minimum delay
	if finalDelay < rm.config.BaseDelay {
		finalDelay = rm.config.BaseDelay
	}

	return finalDelay
}

// getErrorMultiplier returns the backoff multiplier for specific error types
func (rm *RetryManager) getErrorMultiplier(err error) float64 {
	if err == nil {
		return 1.0
	}

	// Check for our custom error types first
	if manticoreErr, ok := err.(*ManticoreError); ok {
		switch manticoreErr.ErrorType {
		case ErrorTypeTimeout:
			return rm.config.TimeoutMultiplier
		case ErrorTypeConnectionRefused:
			return rm.config.ServiceUnavailableMultiplier
		case ErrorTypeConnectionReset, ErrorTypeNetwork:
			return rm.config.ConnectionMultiplier
		case ErrorTypeRateLimit:
			return rm.config.RateLimitMultiplier
		default:
			return 1.0
		}
	}

	if connErr, ok := err.(*ConnectionError); ok {
		switch connErr.ErrorType {
		case ErrorTypeTimeout:
			return rm.config.TimeoutMultiplier
		case ErrorTypeConnectionRefused:
			return rm.config.ServiceUnavailableMultiplier
		case ErrorTypeConnectionReset, ErrorTypeNetwork:
			return rm.config.ConnectionMultiplier
		default:
			return 1.0
		}
	}

	// Fallback to basic error string analysis
	errorStr := err.Error()
	switch {
	case containsAny(errorStr, []string{"timeout", "deadline exceeded"}):
		return rm.config.TimeoutMultiplier
	case containsAny(errorStr, []string{"connection refused", "service unavailable"}):
		return rm.config.ServiceUnavailableMultiplier
	case containsAny(errorStr, []string{"connection reset", "broken pipe", "network"}):
		return rm.config.ConnectionMultiplier
	case containsAny(errorStr, []string{"rate limit", "too many requests"}):
		return rm.config.RateLimitMultiplier
	default:
		return 1.0
	}
}

// calculateJitter adds randomness to prevent thundering herd
func (rm *RetryManager) calculateJitter(delay time.Duration) time.Duration {
	if rm.config.JitterPercent <= 0 {
		return 0
	}

	// Calculate jitter as a percentage of the delay
	maxJitter := time.Duration(float64(delay) * rm.config.JitterPercent)

	// Generate random jitter between 0 and maxJitter
	if maxJitter > 0 {
		return time.Duration(rand.Int63n(int64(maxJitter)))
	}

	return 0
}

// ExecuteWithCustomBackoff allows custom backoff calculation
func (rm *RetryManager) ExecuteWithCustomBackoff(
	ctx context.Context,
	endpoint, method string,
	operation RetryableOperation,
	backoffCalculator func(attempt int, lastError error) time.Duration,
) error {
	retryCtx := &RetryContext{
		Attempt:   0,
		StartTime: time.Now(),
		Endpoint:  endpoint,
		Method:    method,
	}

	// Create context with total timeout if specified
	var operationCtx context.Context
	var cancel context.CancelFunc

	if rm.config.TotalTimeout > 0 {
		operationCtx, cancel = context.WithTimeout(ctx, rm.config.TotalTimeout)
		defer cancel()
	} else {
		operationCtx = ctx
	}

	for retryCtx.Attempt < rm.config.MaxAttempts {
		retryCtx.Attempt++
		retryCtx.TotalDuration = time.Since(retryCtx.StartTime)

		// Create per-attempt context with timeout
		var attemptCtx context.Context
		var attemptCancel context.CancelFunc

		if rm.config.PerAttemptTimeout > 0 {
			attemptCtx, attemptCancel = context.WithTimeout(operationCtx, rm.config.PerAttemptTimeout)
		} else {
			attemptCtx = operationCtx
			attemptCancel = func() {}
		}

		// Execute the operation
		err := operation(attemptCtx, retryCtx)
		attemptCancel()

		if err == nil {
			return nil
		}

		retryCtx.LastError = err

		// Classify the error
		classifiedErr := rm.errorClassifier.ClassifyError(err, endpoint, method)

		// Check if error is retryable
		if !IsRetryableError(classifiedErr) {
			return classifiedErr
		}

		// Check if we've exhausted all attempts
		if retryCtx.Attempt >= rm.config.MaxAttempts {
			return &ManticoreError{
				StatusCode: 0,
				Message:    fmt.Sprintf("max retry attempts (%d) exceeded", rm.config.MaxAttempts),
				Endpoint:   endpoint,
				Method:     method,
				Retryable:  false,
				ErrorType:  ErrorTypeRetryExhausted,
			}
		}

		// Calculate custom backoff delay
		delay := backoffCalculator(retryCtx.Attempt, classifiedErr)

		log.Printf("Retrying operation (attempt %d/%d) after custom %v delay for %s %s",
			retryCtx.Attempt+1, rm.config.MaxAttempts, delay, method, endpoint)

		// Wait for backoff delay
		select {
		case <-operationCtx.Done():
			return operationCtx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return &ManticoreError{
		StatusCode: 0,
		Message:    fmt.Sprintf("unexpected retry loop exit after %d attempts", retryCtx.Attempt),
		Endpoint:   endpoint,
		Method:     method,
		Retryable:  false,
		ErrorType:  ErrorTypeRetryExhausted,
	}
}

// GetRetryStats returns statistics about retry behavior
func (rm *RetryManager) GetRetryStats() RetryStats {
	return RetryStats{
		MaxAttempts:   rm.config.MaxAttempts,
		BaseDelay:     rm.config.BaseDelay,
		MaxDelay:      rm.config.MaxDelay,
		JitterPercent: rm.config.JitterPercent,
	}
}

// RetryStats provides information about retry configuration
type RetryStats struct {
	MaxAttempts   int           `json:"max_attempts"`
	BaseDelay     time.Duration `json:"base_delay"`
	MaxDelay      time.Duration `json:"max_delay"`
	JitterPercent float64       `json:"jitter_percent"`
}

// containsAny checks if a string contains any of the given substrings
func containsAny(s string, substrings []string) bool {
	for _, substring := range substrings {
		if len(substring) > 0 && len(s) >= len(substring) {
			for i := 0; i <= len(s)-len(substring); i++ {
				if s[i:i+len(substring)] == substring {
					return true
				}
			}
		}
	}
	return false
}

// RetryableHTTPOperation is a convenience wrapper for HTTP operations
func (rm *RetryManager) RetryableHTTPOperation(
	ctx context.Context,
	endpoint, method string,
	httpOperation func(ctx context.Context) error,
) error {
	operation := func(ctx context.Context, retryCtx *RetryContext) error {
		return httpOperation(ctx)
	}

	return rm.Execute(ctx, endpoint, method, operation)
}
