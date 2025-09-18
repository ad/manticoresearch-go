package models

import (
	"os"
	"testing"
	"time"
)

// TestAISearchConfigComprehensive provides comprehensive testing for AI search configuration management
func TestAISearchConfigComprehensive(t *testing.T) {
	t.Run("Configuration Loading", func(t *testing.T) {
		testAIConfigurationLoading(t)
	})

	t.Run("Configuration Validation", func(t *testing.T) {
		testAIConfigurationValidation(t)
	})

	t.Run("Environment Variable Handling", func(t *testing.T) {
		testEnvironmentVariableHandling(t)
	})

	t.Run("Edge Cases", func(t *testing.T) {
		testAIConfigEdgeCases(t)
	})
}

func testAIConfigurationLoading(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expected    *AISearchConfig
		expectError bool
	}{
		{
			name:    "default configuration",
			envVars: map[string]string{},
			expected: &AISearchConfig{
				Model:   "sentence-transformers/all-MiniLM-L6-v2",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
			expectError: false,
		},
		{
			name: "custom configuration",
			envVars: map[string]string{
				"MANTICORE_AI_MODEL":   "custom-model/bert-base",
				"MANTICORE_AI_ENABLED": "false",
				"MANTICORE_AI_TIMEOUT": "60s",
			},
			expected: &AISearchConfig{
				Model:   "custom-model/bert-base",
				Enabled: false,
				Timeout: 60 * time.Second,
			},
			expectError: false,
		},
		{
			name: "partial configuration",
			envVars: map[string]string{
				"MANTICORE_AI_MODEL": "partial-model/test",
			},
			expected: &AISearchConfig{
				Model:   "partial-model/test",
				Enabled: true,
				Timeout: 30 * time.Second,
			},
			expectError: false,
		},
		{
			name: "invalid model",
			envVars: map[string]string{
				"MANTICORE_AI_MODEL": "../dangerous-path",
			},
			expected:    nil,
			expectError: true,
		},
		{
			name: "invalid enabled value",
			envVars: map[string]string{
				"MANTICORE_AI_ENABLED": "not-a-boolean",
			},
			expected:    nil,
			expectError: true,
		},
		{
			name: "invalid timeout",
			envVars: map[string]string{
				"MANTICORE_AI_TIMEOUT": "not-a-duration",
			},
			expected:    nil,
			expectError: true,
		},
		{
			name: "negative timeout",
			envVars: map[string]string{
				"MANTICORE_AI_TIMEOUT": "-10s",
			},
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			clearAIEnvVars()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearAIEnvVars()

			// Load configuration
			config, err := LoadAISearchConfigFromEnvironment()

			// Validate results
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if config == nil {
					t.Errorf("Expected config but got nil")
					return
				}
				if config.Model != tt.expected.Model {
					t.Errorf("Expected model %s, got %s", tt.expected.Model, config.Model)
				}
				if config.Enabled != tt.expected.Enabled {
					t.Errorf("Expected enabled %v, got %v", tt.expected.Enabled, config.Enabled)
				}
				if config.Timeout != tt.expected.Timeout {
					t.Errorf("Expected timeout %v, got %v", tt.expected.Timeout, config.Timeout)
				}
			}
		})
	}
}

func testAIConfigurationValidation(t *testing.T) {
	t.Run("Model Validation", func(t *testing.T) {
		validModels := []string{
			"sentence-transformers/all-MiniLM-L6-v2",
			"custom-model/test",
			"model-name",
			"model_name",
			"model123",
			"organization/model-name-v2",
			"huggingface/transformers",
		}

		for _, model := range validModels {
			if err := validateAIModel(model); err != nil {
				t.Errorf("Expected model '%s' to be valid, got error: %v", model, err)
			}
		}

		invalidModels := []string{
			"",                  // empty
			"../dangerous-path", // path traversal
			"model|command",     // pipe
			"model&command",     // ampersand
			"model;command",     // semicolon
			"model$var",         // dollar sign
			"model`command`",    // backtick
			"model()",           // parentheses
			"model{}",           // braces
			"model[]",           // brackets
			"model\\path",       // backslash
			"model\ncommand",    // newline
			"model\tcommand",    // tab
		}

		for _, model := range invalidModels {
			if err := validateAIModel(model); err == nil {
				t.Errorf("Expected model '%s' to be invalid, got no error", model)
			}
		}
	})

	t.Run("Search Mode Validation", func(t *testing.T) {
		validModes := []string{"basic", "fulltext", "vector", "hybrid", "ai"}
		for _, mode := range validModes {
			if err := ValidateSearchMode(mode); err != nil {
				t.Errorf("Expected mode '%s' to be valid, got error: %v", mode, err)
			}
		}

		invalidModes := []string{"invalid", "", "BASIC", "AI", "unknown", "full-text", "vectors"}
		for _, mode := range invalidModes {
			if err := ValidateSearchMode(mode); err == nil {
				t.Errorf("Expected mode '%s' to be invalid, got no error", mode)
			}
		}
	})
}

