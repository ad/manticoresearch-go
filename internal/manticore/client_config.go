package manticore

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// NewClientFromEnvironment creates a new HTTP client based on environment variables
func NewClientFromEnvironment() (ClientInterface, error) {
	config, err := LoadHTTPConfigFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to load HTTP client configuration: %w", err)
	}

	return NewHTTPClient(*config), nil
}

// LoadHTTPConfigFromEnvironment loads HTTP client configuration from environment variables
func LoadHTTPConfigFromEnvironment() (*HTTPClientConfig, error) {
	// Get host configuration
	host := os.Getenv("MANTICORE_HOST")
	port := os.Getenv("MANTICORE_PORT")

	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "9308"
	}

	// Combine host and port
	fullHost := fmt.Sprintf("%s:%s", host, port)

	config := DefaultHTTPConfig(fullHost) // Parse timeout configuration
	if timeoutStr := os.Getenv("MANTICORE_HTTP_TIMEOUT"); timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_TIMEOUT: %w", err)
		}
		config.Timeout = timeout
	}

	// Parse connection pool configuration
	if maxIdleConnsStr := os.Getenv("MANTICORE_HTTP_MAX_IDLE_CONNS"); maxIdleConnsStr != "" {
		maxIdleConns, err := strconv.Atoi(maxIdleConnsStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_MAX_IDLE_CONNS: %w", err)
		}
		config.MaxIdleConns = maxIdleConns
	}

	if maxIdleConnsPerHostStr := os.Getenv("MANTICORE_HTTP_MAX_IDLE_CONNS_PER_HOST"); maxIdleConnsPerHostStr != "" {
		maxIdleConnsPerHost, err := strconv.Atoi(maxIdleConnsPerHostStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_MAX_IDLE_CONNS_PER_HOST: %w", err)
		}
		config.MaxIdleConnsPerHost = maxIdleConnsPerHost
	}

	if idleConnTimeoutStr := os.Getenv("MANTICORE_HTTP_IDLE_CONN_TIMEOUT"); idleConnTimeoutStr != "" {
		idleConnTimeout, err := time.ParseDuration(idleConnTimeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_IDLE_CONN_TIMEOUT: %w", err)
		}
		config.IdleConnTimeout = idleConnTimeout
	}

	// Parse retry configuration
	if maxAttemptsStr := os.Getenv("MANTICORE_HTTP_RETRY_MAX_ATTEMPTS"); maxAttemptsStr != "" {
		maxAttempts, err := strconv.Atoi(maxAttemptsStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_RETRY_MAX_ATTEMPTS: %w", err)
		}
		config.RetryConfig.MaxAttempts = maxAttempts
	}

	if baseDelayStr := os.Getenv("MANTICORE_HTTP_RETRY_BASE_DELAY"); baseDelayStr != "" {
		baseDelay, err := time.ParseDuration(baseDelayStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_RETRY_BASE_DELAY: %w", err)
		}
		config.RetryConfig.BaseDelay = baseDelay
	}

	if maxDelayStr := os.Getenv("MANTICORE_HTTP_RETRY_MAX_DELAY"); maxDelayStr != "" {
		maxDelay, err := time.ParseDuration(maxDelayStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_RETRY_MAX_DELAY: %w", err)
		}
		config.RetryConfig.MaxDelay = maxDelay
	}

	if jitterPercentStr := os.Getenv("MANTICORE_HTTP_RETRY_JITTER_PERCENT"); jitterPercentStr != "" {
		jitterPercent, err := strconv.ParseFloat(jitterPercentStr, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_RETRY_JITTER_PERCENT: %w", err)
		}
		config.RetryConfig.JitterPercent = jitterPercent
	}

	// Parse circuit breaker configuration
	if failureThresholdStr := os.Getenv("MANTICORE_HTTP_CB_FAILURE_THRESHOLD"); failureThresholdStr != "" {
		failureThreshold, err := strconv.Atoi(failureThresholdStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_CB_FAILURE_THRESHOLD: %w", err)
		}
		config.CircuitBreakerConfig.FailureThreshold = failureThreshold
	}

	if recoveryTimeoutStr := os.Getenv("MANTICORE_HTTP_CB_RECOVERY_TIMEOUT"); recoveryTimeoutStr != "" {
		recoveryTimeout, err := time.ParseDuration(recoveryTimeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_CB_RECOVERY_TIMEOUT: %w", err)
		}
		config.CircuitBreakerConfig.RecoveryTimeout = recoveryTimeout
	}

	if halfOpenMaxCallsStr := os.Getenv("MANTICORE_HTTP_CB_HALF_OPEN_MAX_CALLS"); halfOpenMaxCallsStr != "" {
		halfOpenMaxCalls, err := strconv.Atoi(halfOpenMaxCallsStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_HTTP_CB_HALF_OPEN_MAX_CALLS: %w", err)
		}
		config.CircuitBreakerConfig.HalfOpenMaxCalls = halfOpenMaxCalls
	}

	return config, nil
}

// DefaultHTTPConfig returns default HTTP client configuration
func DefaultHTTPConfig(host string) *HTTPClientConfig {
	baseURL := fmt.Sprintf("http://%s", host)

	return &HTTPClientConfig{
		BaseURL:             baseURL,
		Timeout:             60 * time.Second,
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		RetryConfig: RetryConfig{
			MaxAttempts:   5,
			BaseDelay:     500 * time.Millisecond,
			MaxDelay:      30 * time.Second,
			JitterPercent: 0.1,
		},
		CircuitBreakerConfig: CircuitBreakerConfig{
			FailureThreshold: 5,
			RecoveryTimeout:  30 * time.Second,
			HalfOpenMaxCalls: 3,
		},
		BulkConfig: DefaultBulkConfig(),
	}
}
