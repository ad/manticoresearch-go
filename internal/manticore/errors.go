package manticore

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// ManticoreError represents an error from Manticore API with enhanced details
type ManticoreError struct {
	StatusCode int           `json:"status_code"`
	Message    string        `json:"message"`
	Endpoint   string        `json:"endpoint"`
	Method     string        `json:"method"`
	RetryAfter time.Duration `json:"retry_after"`
	Retryable  bool          `json:"retryable"`
	ErrorType  ErrorType     `json:"error_type"`
	RawBody    string        `json:"raw_body,omitempty"`
}

// Error implements the error interface
func (e *ManticoreError) Error() string {
	return fmt.Sprintf("Manticore API error [%d] %s %s: %s",
		e.StatusCode, e.Method, e.Endpoint, e.Message)
}

// IsRetryable returns whether this error should be retried
func (e *ManticoreError) IsRetryable() bool {
	return e.Retryable
}

// GetRetryAfter returns the suggested retry delay
func (e *ManticoreError) GetRetryAfter() time.Duration {
	return e.RetryAfter
}

// ConnectionError represents a connection-related error with enhanced classification
type ConnectionError struct {
	Cause        error         `json:"cause"`
	Retryable    bool          `json:"retryable"`
	BackoffDelay time.Duration `json:"backoff_delay"`
	ErrorType    ErrorType     `json:"error_type"`
	Endpoint     string        `json:"endpoint"`
	Attempt      int           `json:"attempt"`
}

// Error implements the error interface
func (e *ConnectionError) Error() string {
	return fmt.Sprintf("Connection error to %s (attempt %d, retryable: %v, backoff: %v): %v",
		e.Endpoint, e.Attempt, e.Retryable, e.BackoffDelay, e.Cause)
}

// IsRetryable returns whether this error should be retried
func (e *ConnectionError) IsRetryable() bool {
	return e.Retryable
}

// GetBackoffDelay returns the suggested backoff delay
func (e *ConnectionError) GetBackoffDelay() time.Duration {
	return e.BackoffDelay
}

// Unwrap returns the underlying error for error unwrapping
func (e *ConnectionError) Unwrap() error {
	return e.Cause
}

// ErrorType represents different categories of errors
type ErrorType int

const (
	ErrorTypeUnknown ErrorType = iota
	ErrorTypeNetwork
	ErrorTypeTimeout
	ErrorTypeConnectionRefused
	ErrorTypeConnectionReset
	ErrorTypeDNS
	ErrorTypeHTTPClient
	ErrorTypeHTTPServer
	ErrorTypeAuthentication
	ErrorTypeRateLimit
	ErrorTypeValidation
	ErrorTypeCircuitBreaker
	ErrorTypeRetryExhausted
)

// String returns the string representation of ErrorType
func (et ErrorType) String() string {
	switch et {
	case ErrorTypeNetwork:
		return "network"
	case ErrorTypeTimeout:
		return "timeout"
	case ErrorTypeConnectionRefused:
		return "connection_refused"
	case ErrorTypeConnectionReset:
		return "connection_reset"
	case ErrorTypeDNS:
		return "dns"
	case ErrorTypeHTTPClient:
		return "http_client"
	case ErrorTypeHTTPServer:
		return "http_server"
	case ErrorTypeAuthentication:
		return "authentication"
	case ErrorTypeRateLimit:
		return "rate_limit"
	case ErrorTypeValidation:
		return "validation"
	case ErrorTypeCircuitBreaker:
		return "circuit_breaker"
	case ErrorTypeRetryExhausted:
		return "retry_exhausted"
	default:
		return "unknown"
	}
}

// ErrorClassifier provides methods to classify and handle different types of errors
type ErrorClassifier struct{}

// NewErrorClassifier creates a new error classifier
func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{}
}

