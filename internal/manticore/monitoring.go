package manticore

import (
	"log"
	"sync"
	"time"
)

// MetricsCollector collects and tracks performance metrics for Manticore operations
type MetricsCollector struct {
	mu                    sync.RWMutex
	requestCount          int64
	successCount          int64
	errorCount            int64
	totalDuration         time.Duration
	circuitBreakerOpens   int64
	circuitBreakerCloses  int64
	retryAttempts         int64
	bulkOperations        int64
	bulkDocumentsIndexed  int64
	searchOperations      int64
	indexOperations       int64
	schemaOperations      int64
	lastOperationTime     time.Time
	operationTypes        map[string]int64
	errorTypes            map[string]int64
	responseTimeHistogram map[string][]time.Duration
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		operationTypes:        make(map[string]int64),
		errorTypes:            make(map[string]int64),
		responseTimeHistogram: make(map[string][]time.Duration),
	}
}

// RecordRequest records a request with its duration and outcome
func (mc *MetricsCollector) RecordRequest(operation string, duration time.Duration, success bool, errorType string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.requestCount++
	mc.totalDuration += duration
	mc.lastOperationTime = time.Now()
	mc.operationTypes[operation]++

	// Record response time
	if mc.responseTimeHistogram[operation] == nil {
		mc.responseTimeHistogram[operation] = make([]time.Duration, 0)
	}
	mc.responseTimeHistogram[operation] = append(mc.responseTimeHistogram[operation], duration)

	// Keep only last 100 response times per operation
	if len(mc.responseTimeHistogram[operation]) > 100 {
		mc.responseTimeHistogram[operation] = mc.responseTimeHistogram[operation][1:]
	}

	if success {
		mc.successCount++
	} else {
		mc.errorCount++
		if errorType != "" {
			mc.errorTypes[errorType]++
		}
	}
}

// RecordCircuitBreakerOpen records a circuit breaker opening
func (mc *MetricsCollector) RecordCircuitBreakerOpen() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.circuitBreakerOpens++
}

// RecordCircuitBreakerClose records a circuit breaker closing
func (mc *MetricsCollector) RecordCircuitBreakerClose() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.circuitBreakerCloses++
}

// RecordRetryAttempt records a retry attempt
func (mc *MetricsCollector) RecordRetryAttempt() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.retryAttempts++
}

// RecordBulkOperation records a bulk operation with document count
func (mc *MetricsCollector) RecordBulkOperation(documentCount int) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.bulkOperations++
	mc.bulkDocumentsIndexed += int64(documentCount)
}

// RecordSearchOperation records a search operation
func (mc *MetricsCollector) RecordSearchOperation() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.searchOperations++
}

// RecordIndexOperation records an index operation
func (mc *MetricsCollector) RecordIndexOperation() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.indexOperations++
}

// RecordSchemaOperation records a schema operation
func (mc *MetricsCollector) RecordSchemaOperation() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.schemaOperations++
}

// GetMetrics returns current metrics snapshot
func (mc *MetricsCollector) GetMetrics() Metrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Calculate average response time
	avgDuration := time.Duration(0)
	if mc.requestCount > 0 {
		avgDuration = mc.totalDuration / time.Duration(mc.requestCount)
	}

	// Calculate success rate
	successRate := 0.0
	if mc.requestCount > 0 {
		successRate = float64(mc.successCount) / float64(mc.requestCount) * 100
	}

	// Copy maps to avoid race conditions
	operationTypes := make(map[string]int64)
	for k, v := range mc.operationTypes {
		operationTypes[k] = v
	}

	errorTypes := make(map[string]int64)
	for k, v := range mc.errorTypes {
		errorTypes[k] = v
	}

	// Calculate response time percentiles
	responseTimePercentiles := make(map[string]ResponseTimePercentiles)
	for operation, times := range mc.responseTimeHistogram {
		if len(times) > 0 {
			responseTimePercentiles[operation] = calculatePercentiles(times)
		}
	}

	return Metrics{
		RequestCount:            mc.requestCount,
		SuccessCount:            mc.successCount,
		ErrorCount:              mc.errorCount,
		SuccessRate:             successRate,
		AverageResponseTime:     avgDuration,
		TotalDuration:           mc.totalDuration,
		CircuitBreakerOpens:     mc.circuitBreakerOpens,
		CircuitBreakerCloses:    mc.circuitBreakerCloses,
		RetryAttempts:           mc.retryAttempts,
		BulkOperations:          mc.bulkOperations,
		BulkDocumentsIndexed:    mc.bulkDocumentsIndexed,
		SearchOperations:        mc.searchOperations,
		IndexOperations:         mc.indexOperations,
		SchemaOperations:        mc.schemaOperations,
		LastOperationTime:       mc.lastOperationTime,
		OperationTypes:          operationTypes,
		ErrorTypes:              errorTypes,
		ResponseTimePercentiles: responseTimePercentiles,
	}
}