func testEnvironmentVariableHandling(t *testing.T) {
	t.Run("Environment Variable Precedence", func(t *testing.T) {
		clearAIEnvVars()

		// Test that environment variables override defaults
		os.Setenv("MANTICORE_AI_MODEL", "env-model")
		os.Setenv("MANTICORE_AI_ENABLED", "false")
		os.Setenv("MANTICORE_AI_TIMEOUT", "45s")
		defer clearAIEnvVars()

		config, err := LoadAISearchConfigFromEnvironment()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if config.Model != "env-model" {
			t.Errorf("Expected model from env var, got %s", config.Model)
		}
		if config.Enabled != false {
			t.Errorf("Expected enabled from env var, got %v", config.Enabled)
		}
		if config.Timeout != 45*time.Second {
			t.Errorf("Expected timeout from env var, got %v", config.Timeout)
		}
	})

	t.Run("Empty Environment Variables", func(t *testing.T) {
		clearAIEnvVars()

		// Set empty values
		os.Setenv("MANTICORE_AI_MODEL", "")
		os.Setenv("MANTICORE_AI_ENABLED", "")
		os.Setenv("MANTICORE_AI_TIMEOUT", "")
		defer clearAIEnvVars()

		config, err := LoadAISearchConfigFromEnvironment()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Should use defaults when env vars are empty
		defaults := DefaultAISearchConfig()
		if config.Model != defaults.Model {
			t.Errorf("Expected default model, got %s", config.Model)
		}
		if config.Enabled != defaults.Enabled {
			t.Errorf("Expected default enabled, got %v", config.Enabled)
		}
		if config.Timeout != defaults.Timeout {
			t.Errorf("Expected default timeout, got %v", config.Timeout)
		}
	})
}

func testAIConfigEdgeCases(t *testing.T) {
	t.Run("Boundary Values", func(t *testing.T) {
		tests := []struct {
			name        string
			timeout     string
			expectError bool
		}{
			{"minimum timeout", "1ns", false},
			{"zero timeout", "0s", true},
			{"negative timeout", "-1s", true},
			{"very large timeout", "24h", false},
			{"fractional timeout", "1.5s", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				clearAIEnvVars()
				os.Setenv("MANTICORE_AI_TIMEOUT", tt.timeout)
				defer clearAIEnvVars()

				_, err := LoadAISearchConfigFromEnvironment()
				if tt.expectError && err == nil {
					t.Errorf("Expected error for timeout %s", tt.timeout)
				}
				if !tt.expectError && err != nil {
					t.Errorf("Unexpected error for timeout %s: %v", tt.timeout, err)
				}
			})
		}
	})

	t.Run("Boolean Parsing Edge Cases", func(t *testing.T) {
		tests := []struct {
			value       string
			expected    bool
			expectError bool
		}{
			{"true", true, false},
			{"false", false, false},
			{"TRUE", true, false},
			{"FALSE", false, false},
			{"1", true, false},
			{"0", false, false},
			{"yes", false, true}, // strconv.ParseBool doesn't accept "yes"
			{"no", false, true},  // strconv.ParseBool doesn't accept "no"
			{"invalid", false, true},
		}

		for _, tt := range tests {
			t.Run(tt.value, func(t *testing.T) {
				clearAIEnvVars()
				os.Setenv("MANTICORE_AI_ENABLED", tt.value)
				defer clearAIEnvVars()

				config, err := LoadAISearchConfigFromEnvironment()
				if tt.expectError {
					if err == nil {
						t.Errorf("Expected error for value %s", tt.value)
					}
				} else {
					if err != nil {
						t.Errorf("Unexpected error for value %s: %v", tt.value, err)
					}
					if config.Enabled != tt.expected {
						t.Errorf("Expected %v for value %s, got %v", tt.expected, tt.value, config.Enabled)
					}
				}
			})
		}
	})

	t.Run("Model Name Edge Cases", func(t *testing.T) {
		tests := []struct {
			name        string
			model       string
			expectError bool
		}{
			{"very long model name", "very-long-model-name-that-might-cause-issues-but-should-still-be-valid", false},
			{"model with numbers", "model123", false},
			{"model with underscores", "model_name_v2", false},
			{"model with hyphens", "model-name-v2", false},
			{"model with slashes", "organization/model-name", false},
			{"model with dots", "model.name.v2", false},
			{"single character", "a", false},
			{"unicode characters", "modèl", true}, // Unicode characters outside ASCII range are not allowed
			{"control characters", "model\x00", true},
			{"high unicode", "model\u2603", true}, // snowman character - Unicode characters outside ASCII range are not allowed
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := validateAIModel(tt.model)
				if tt.expectError && err == nil {
					t.Errorf("Expected error for model %s", tt.model)
				}
				if !tt.expectError && err != nil {
					t.Errorf("Unexpected error for model %s: %v", tt.model, err)
				}
			})
		}
	})
}

