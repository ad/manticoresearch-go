package manticore

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// CircuitBreakerState represents the state of the circuit breaker
type CircuitBreakerState int

const (
	// CircuitBreakerClosed - normal operation, requests pass through
	CircuitBreakerClosed CircuitBreakerState = iota
	// CircuitBreakerOpen - failing fast, requests rejected immediately
	CircuitBreakerOpen
	// CircuitBreakerHalfOpen - testing recovery, limited requests allowed
	CircuitBreakerHalfOpen
)

// String returns the string representation of CircuitBreakerState
func (s CircuitBreakerState) String() string {
	switch s {
	case CircuitBreakerClosed:
		return "CLOSED"
	case CircuitBreakerOpen:
		return "OPEN"
	case CircuitBreakerHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreakerConfig defines circuit breaker behavior
type CircuitBreakerConfig struct {
	FailureThreshold int           `json:"failure_threshold"`
	RecoveryTimeout  time.Duration `json:"recovery_timeout"`
	HalfOpenMaxCalls int           `json:"half_open_max_calls"`

	// Advanced configuration
	SuccessThreshold     int           `json:"success_threshold"`      // Successes needed to close from half-open
	MinRequestThreshold  int           `json:"min_request_threshold"`  // Minimum requests before considering failure rate
	FailureRateThreshold float64       `json:"failure_rate_threshold"` // Failure rate (0.0-1.0) to trigger opening
	SlidingWindowSize    int           `json:"sliding_window_size"`    // Size of sliding window for failure rate calculation
	MonitoringInterval   time.Duration `json:"monitoring_interval"`    // Interval for monitoring and state transitions
}

// CircuitBreakerCallback defines callbacks for circuit breaker state changes
type CircuitBreakerCallback interface {
	OnStateChange(oldState, newState CircuitBreakerState, reason string)
}

// DefaultCircuitBreakerConfig returns a default circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:     10,
		RecoveryTimeout:      30 * time.Second,
		HalfOpenMaxCalls:     3,
		SuccessThreshold:     2,
		MinRequestThreshold:  5,
		FailureRateThreshold: 0.5, // 50% failure rate
		SlidingWindowSize:    20,
		MonitoringInterval:   5 * time.Second,
	}
}

// CircuitBreaker implements the circuit breaker pattern with enhanced features
type CircuitBreaker struct {
	config CircuitBreakerConfig

	// State management
	state           CircuitBreakerState
	lastStateChange time.Time
	lastFailureTime time.Time

	// Counters
	consecutiveFailures  int
	consecutiveSuccesses int
	halfOpenCalls        int

	// Sliding window for failure rate calculation
	requestWindow []RequestResult
	windowIndex   int

	// Statistics
	stats CircuitBreakerStats

	// Thread safety
	mutex sync.RWMutex

	// Monitoring
	stopMonitoring chan struct{}
	monitoring     bool

	// Callback for state changes
	callback CircuitBreakerCallback
}

// RequestResult represents the result of a request
type RequestResult struct {
	Timestamp time.Time
	Success   bool
	ErrorType ErrorType
}

// CircuitBreakerStats provides statistics about circuit breaker operation
type CircuitBreakerStats struct {
	State                CircuitBreakerState `json:"state"`
	ConsecutiveFailures  int                 `json:"consecutive_failures"`
	ConsecutiveSuccesses int                 `json:"consecutive_successes"`
	HalfOpenCalls        int                 `json:"half_open_calls"`
	TotalRequests        int64               `json:"total_requests"`
	TotalFailures        int64               `json:"total_failures"`
	TotalSuccesses       int64               `json:"total_successes"`
	CurrentFailureRate   float64             `json:"current_failure_rate"`
	LastStateChange      time.Time           `json:"last_state_change"`
	LastFailureTime      time.Time           `json:"last_failure_time"`
	StateChanges         int64               `json:"state_changes"`
}

// NewCircuitBreaker creates a new circuit breaker with enhanced features
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	cb := &CircuitBreaker{
		config:          config,
		state:           CircuitBreakerClosed,
		lastStateChange: time.Now(),
		requestWindow:   make([]RequestResult, config.SlidingWindowSize),
		stopMonitoring:  make(chan struct{}),
	}

	// Start monitoring goroutine
	cb.startMonitoring()

	return cb
}