// LogMetrics logs current metrics to the standard logger
func (mc *MetricsCollector) LogMetrics() {
	metrics := mc.GetMetrics()

	log.Printf("[METRICS] === Manticore Client Metrics ===")
	log.Printf("[METRICS] Total Requests: %d (Success: %d, Errors: %d)",
		metrics.RequestCount, metrics.SuccessCount, metrics.ErrorCount)
	log.Printf("[METRICS] Success Rate: %.2f%%", metrics.SuccessRate)
	log.Printf("[METRICS] Average Response Time: %v", metrics.AverageResponseTime)
	log.Printf("[METRICS] Total Duration: %v", metrics.TotalDuration)

	if metrics.CircuitBreakerOpens > 0 || metrics.CircuitBreakerCloses > 0 {
		log.Printf("[METRICS] Circuit Breaker: Opens=%d, Closes=%d",
			metrics.CircuitBreakerOpens, metrics.CircuitBreakerCloses)
	}

	if metrics.RetryAttempts > 0 {
		log.Printf("[METRICS] Retry Attempts: %d", metrics.RetryAttempts)
	}

	if metrics.BulkOperations > 0 {
		log.Printf("[METRICS] Bulk Operations: %d (Documents: %d)",
			metrics.BulkOperations, metrics.BulkDocumentsIndexed)
	}

	log.Printf("[METRICS] Operations: Search=%d, Index=%d, Schema=%d",
		metrics.SearchOperations, metrics.IndexOperations, metrics.SchemaOperations)

	if len(metrics.OperationTypes) > 0 {
		log.Printf("[METRICS] Operation Types:")
		for op, count := range metrics.OperationTypes {
			log.Printf("[METRICS]   %s: %d", op, count)
		}
	}

	if len(metrics.ErrorTypes) > 0 {
		log.Printf("[METRICS] Error Types:")
		for errType, count := range metrics.ErrorTypes {
			log.Printf("[METRICS]   %s: %d", errType, count)
		}
	}

	if len(metrics.ResponseTimePercentiles) > 0 {
		log.Printf("[METRICS] Response Time Percentiles:")
		for operation, percentiles := range metrics.ResponseTimePercentiles {
			log.Printf("[METRICS]   %s: P50=%v, P95=%v, P99=%v",
				operation, percentiles.P50, percentiles.P95, percentiles.P99)
		}
	}

	log.Printf("[METRICS] Last Operation: %v", metrics.LastOperationTime.Format(time.RFC3339))
	log.Printf("[METRICS] ================================")
}

// Metrics represents a snapshot of metrics
type Metrics struct {
	RequestCount            int64
	SuccessCount            int64
	ErrorCount              int64
	SuccessRate             float64
	AverageResponseTime     time.Duration
	TotalDuration           time.Duration
	CircuitBreakerOpens     int64
	CircuitBreakerCloses    int64
	RetryAttempts           int64
	BulkOperations          int64
	BulkDocumentsIndexed    int64
	SearchOperations        int64
	IndexOperations         int64
	SchemaOperations        int64
	LastOperationTime       time.Time
	OperationTypes          map[string]int64
	ErrorTypes              map[string]int64
	ResponseTimePercentiles map[string]ResponseTimePercentiles
}

// ResponseTimePercentiles represents response time percentiles for an operation
type ResponseTimePercentiles struct {
	P50 time.Duration
	P95 time.Duration
	P99 time.Duration
}

