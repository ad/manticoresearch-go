package manticore

import (
	"os"
	"testing"
	"time"
)

func TestNewClientFromEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "default configuration",
			envVars: map[string]string{
				"MANTICORE_HOST": "localhost:9308",
			},
			wantErr: false,
		},
		{
			name: "custom timeout",
			envVars: map[string]string{
				"MANTICORE_HOST":         "localhost:9308",
				"MANTICORE_HTTP_TIMEOUT": "30s",
			},
			wantErr: false,
		},
		{
			name: "invalid timeout",
			envVars: map[string]string{
				"MANTICORE_HOST":         "localhost:9308",
				"MANTICORE_HTTP_TIMEOUT": "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			client, err := NewClientFromEnvironment()

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewClientFromEnvironment() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("NewClientFromEnvironment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if client == nil {
				t.Errorf("NewClientFromEnvironment() returned nil client")
				return
			}
		})
	}
}

func TestLoadHTTPConfigFromEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		checkFn func(*HTTPClientConfig) error
	}{
		{
			name: "default configuration",
			envVars: map[string]string{
				"MANTICORE_HOST": "localhost:9308",
			},
			wantErr: false,
			checkFn: func(config *HTTPClientConfig) error {
				if config.BaseURL != "http://localhost:9308" {
					t.Errorf("Expected BaseURL http://localhost:9308, got %s", config.BaseURL)
				}
				if config.Timeout != 60*time.Second {
					t.Errorf("Expected timeout 60s, got %v", config.Timeout)
				}
				return nil
			},
		},
		{
			name: "custom timeout",
			envVars: map[string]string{
				"MANTICORE_HOST":         "localhost:9308",
				"MANTICORE_HTTP_TIMEOUT": "30s",
			},
			wantErr: false,
			checkFn: func(config *HTTPClientConfig) error {
				if config.Timeout != 30*time.Second {
					t.Errorf("Expected timeout 30s, got %v", config.Timeout)
				}
				return nil
			},
		},
		{
			name: "custom connection pool settings",
			envVars: map[string]string{
				"MANTICORE_HOST":                         "localhost:9308",
				"MANTICORE_HTTP_MAX_IDLE_CONNS":          "50",
				"MANTICORE_HTTP_MAX_IDLE_CONNS_PER_HOST": "25",
				"MANTICORE_HTTP_IDLE_CONN_TIMEOUT":       "120s",
			},
			wantErr: false,
			checkFn: func(config *HTTPClientConfig) error {
				if config.MaxIdleConns != 50 {
					t.Errorf("Expected MaxIdleConns 50, got %d", config.MaxIdleConns)
				}
				if config.MaxIdleConnsPerHost != 25 {
					t.Errorf("Expected MaxIdleConnsPerHost 25, got %d", config.MaxIdleConnsPerHost)
				}
				if config.IdleConnTimeout != 120*time.Second {
					t.Errorf("Expected IdleConnTimeout 120s, got %v", config.IdleConnTimeout)
				}
				return nil
			},
		},
		{
			name: "custom retry settings",
			envVars: map[string]string{
				"MANTICORE_HOST":                      "localhost:9308",
				"MANTICORE_HTTP_RETRY_MAX_ATTEMPTS":   "3",
				"MANTICORE_HTTP_RETRY_BASE_DELAY":     "1s",
				"MANTICORE_HTTP_RETRY_MAX_DELAY":      "60s",
				"MANTICORE_HTTP_RETRY_JITTER_PERCENT": "0.2",
			},
			wantErr: false,
			checkFn: func(config *HTTPClientConfig) error {
				if config.RetryConfig.MaxAttempts != 3 {
					t.Errorf("Expected MaxAttempts 3, got %d", config.RetryConfig.MaxAttempts)
				}
				if config.RetryConfig.BaseDelay != time.Second {
					t.Errorf("Expected BaseDelay 1s, got %v", config.RetryConfig.BaseDelay)
				}
				if config.RetryConfig.MaxDelay != 60*time.Second {
					t.Errorf("Expected MaxDelay 60s, got %v", config.RetryConfig.MaxDelay)
				}
				if config.RetryConfig.JitterPercent != 0.2 {
					t.Errorf("Expected JitterPercent 0.2, got %f", config.RetryConfig.JitterPercent)
				}
				return nil
			},
		},
		{
			name: "custom circuit breaker settings",
			envVars: map[string]string{
				"MANTICORE_HOST":                        "localhost:9308",
				"MANTICORE_HTTP_CB_FAILURE_THRESHOLD":   "10",
				"MANTICORE_HTTP_CB_RECOVERY_TIMEOUT":    "60s",
				"MANTICORE_HTTP_CB_HALF_OPEN_MAX_CALLS": "5",
			},
			wantErr: false,
			checkFn: func(config *HTTPClientConfig) error {
				if config.CircuitBreakerConfig.FailureThreshold != 10 {
					t.Errorf("Expected FailureThreshold 10, got %d", config.CircuitBreakerConfig.FailureThreshold)
				}
				if config.CircuitBreakerConfig.RecoveryTimeout != 60*time.Second {
					t.Errorf("Expected RecoveryTimeout 60s, got %v", config.CircuitBreakerConfig.RecoveryTimeout)
				}
				if config.CircuitBreakerConfig.HalfOpenMaxCalls != 5 {
					t.Errorf("Expected HalfOpenMaxCalls 5, got %d", config.CircuitBreakerConfig.HalfOpenMaxCalls)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			config, err := LoadHTTPConfigFromEnvironment()

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadHTTPConfigFromEnvironment() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("LoadHTTPConfigFromEnvironment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if config == nil {
				t.Errorf("LoadHTTPConfigFromEnvironment() returned nil config")
				return
			}

			if tt.checkFn != nil {
				tt.checkFn(config)
			}
		})
	}
}

func TestDefaultHTTPConfig(t *testing.T) {
	host := "localhost:9308"
	config := DefaultHTTPConfig(host)

	expectedBaseURL := "http://localhost:9308"
	if config.BaseURL != expectedBaseURL {
		t.Errorf("DefaultHTTPConfig() BaseURL = %v, want %v", config.BaseURL, expectedBaseURL)
	}

	if config.Timeout <= 0 {
		t.Errorf("DefaultHTTPConfig() Timeout should be positive, got %v", config.Timeout)
	}

	if config.MaxIdleConns <= 0 {
		t.Errorf("DefaultHTTPConfig() MaxIdleConns should be positive, got %v", config.MaxIdleConns)
	}

	if config.RetryConfig.MaxAttempts <= 0 {
		t.Errorf("DefaultHTTPConfig() RetryConfig.MaxAttempts should be positive, got %v", config.RetryConfig.MaxAttempts)
	}

	if config.CircuitBreakerConfig.FailureThreshold <= 0 {
		t.Errorf("DefaultHTTPConfig() CircuitBreakerConfig.FailureThreshold should be positive, got %v", config.CircuitBreakerConfig.FailureThreshold)
	}
}
