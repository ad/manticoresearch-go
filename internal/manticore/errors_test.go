package manticore

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestErrorClassifier_ClassifyError(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name         string
		err          error
		endpoint     string
		method       string
		expectedType ErrorType
		retryable    bool
	}{
		{
			name:         "connection refused",
			err:          errors.New("connection refused"),
			endpoint:     "/test",
			method:       "GET",
			expectedType: ErrorTypeConnectionRefused,
			retryable:    true,
		},
		{
			name:         "timeout error",
			err:          errors.New("context deadline exceeded"),
			endpoint:     "/test",
			method:       "POST",
			expectedType: ErrorTypeTimeout,
			retryable:    true,
		},
		{
			name:         "authentication error",
			err:          errors.New("unauthorized access"),
			endpoint:     "/test",
			method:       "GET",
			expectedType: ErrorTypeAuthentication,
			retryable:    false,
		},
		{
			name:         "validation error",
			err:          errors.New("invalid json format"),
			endpoint:     "/test",
			method:       "POST",
			expectedType: ErrorTypeValidation,
			retryable:    false,
		},
		{
			name:         "network error",
			err:          &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("network unreachable")},
			endpoint:     "/test",
			method:       "GET",
			expectedType: ErrorTypeNetwork,
			retryable:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyError(tt.err, tt.endpoint, tt.method)

			// Check if it's a ManticoreError or ConnectionError
			var errorType ErrorType
			var retryable bool

			if manticoreErr, ok := result.(*ManticoreError); ok {
				errorType = manticoreErr.ErrorType
				retryable = manticoreErr.Retryable
			} else if connErr, ok := result.(*ConnectionError); ok {
				errorType = connErr.ErrorType
				retryable = connErr.Retryable
			} else {
				t.Fatalf("Expected ManticoreError or ConnectionError, got %T", result)
			}

			if errorType != tt.expectedType {
				t.Errorf("Expected error type %v, got %v", tt.expectedType, errorType)
			}

			if retryable != tt.retryable {
				t.Errorf("Expected retryable %v, got %v", tt.retryable, retryable)
			}
		})
	}
}

func TestErrorClassifier_IsHTTPStatusRetryable(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		statusCode int
		retryable  bool
	}{
		{200, false}, // Success
		{400, false}, // Bad Request
		{401, false}, // Unauthorized
		{403, false}, // Forbidden
		{404, false}, // Not Found
		{408, true},  // Request Timeout
		{429, true},  // Too Many Requests
		{500, true},  // Internal Server Error
		{502, true},  // Bad Gateway
		{503, true},  // Service Unavailable
		{504, true},  // Gateway Timeout
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
			result := classifier.isHTTPStatusRetryable(tt.statusCode)
			if result != tt.retryable {
				t.Errorf("Status code %d: expected retryable %v, got %v", tt.statusCode, tt.retryable, result)
			}
		})
	}
}

func TestErrorClassifier_SanitizeErrorMessage(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "localhost replacement",
			message:  "connection to 127.0.0.1:9308 failed",
			expected: "connection to [localhost]:9308 failed",
		},
		{
			name:     "localhost name replacement",
			message:  "connection to localhost:9308 failed",
			expected: "connection to [localhost]:9308 failed",
		},
		{
			name:     "long message truncation",
			message:  strings.Repeat("a", 600),
			expected: strings.Repeat("a", 500) + "... [truncated]",
		},
		{
			name:     "normal message",
			message:  "normal error message",
			expected: "normal error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.sanitizeErrorMessage(tt.message)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestErrorClassifier_CalculateBackoffDelay(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name        string
		errorType   ErrorType
		attempt     int
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{
			name:        "connection refused",
			errorType:   ErrorTypeConnectionRefused,
			attempt:     1,
			minExpected: 2 * time.Second, // 500ms * 4.0 multiplier * 2^1
			maxExpected: 5 * time.Second,
		},
		{
			name:        "timeout",
			errorType:   ErrorTypeTimeout,
			attempt:     1,
			minExpected: 1 * time.Second, // 500ms * 2.0 multiplier * 2^1
			maxExpected: 3 * time.Second,
		},
		{
			name:        "network error",
			errorType:   ErrorTypeNetwork,
			attempt:     2,
			minExpected: 5 * time.Second, // 500ms * 3.0 multiplier * 2^2
			maxExpected: 8 * time.Second,
		},
		{
			name:        "rate limit",
			errorType:   ErrorTypeRateLimit,
			attempt:     1,
			minExpected: 4 * time.Second, // 500ms * 5.0 multiplier * 2^1
			maxExpected: 7 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.calculateBackoffDelay(tt.errorType, tt.attempt)

			if result < tt.minExpected || result > tt.maxExpected {
				t.Errorf("Backoff delay %v not in expected range [%v, %v]", result, tt.minExpected, tt.maxExpected)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "nil error",
			err:       nil,
			retryable: false,
		},
		{
			name: "ManticoreError retryable",
			err: &ManticoreError{
				StatusCode: 500,
				Message:    "Internal Server Error",
				Retryable:  true,
				ErrorType:  ErrorTypeHTTPServer,
			},
			retryable: true,
		},
		{
			name: "ManticoreError not retryable",
			err: &ManticoreError{
				StatusCode: 400,
				Message:    "Bad Request",
				Retryable:  false,
				ErrorType:  ErrorTypeHTTPClient,
			},
			retryable: false,
		},
		{
			name: "ConnectionError retryable",
			err: &ConnectionError{
				Cause:     errors.New("connection refused"),
				Retryable: true,
				ErrorType: ErrorTypeConnectionRefused,
			},
			retryable: true,
		},
		{
			name: "ConnectionError not retryable",
			err: &ConnectionError{
				Cause:     errors.New("invalid credentials"),
				Retryable: false,
				ErrorType: ErrorTypeAuthentication,
			},
			retryable: false,
		},
		{
			name:      "generic timeout error",
			err:       errors.New("operation timed out"),
			retryable: true,
		},
		{
			name:      "generic validation error",
			err:       errors.New("bad request format"),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			if result != tt.retryable {
				t.Errorf("Expected retryable %v, got %v", tt.retryable, result)
			}
		})
	}
}

