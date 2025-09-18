package manticore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ad/manticoresearch-go/internal/models"
)

// manticoreHTTPClient implements ManticoreHTTPClient interface
type manticoreHTTPClient struct {
	httpClient              *http.Client
	baseURL                 string
	circuitBreakerWithRetry *CircuitBreakerWithRetry
	isConnected             bool
	bulkConfig              BulkConfig
	metricsCollector        *MetricsCollector
	logger                  *Logger
}

// Ensure manticoreHTTPClient implements ClientInterface
var _ ClientInterface = (*manticoreHTTPClient)(nil)

// NewHTTPClient creates a new HTTP-based Manticore client
func NewHTTPClient(config HTTPClientConfig) ClientInterface {
	// Configure HTTP transport with optimized settings
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		DisableCompression:    false,
		ForceAttemptHTTP2:     false, // Disable HTTP/2 for better compatibility
		WriteBufferSize:       32768, // 32KB write buffer
		ReadBufferSize:        32768, // 32KB read buffer
	}

	httpClient := &http.Client{
		Timeout:   config.Timeout,
		Transport: transport,
	}

	// Create enhanced circuit breaker with retry integration
	circuitBreakerWithRetry := NewCircuitBreakerWithRetry(
		CircuitBreakerConfig{
			FailureThreshold:     config.CircuitBreakerConfig.FailureThreshold,
			RecoveryTimeout:      config.CircuitBreakerConfig.RecoveryTimeout,
			HalfOpenMaxCalls:     config.CircuitBreakerConfig.HalfOpenMaxCalls,
			SuccessThreshold:     2,
			MinRequestThreshold:  5,
			FailureRateThreshold: 0.5,
			SlidingWindowSize:    20,
			MonitoringInterval:   5 * time.Second,
		},
		RetryConfig{
			MaxAttempts:                  config.RetryConfig.MaxAttempts,
			BaseDelay:                    config.RetryConfig.BaseDelay,
			MaxDelay:                     config.RetryConfig.MaxDelay,
			JitterPercent:                config.RetryConfig.JitterPercent,
			TimeoutMultiplier:            2.0,
			ConnectionMultiplier:         3.0,
			ServiceUnavailableMultiplier: 4.0,
			RateLimitMultiplier:          5.0,
			PerAttemptTimeout:            30 * time.Second,
			TotalTimeout:                 5 * time.Minute,
		},
	)

	// Initialize monitoring components
	metricsCollector := NewMetricsCollector()
	logger := NewLogger(LogLevelInfo)

	// Set up circuit breaker callback for monitoring
	callback := NewMetricsCircuitBreakerCallback(metricsCollector, logger)
	circuitBreakerWithRetry.SetCallback(callback)

	return &manticoreHTTPClient{
		httpClient:              httpClient,
		baseURL:                 strings.TrimSuffix(config.BaseURL, "/"),
		circuitBreakerWithRetry: circuitBreakerWithRetry,
		isConnected:             false,
		bulkConfig:              config.BulkConfig,
		metricsCollector:        metricsCollector,
		logger:                  logger,
	}
}

// Connection management methods

// WaitForReady waits for Manticore to be ready with timeout and comprehensive logging
func (mc *manticoreHTTPClient) WaitForReady(timeout time.Duration) error {
	startTime := time.Now()
	deadline := startTime.Add(timeout)
	log.Printf("Waiting for Manticore HTTP client to be ready (timeout: %v)", timeout)

	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		log.Printf("Health check attempt %d", attempt)

		if err := mc.HealthCheck(); err == nil {
			totalDuration := time.Since(startTime)
			log.Printf("Manticore HTTP client is ready after %v (%d attempts)", totalDuration, attempt)
			mc.isConnected = true
			return nil
		}

		// Wait before next attempt
		time.Sleep(2 * time.Second)
	}

	totalDuration := time.Since(startTime)
	log.Printf("Timeout waiting for Manticore HTTP client to be ready after %v (%d attempts)", totalDuration, attempt)
	return fmt.Errorf("timeout waiting for Manticore to be ready after %v", totalDuration)
}

// HealthCheck verifies that the Manticore connection is healthy
func (mc *manticoreHTTPClient) HealthCheck() error {
	log.Printf("Performing health check on %s", mc.baseURL)

	// Create a simple search request to test connectivity
	testRequest := SearchRequest{
		Index: "test_health_check",
		Query: map[string]interface{}{
			"match_all": map[string]interface{}{},
		},
		Limit: 1,
	}

	reqBody, err := json.Marshal(testRequest)
	if err != nil {
		log.Printf("Health check failed: could not marshal test request: %v", err)
		return fmt.Errorf("health check failed: %v", err)
	}

	req, err := http.NewRequest("POST", mc.baseURL+"/search", bytes.NewReader(reqBody))
	if err != nil {
		log.Printf("Health check failed: could not create HTTP request: %v", err)
		return fmt.Errorf("health check failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use a shorter timeout for health checks
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Health check failed: HTTP request failed: %v", err)
		return fmt.Errorf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	// Accept both success responses and 400 responses (table not found is OK for health check)
	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Health check failed: HTTP %d, %s", resp.StatusCode, string(body))
		return fmt.Errorf("health check failed: HTTP %d", resp.StatusCode)
	}

	log.Printf("Health check passed: HTTP %d", resp.StatusCode)
	return nil
}

// IsConnected returns the connection status
func (mc *manticoreHTTPClient) IsConnected() bool {
	return mc.isConnected
}

// Close performs graceful shutdown of the HTTP client
func (mc *manticoreHTTPClient) Close() error {
	log.Printf("Closing Manticore HTTP client")

	// Close circuit breaker monitoring
	if mc.circuitBreakerWithRetry != nil {
		mc.circuitBreakerWithRetry.Close()
	}

	// Close idle connections
	if transport, ok := mc.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}

	mc.isConnected = false

	// Log final metrics before closing
	if mc.metricsCollector != nil {
		mc.metricsCollector.LogMetrics()
	}

	return nil
}

// GetMetrics returns current metrics
func (mc *manticoreHTTPClient) GetMetrics() Metrics {
	if mc.metricsCollector != nil {
		return mc.metricsCollector.GetMetrics()
	}
	return Metrics{}
}

// LogMetrics logs current metrics
func (mc *manticoreHTTPClient) LogMetrics() {
	if mc.metricsCollector != nil {
		mc.metricsCollector.LogMetrics()
	}
}

// Search performs search using the HTTP client (adapter method for ClientInterface)
func (mc *manticoreHTTPClient) Search(query string, mode models.SearchMode, page, pageSize int) (*models.SearchResponse, error) {
	// This method is implemented as an adapter to maintain compatibility
	// The actual search logic should be handled by the search engine
	return nil, fmt.Errorf("search method not implemented for HTTP client - use search engine instead")
}