// calculatePercentiles calculates response time percentiles
func calculatePercentiles(times []time.Duration) ResponseTimePercentiles {
	if len(times) == 0 {
		return ResponseTimePercentiles{}
	}

	// Sort times
	sorted := make([]time.Duration, len(times))
	copy(sorted, times)

	// Simple bubble sort for small arrays
	for i := 0; i < len(sorted); i++ {
		for j := 0; j < len(sorted)-1-i; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	// Calculate percentiles
	p50Index := int(float64(len(sorted)) * 0.5)
	p95Index := int(float64(len(sorted)) * 0.95)
	p99Index := int(float64(len(sorted)) * 0.99)

	if p50Index >= len(sorted) {
		p50Index = len(sorted) - 1
	}
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	if p99Index >= len(sorted) {
		p99Index = len(sorted) - 1
	}

	return ResponseTimePercentiles{
		P50: sorted[p50Index],
		P95: sorted[p95Index],
		P99: sorted[p99Index],
	}
}

// PeriodicMetricsLogger logs metrics periodically
type PeriodicMetricsLogger struct {
	collector *MetricsCollector
	interval  time.Duration
	stopCh    chan struct{}
	stopped   bool
	mu        sync.Mutex
}

// NewPeriodicMetricsLogger creates a new periodic metrics logger
func NewPeriodicMetricsLogger(collector *MetricsCollector, interval time.Duration) *PeriodicMetricsLogger {
	return &PeriodicMetricsLogger{
		collector: collector,
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

// Start starts the periodic logging
func (pml *PeriodicMetricsLogger) Start() {
	pml.mu.Lock()
	defer pml.mu.Unlock()

	if pml.stopped {
		return
	}

	go func() {
		ticker := time.NewTicker(pml.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				pml.collector.LogMetrics()
			case <-pml.stopCh:
				return
			}
		}
	}()
}

// Stop stops the periodic logging
func (pml *PeriodicMetricsLogger) Stop() {
	pml.mu.Lock()
	defer pml.mu.Unlock()

	if !pml.stopped {
		close(pml.stopCh)
		pml.stopped = true
	}
}

// LogLevel represents different log levels
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// String returns the string representation of the log level
func (ll LogLevel) String() string {
	switch ll {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger provides structured logging for Manticore operations
type Logger struct {
	level LogLevel
}

// NewLogger creates a new logger with the specified level
func NewLogger(level LogLevel) *Logger {
	return &Logger{level: level}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level <= LogLevelDebug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level <= LogLevelInfo {
		log.Printf("[INFO] "+format, args...)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level <= LogLevelWarn {
		log.Printf("[WARN] "+format, args...)
	}
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level <= LogLevelError {
		log.Printf("[ERROR] "+format, args...)
	}
}

// LogOperation logs an operation with timing and outcome
func (l *Logger) LogOperation(operation string, duration time.Duration, success bool, details string) {
	status := "SUCCESS"
	if !success {
		status = "FAILED"
	}

	l.Info("[%s] %s completed in %v - %s", operation, status, duration, details)
}

// LogCircuitBreakerStateChange logs circuit breaker state changes
func (l *Logger) LogCircuitBreakerStateChange(oldState, newState CircuitBreakerState, reason string) {
	l.Warn("Circuit breaker state changed: %s -> %s (%s)", oldState, newState, reason)
}

// LogRetryAttempt logs retry attempts
func (l *Logger) LogRetryAttempt(operation string, attempt int, maxAttempts int, delay time.Duration, err error) {
	l.Warn("Retrying %s (attempt %d/%d) after %v delay due to error: %v",
		operation, attempt, maxAttempts, delay, err)
}

// LogBulkOperation logs bulk operation progress
func (l *Logger) LogBulkOperation(operation string, processed, total int, duration time.Duration) {
	l.Info("Bulk %s progress: %d/%d documents processed in %v",
		operation, processed, total, duration)
}

// MetricsCircuitBreakerCallback implements CircuitBreakerCallback for metrics collection
type MetricsCircuitBreakerCallback struct {
	metricsCollector *MetricsCollector
	logger           *Logger
}

// NewMetricsCircuitBreakerCallback creates a new metrics callback
func NewMetricsCircuitBreakerCallback(collector *MetricsCollector, logger *Logger) *MetricsCircuitBreakerCallback {
	return &MetricsCircuitBreakerCallback{
		metricsCollector: collector,
		logger:           logger,
	}
}

// OnStateChange handles circuit breaker state changes
func (mcb *MetricsCircuitBreakerCallback) OnStateChange(oldState, newState CircuitBreakerState, reason string) {
	if mcb.metricsCollector != nil {
		if newState == CircuitBreakerOpen {
			mcb.metricsCollector.RecordCircuitBreakerOpen()
		} else if newState == CircuitBreakerClosed && oldState != CircuitBreakerClosed {
			mcb.metricsCollector.RecordCircuitBreakerClose()
		}
	}

	if mcb.logger != nil {
		mcb.logger.LogCircuitBreakerStateChange(oldState, newState, reason)
	}
}