func TestGetErrorBackoffDelay(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected time.Duration
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: 0,
		},
		{
			name: "ManticoreError with RetryAfter",
			err: &ManticoreError{
				StatusCode: 429,
				Message:    "Too Many Requests",
				RetryAfter: 10 * time.Second,
				Retryable:  true,
				ErrorType:  ErrorTypeRateLimit,
			},
			expected: 10 * time.Second,
		},
		{
			name: "ConnectionError with BackoffDelay",
			err: &ConnectionError{
				Cause:        errors.New("connection refused"),
				Retryable:    true,
				BackoffDelay: 5 * time.Second,
				ErrorType:    ErrorTypeConnectionRefused,
			},
			expected: 5 * time.Second,
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			expected: 500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetErrorBackoffDelay(tt.err)
			if result != tt.expected {
				t.Errorf("Expected backoff delay %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestManticoreError_Error(t *testing.T) {
	err := &ManticoreError{
		StatusCode: 500,
		Message:    "Internal Server Error",
		Endpoint:   "/search",
		Method:     "POST",
		Retryable:  true,
		ErrorType:  ErrorTypeHTTPServer,
	}

	expected := "Manticore API error [500] POST /search: Internal Server Error"
	result := err.Error()

	if result != expected {
		t.Errorf("Expected error message %q, got %q", expected, result)
	}
}

func TestConnectionError_Error(t *testing.T) {
	err := &ConnectionError{
		Cause:        errors.New("connection refused"),
		Retryable:    true,
		BackoffDelay: 2 * time.Second,
		ErrorType:    ErrorTypeConnectionRefused,
		Endpoint:     "/test",
		Attempt:      1,
	}

	result := err.Error()

	// Check that the error message contains expected components
	if !strings.Contains(result, "Connection error") {
		t.Errorf("Error message should contain 'Connection error', got: %s", result)
	}
	if !strings.Contains(result, "/test") {
		t.Errorf("Error message should contain endpoint '/test', got: %s", result)
	}
	if !strings.Contains(result, "retryable: true") {
		t.Errorf("Error message should contain 'retryable: true', got: %s", result)
	}
	if !strings.Contains(result, "connection refused") {
		t.Errorf("Error message should contain underlying cause, got: %s", result)
	}
}

func TestErrorType_String(t *testing.T) {
	tests := []struct {
		errorType ErrorType
		expected  string
	}{
		{ErrorTypeUnknown, "unknown"},
		{ErrorTypeNetwork, "network"},
		{ErrorTypeTimeout, "timeout"},
		{ErrorTypeConnectionRefused, "connection_refused"},
		{ErrorTypeConnectionReset, "connection_reset"},
		{ErrorTypeDNS, "dns"},
		{ErrorTypeHTTPClient, "http_client"},
		{ErrorTypeHTTPServer, "http_server"},
		{ErrorTypeAuthentication, "authentication"},
		{ErrorTypeRateLimit, "rate_limit"},
		{ErrorTypeValidation, "validation"},
		{ErrorTypeCircuitBreaker, "circuit_breaker"},
		{ErrorTypeRetryExhausted, "retry_exhausted"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.errorType.String()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