// SetCallback sets the callback for state change notifications
func (cb *CircuitBreaker) SetCallback(callback CircuitBreakerCallback) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	cb.callback = callback
}

// Execute executes an operation with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, operation func(ctx context.Context) error) error {
	// Check if request should be allowed
	if !cb.shouldAllowRequest() {
		cb.recordRejection()
		return &ManticoreError{
			StatusCode: 0,
			Message:    fmt.Sprintf("circuit breaker is %s: too many failures", cb.getState()),
			Retryable:  true, // Circuit breaker errors are retryable after recovery
			ErrorType:  ErrorTypeCircuitBreaker,
		}
	}

	// Execute the operation
	err := operation(ctx)

	// Record the result
	if err != nil {
		cb.recordFailure(err)
	} else {
		cb.recordSuccess()
	}

	return err
}

// shouldAllowRequest determines if a request should be allowed based on circuit breaker state
func (cb *CircuitBreaker) shouldAllowRequest() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	switch cb.state {
	case CircuitBreakerClosed:
		return true

	case CircuitBreakerOpen:
		// Check if recovery timeout has passed
		if time.Since(cb.lastFailureTime) >= cb.config.RecoveryTimeout {
			cb.transitionToHalfOpen()
			return true
		}
		return false

	case CircuitBreakerHalfOpen:
		// Allow limited requests in half-open state
		if cb.halfOpenCalls < cb.config.HalfOpenMaxCalls {
			cb.halfOpenCalls++
			return true
		}
		return false

	default:
		return false
	}
}

// recordSuccess records a successful operation
func (cb *CircuitBreaker) recordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Update statistics
	cb.stats.TotalRequests++
	cb.stats.TotalSuccesses++

	// Add to sliding window
	cb.addToWindow(RequestResult{
		Timestamp: time.Now(),
		Success:   true,
	})

	switch cb.state {
	case CircuitBreakerHalfOpen:
		cb.consecutiveSuccesses++
		log.Printf("Circuit breaker: success %d/%d in HALF-OPEN state",
			cb.consecutiveSuccesses, cb.config.SuccessThreshold)

		// Check if we have enough successes to close the circuit
		if cb.consecutiveSuccesses >= cb.config.SuccessThreshold {
			cb.transitionToClosed()
		}

	case CircuitBreakerClosed:
		// Reset failure count on success
		if cb.consecutiveFailures > 0 {
			log.Printf("Circuit breaker: resetting %d consecutive failures after success",
				cb.consecutiveFailures)
			cb.consecutiveFailures = 0
		}
	}
}

// recordFailure records a failed operation
func (cb *CircuitBreaker) recordFailure(err error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Update statistics
	cb.stats.TotalRequests++
	cb.stats.TotalFailures++
	cb.lastFailureTime = time.Now()

	// Classify error type
	errorType := ErrorTypeUnknown
	if manticoreErr, ok := err.(*ManticoreError); ok {
		errorType = manticoreErr.ErrorType
	} else if connErr, ok := err.(*ConnectionError); ok {
		errorType = connErr.ErrorType
	}

	// Add to sliding window
	cb.addToWindow(RequestResult{
		Timestamp: time.Now(),
		Success:   false,
		ErrorType: errorType,
	})

	cb.consecutiveFailures++
	cb.consecutiveSuccesses = 0 // Reset success count

	switch cb.state {
	case CircuitBreakerClosed:
		// Check if we should open the circuit
		if cb.shouldOpenCircuit() {
			cb.transitionToOpen()
		}

	case CircuitBreakerHalfOpen:
		// Failure in half-open state - back to open
		log.Printf("Circuit breaker: failure during recovery test, returning to OPEN state")
		cb.transitionToOpen()
	}
}

// recordRejection records a rejected request (when circuit is open)
func (cb *CircuitBreaker) recordRejection() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.stats.TotalRequests++
	// Note: rejections are not counted as failures in statistics
}