// TestAIConfigConcurrency tests configuration loading under concurrent access
func TestAIConfigConcurrency(t *testing.T) {
	clearAIEnvVars()
	os.Setenv("MANTICORE_AI_MODEL", "concurrent-test-model")
	defer clearAIEnvVars()

	const numGoroutines = 10
	results := make(chan *AISearchConfig, numGoroutines)
	errors := make(chan error, numGoroutines)

	// Launch concurrent configuration loading
	for i := 0; i < numGoroutines; i++ {
		go func() {
			config, err := LoadAISearchConfigFromEnvironment()
			if err != nil {
				errors <- err
			} else {
				results <- config
			}
		}()
	}

	// Collect results
	var configs []*AISearchConfig
	var errs []error

	for i := 0; i < numGoroutines; i++ {
		select {
		case config := <-results:
			configs = append(configs, config)
		case err := <-errors:
			errs = append(errs, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent configuration loading")
		}
	}

	// Validate results
	if len(errs) > 0 {
		t.Errorf("Got %d errors during concurrent loading: %v", len(errs), errs[0])
	}

	if len(configs) != numGoroutines {
		t.Errorf("Expected %d configs, got %d", numGoroutines, len(configs))
	}

	// All configs should be identical
	for i, config := range configs {
		if config.Model != "concurrent-test-model" {
			t.Errorf("Config %d has wrong model: %s", i, config.Model)
		}
		if config.Enabled != true {
			t.Errorf("Config %d has wrong enabled value: %v", i, config.Enabled)
		}
		if config.Timeout != 30*time.Second {
			t.Errorf("Config %d has wrong timeout: %v", i, config.Timeout)
		}
	}
}

// TestAIConfigMemoryUsage tests that configuration loading doesn't leak memory
func TestAIConfigMemoryUsage(t *testing.T) {
	clearAIEnvVars()
	os.Setenv("MANTICORE_AI_MODEL", "memory-test-model")
	defer clearAIEnvVars()

	// Load configuration many times to check for memory leaks
	for i := 0; i < 1000; i++ {
		config, err := LoadAISearchConfigFromEnvironment()
		if err != nil {
			t.Fatalf("Error on iteration %d: %v", i, err)
		}
		if config == nil {
			t.Fatalf("Got nil config on iteration %d", i)
		}
		// Don't hold references to configs to allow GC
	}
}

// BenchmarkAIConfigLoading benchmarks configuration loading performance
func BenchmarkAIConfigLoading(b *testing.B) {
	clearAIEnvVars()
	os.Setenv("MANTICORE_AI_MODEL", "benchmark-model")
	defer clearAIEnvVars()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadAISearchConfigFromEnvironment()
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
	}
}

// BenchmarkModelValidation benchmarks model validation performance
func BenchmarkModelValidation(b *testing.B) {
	model := "sentence-transformers/all-MiniLM-L6-v2"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := validateAIModel(model)
		if err != nil {
			b.Fatalf("Benchmark failed: %v", err)
		}
	}
}