// ClassifyError analyzes an error and returns its classification
func (ec *ErrorClassifier) ClassifyError(err error, endpoint, method string) error {
	if err == nil {
		return nil
	}

	// Check if it's already a classified error
	if manticoreErr, ok := err.(*ManticoreError); ok {
		return manticoreErr
	}
	if connErr, ok := err.(*ConnectionError); ok {
		return connErr
	}

	// Classify based on error type and content
	errorType, retryable := ec.classifyErrorType(err)

	// Note: HTTP response errors should be handled by the caller before classification

	// Handle network errors
	if ec.isNetworkError(err) {
		backoffDelay := ec.calculateBackoffDelay(errorType, 1)
		return &ConnectionError{
			Cause:        err,
			Retryable:    retryable,
			BackoffDelay: backoffDelay,
			ErrorType:    errorType,
			Endpoint:     endpoint,
			Attempt:      1,
		}
	}

	// Default to ManticoreError for other cases
	return &ManticoreError{
		StatusCode: 0,
		Message:    ec.sanitizeErrorMessage(err.Error()),
		Endpoint:   endpoint,
		Method:     method,
		Retryable:  retryable,
		ErrorType:  errorType,
	}
}

// classifyErrorType determines the error type and retryability
func (ec *ErrorClassifier) classifyErrorType(err error) (ErrorType, bool) {
	if err == nil {
		return ErrorTypeUnknown, false
	}

	errStr := strings.ToLower(err.Error())

	// Network-related errors (typically retryable)
	networkErrors := map[string]ErrorType{
		"connection reset by peer":         ErrorTypeConnectionReset,
		"connection refused":               ErrorTypeConnectionRefused,
		"broken pipe":                      ErrorTypeConnectionReset,
		"use of closed network connection": ErrorTypeConnectionReset,
		"write: broken pipe":               ErrorTypeConnectionReset,
		"read: connection reset":           ErrorTypeConnectionReset,
		"server closed idle connection":    ErrorTypeConnectionReset,
		"connection lost":                  ErrorTypeConnectionReset,
		"readfrom tcp":                     ErrorTypeNetwork,
		"write tcp":                        ErrorTypeNetwork,
		"dial tcp":                         ErrorTypeNetwork,
		"no route to host":                 ErrorTypeNetwork,
		"host is down":                     ErrorTypeNetwork,
		"network is unreachable":           ErrorTypeNetwork,
	}

	for errorPattern, errorType := range networkErrors {
		if strings.Contains(errStr, errorPattern) {
			return errorType, true // Network errors are retryable
		}
	}

	// Timeout errors (retryable)
	timeoutErrors := []string{
		"timeout",
		"i/o timeout",
		"context deadline exceeded",
		"operation timed out",
		"temporary failure",
	}

	for _, timeoutError := range timeoutErrors {
		if strings.Contains(errStr, timeoutError) {
			return ErrorTypeTimeout, true
		}
	}

	// DNS errors (retryable)
	dnsErrors := []string{
		"no such host",
		"dns",
		"name resolution",
	}

	for _, dnsError := range dnsErrors {
		if strings.Contains(errStr, dnsError) {
			return ErrorTypeDNS, true
		}
	}

	// Authentication errors (not retryable)
	authErrors := []string{
		"unauthorized",
		"authentication",
		"invalid credentials",
		"access denied",
	}

	for _, authError := range authErrors {
		if strings.Contains(errStr, authError) {
			return ErrorTypeAuthentication, false
		}
	}

	// Validation errors (not retryable)
	validationErrors := []string{
		"invalid json",
		"malformed",
		"bad request",
		"validation failed",
	}

	for _, validationError := range validationErrors {
		if strings.Contains(errStr, validationError) {
			return ErrorTypeValidation, false
		}
	}

	// Default to unknown, not retryable
	return ErrorTypeUnknown, false
}

// isNetworkError checks if an error is network-related
func (ec *ErrorClassifier) isNetworkError(err error) bool {
	// Check for net.Error interface
	if netErr, ok := err.(net.Error); ok {
		return netErr.Temporary() || netErr.Timeout()
	}

	// Check for specific network error types
	switch err.(type) {
	case *net.OpError, *net.DNSError, *net.AddrError:
		return true
	}

	return false
}