// shouldOpenCircuit determines if the circuit should be opened
func (cb *CircuitBreaker) shouldOpenCircuit() bool {
	// Check consecutive failures threshold
	if cb.consecutiveFailures >= cb.config.FailureThreshold {
		log.Printf("Circuit breaker: opening due to %d consecutive failures (threshold: %d)",
			cb.consecutiveFailures, cb.config.FailureThreshold)
		return true
	}

	// Check failure rate if we have enough requests
	if cb.stats.TotalRequests >= int64(cb.config.MinRequestThreshold) {
		failureRate := cb.calculateCurrentFailureRate()
		if failureRate >= cb.config.FailureRateThreshold {
			log.Printf("Circuit breaker: opening due to failure rate %.2f%% (threshold: %.2f%%)",
				failureRate*100, cb.config.FailureRateThreshold*100)
			return true
		}
	}

	return false
}

// calculateCurrentFailureRate calculates the current failure rate from the sliding window
func (cb *CircuitBreaker) calculateCurrentFailureRate() float64 {
	if len(cb.requestWindow) == 0 {
		return 0.0
	}

	totalRequests := 0
	failures := 0
	cutoff := time.Now().Add(-cb.config.MonitoringInterval * 5) // Consider last 5 monitoring intervals

	for _, result := range cb.requestWindow {
		if !result.Timestamp.IsZero() && result.Timestamp.After(cutoff) {
			totalRequests++
			if !result.Success {
				failures++
			}
		}
	}

	if totalRequests == 0 {
		return 0.0
	}

	return float64(failures) / float64(totalRequests)
}

// addToWindow adds a request result to the sliding window
func (cb *CircuitBreaker) addToWindow(result RequestResult) {
	cb.requestWindow[cb.windowIndex] = result
	cb.windowIndex = (cb.windowIndex + 1) % len(cb.requestWindow)
}

// State transition methods

// transitionToClosed transitions the circuit breaker to closed state
func (cb *CircuitBreaker) transitionToClosed() {
	if cb.state != CircuitBreakerClosed {
		oldState := cb.state
		log.Printf("Circuit breaker: transitioning from %s to CLOSED", cb.state)
		cb.state = CircuitBreakerClosed
		cb.lastStateChange = time.Now()
		cb.consecutiveFailures = 0
		cb.consecutiveSuccesses = 0
		cb.halfOpenCalls = 0
		cb.stats.StateChanges++

		// Notify callback
		if cb.callback != nil {
			cb.callback.OnStateChange(oldState, CircuitBreakerClosed, "successful recovery")
		}
	}
}

// transitionToOpen transitions the circuit breaker to open state
func (cb *CircuitBreaker) transitionToOpen() {
	if cb.state != CircuitBreakerOpen {
		oldState := cb.state
		log.Printf("Circuit breaker: transitioning from %s to OPEN after %d consecutive failures",
			cb.state, cb.consecutiveFailures)
		cb.state = CircuitBreakerOpen
		cb.lastStateChange = time.Now()
		cb.halfOpenCalls = 0
		cb.consecutiveSuccesses = 0
		cb.stats.StateChanges++

		// Notify callback
		if cb.callback != nil {
			cb.callback.OnStateChange(oldState, CircuitBreakerOpen, fmt.Sprintf("too many failures (%d)", cb.consecutiveFailures))
		}
	}
}

// transitionToHalfOpen transitions the circuit breaker to half-open state
func (cb *CircuitBreaker) transitionToHalfOpen() {
	if cb.state != CircuitBreakerHalfOpen {
		oldState := cb.state
		log.Printf("Circuit breaker: transitioning from %s to HALF-OPEN for recovery test", cb.state)
		cb.state = CircuitBreakerHalfOpen
		cb.lastStateChange = time.Now()
		cb.halfOpenCalls = 0
		cb.consecutiveSuccesses = 0
		cb.stats.StateChanges++

		// Notify callback
		if cb.callback != nil {
			cb.callback.OnStateChange(oldState, CircuitBreakerHalfOpen, "recovery timeout reached")
		}
	}
}

// Public methods for state inspection

// GetState returns the current circuit breaker state (thread-safe)
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// getState returns the current circuit breaker state (internal, assumes lock held)
func (cb *CircuitBreaker) getState() CircuitBreakerState {
	return cb.state
}

// GetStats returns current circuit breaker statistics
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	stats := cb.stats
	stats.State = cb.state
	stats.ConsecutiveFailures = cb.consecutiveFailures
	stats.ConsecutiveSuccesses = cb.consecutiveSuccesses
	stats.HalfOpenCalls = cb.halfOpenCalls
	stats.CurrentFailureRate = cb.calculateCurrentFailureRate()
	stats.LastStateChange = cb.lastStateChange
	stats.LastFailureTime = cb.lastFailureTime

	return stats
}

// IsOpen returns true if the circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.GetState() == CircuitBreakerOpen
}

// IsClosed returns true if the circuit breaker is closed
func (cb *CircuitBreaker) IsClosed() bool {
	return cb.GetState() == CircuitBreakerClosed
}

// IsHalfOpen returns true if the circuit breaker is half-open
func (cb *CircuitBreaker) IsHalfOpen() bool {
	return cb.GetState() == CircuitBreakerHalfOpen
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	log.Printf("Circuit breaker: manual reset to CLOSED state")
	cb.transitionToClosed()
}

// ForceOpen manually forces the circuit breaker to open state
func (cb *CircuitBreaker) ForceOpen() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	log.Printf("Circuit breaker: manual force to OPEN state")
	cb.transitionToOpen()
}

// Monitoring

// startMonitoring starts the background monitoring goroutine
func (cb *CircuitBreaker) startMonitoring() {
	if cb.monitoring {
		return
	}

	cb.monitoring = true
	go cb.monitoringLoop()
}

// stopMonitoringLoop stops the background monitoring goroutine
func (cb *CircuitBreaker) stopMonitoringLoop() {
	if !cb.monitoring {
		return
	}

	close(cb.stopMonitoring)
	cb.monitoring = false
}

// monitoringLoop runs the background monitoring
func (cb *CircuitBreaker) monitoringLoop() {
	ticker := time.NewTicker(cb.config.MonitoringInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cb.performMonitoringCheck()
		case <-cb.stopMonitoring:
			return
		}
	}
}

// performMonitoringCheck performs periodic monitoring checks
func (cb *CircuitBreaker) performMonitoringCheck() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Update current failure rate in stats
	cb.stats.CurrentFailureRate = cb.calculateCurrentFailureRate()

	// Log periodic status if there's activity
	if cb.stats.TotalRequests > 0 {
		log.Printf("Circuit breaker status: state=%s, failures=%d, successes=%d, failure_rate=%.2f%%",
			cb.state, cb.stats.TotalFailures, cb.stats.TotalSuccesses, cb.stats.CurrentFailureRate*100)
	}
}

// Close gracefully shuts down the circuit breaker
func (cb *CircuitBreaker) Close() {
	cb.stopMonitoringLoop()
}

// CircuitBreakerWithRetry combines circuit breaker with retry mechanism
type CircuitBreakerWithRetry struct {
	circuitBreaker *CircuitBreaker
	retryManager   *RetryManager
}

// NewCircuitBreakerWithRetry creates a new circuit breaker integrated with retry mechanism
func NewCircuitBreakerWithRetry(cbConfig CircuitBreakerConfig, retryConfig RetryConfig) *CircuitBreakerWithRetry {
	return &CircuitBreakerWithRetry{
		circuitBreaker: NewCircuitBreaker(cbConfig),
		retryManager:   NewRetryManager(retryConfig),
	}
}

// SetCallback sets the callback for circuit breaker state changes
func (cbr *CircuitBreakerWithRetry) SetCallback(callback CircuitBreakerCallback) {
	cbr.circuitBreaker.SetCallback(callback)
}

// Execute executes an operation with both circuit breaker protection and retry logic
func (cbr *CircuitBreakerWithRetry) Execute(ctx context.Context, endpoint, method string, operation func(ctx context.Context) error) error {
	// Wrap the operation with circuit breaker
	circuitBreakerOperation := func(ctx context.Context, retryCtx *RetryContext) error {
		return cbr.circuitBreaker.Execute(ctx, operation)
	}

	// Execute with retry mechanism
	return cbr.retryManager.Execute(ctx, endpoint, method, circuitBreakerOperation)
}

// GetCircuitBreakerStats returns circuit breaker statistics
func (cbr *CircuitBreakerWithRetry) GetCircuitBreakerStats() CircuitBreakerStats {
	return cbr.circuitBreaker.GetStats()
}

// GetRetryStats returns retry manager statistics
func (cbr *CircuitBreakerWithRetry) GetRetryStats() RetryStats {
	return cbr.retryManager.GetRetryStats()
}

// Close gracefully shuts down both circuit breaker and retry manager
func (cbr *CircuitBreakerWithRetry) Close() {
	cbr.circuitBreaker.Close()
}