// isHTTPStatusRetryable determines if an HTTP status code is retryable
func (ec *ErrorClassifier) isHTTPStatusRetryable(statusCode int) bool {
	switch {
	case statusCode >= 500: // 5xx server errors are retryable
		return true
	case statusCode == 429: // Rate limit is retryable
		return true
	case statusCode == 408: // Request timeout is retryable
		return true
	case statusCode >= 400 && statusCode < 500: // Other 4xx errors are not retryable
		return false
	default:
		return false
	}
}

// calculateBackoffDelay calculates backoff delay based on error type and attempt
func (ec *ErrorClassifier) calculateBackoffDelay(errorType ErrorType, attempt int) time.Duration {
	baseDelay := 500 * time.Millisecond

	// Apply multipliers based on error type
	multiplier := 1.0
	switch errorType {
	case ErrorTypeConnectionRefused:
		multiplier = 4.0 // Aggressive backoff for service unavailable
	case ErrorTypeConnectionReset, ErrorTypeNetwork:
		multiplier = 3.0 // Longer delays for connection issues
	case ErrorTypeTimeout:
		multiplier = 2.0 // Moderate increase for timeouts
	case ErrorTypeDNS:
		multiplier = 2.5 // DNS issues need time to resolve
	case ErrorTypeRateLimit:
		multiplier = 5.0 // Respect rate limits with longer delays
	default:
		multiplier = 1.0 // Standard backoff
	}

	// Exponential backoff: delay = baseDelay * multiplier * (2^attempt)
	exponentialFactor := 1 << uint(attempt)
	delay := time.Duration(float64(baseDelay) * multiplier * float64(exponentialFactor))

	// Cap maximum delay at 30 seconds
	maxDelay := 30 * time.Second
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// sanitizeErrorMessage removes sensitive information from error messages
func (ec *ErrorClassifier) sanitizeErrorMessage(message string) string {
	// Remove potential sensitive information
	sanitized := message

	// Remove IP addresses (basic pattern)
	sanitized = strings.ReplaceAll(sanitized, "127.0.0.1", "[localhost]")
	// Only replace standalone localhost, not already replaced ones
	if !strings.Contains(sanitized, "[localhost]") {
		sanitized = strings.ReplaceAll(sanitized, "localhost", "[localhost]")
	}

	// Remove potential credentials from URLs
	if strings.Contains(sanitized, "://") {
		// This is a basic sanitization - in production, use more sophisticated URL parsing
		parts := strings.Split(sanitized, "://")
		if len(parts) > 1 {
			urlPart := parts[1]
			if strings.Contains(urlPart, "@") {
				// Remove credentials from URL
				atIndex := strings.Index(urlPart, "@")
				sanitized = parts[0] + "://[credentials]@" + urlPart[atIndex+1:]
			}
		}
	}

	// Limit message length to prevent log flooding
	maxLength := 500
	if len(sanitized) > maxLength {
		sanitized = sanitized[:maxLength] + "... [truncated]"
	}

	return sanitized
}

// IsRetryableError is a utility function to check if any error is retryable
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for our custom error types
	if manticoreErr, ok := err.(*ManticoreError); ok {
		return manticoreErr.IsRetryable()
	}
	if connErr, ok := err.(*ConnectionError); ok {
		return connErr.IsRetryable()
	}

	// Fallback to basic classification
	classifier := NewErrorClassifier()
	_, retryable := classifier.classifyErrorType(err)
	return retryable
}

// GetErrorBackoffDelay extracts backoff delay from error
func GetErrorBackoffDelay(err error) time.Duration {
	if err == nil {
		return 0
	}

	// Check for our custom error types
	if manticoreErr, ok := err.(*ManticoreError); ok {
		if manticoreErr.RetryAfter > 0 {
			return manticoreErr.RetryAfter
		}
	}
	if connErr, ok := err.(*ConnectionError); ok {
		return connErr.GetBackoffDelay()
	}

	// Default backoff
	return 500 * time.Millisecond
}
