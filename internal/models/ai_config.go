package models

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// LoadAISearchConfigFromEnvironment loads AI search configuration from environment variables
func LoadAISearchConfigFromEnvironment() (*AISearchConfig, error) {
	config := DefaultAISearchConfig()

	// Parse AI model configuration - support both environment variables
	model := os.Getenv("AI_SEARCH_MODEL")
	if model == "" {
		model = os.Getenv("MANTICORE_AI_MODEL")
	}
	if model != "" {
		if err := validateAIModel(model); err != nil {
			return nil, fmt.Errorf("invalid AI model configuration: %w", err)
		}
		config.Model = model
	}

	// Parse AI enabled configuration
	if enabledStr := os.Getenv("MANTICORE_AI_ENABLED"); enabledStr != "" {
		enabled, err := strconv.ParseBool(enabledStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_AI_ENABLED: %w", err)
		}
		config.Enabled = enabled
	}

	// Parse AI timeout configuration
	if timeoutStr := os.Getenv("MANTICORE_AI_TIMEOUT"); timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid MANTICORE_AI_TIMEOUT: %w", err)
		}
		if timeout <= 0 {
			return nil, fmt.Errorf("MANTICORE_AI_TIMEOUT must be positive, got: %v", timeout)
		}
		config.Timeout = timeout
	}

	return config, nil
}

// DefaultAISearchConfig returns default AI search configuration
func DefaultAISearchConfig() *AISearchConfig {
	return &AISearchConfig{
		Model:   "sentence-transformers/all-MiniLM-L6-v2",
		Enabled: true,
		Timeout: 30 * time.Second,
	}
}

// validateAIModel validates the AI model name
func validateAIModel(model string) error {
	if model == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	// Basic validation - model name should not contain dangerous characters
	for _, char := range model {
		if char < 32 || char > 126 {
			return fmt.Errorf("model name contains invalid characters")
		}
	}

	// Check for common injection patterns
	dangerousPatterns := []string{
		"../", "./", "\\", "|", "&", ";", "$", "`", "(", ")", "{", "}", "[", "]",
	}

	for _, pattern := range dangerousPatterns {
		if containsPattern(model, pattern) {
			return fmt.Errorf("model name contains potentially dangerous pattern: %s", pattern)
		}
	}

	return nil
}

// containsPattern checks if a string contains a specific pattern
func containsPattern(s, pattern string) bool {
	for i := 0; i <= len(s)-len(pattern); i++ {
		if s[i:i+len(pattern)] == pattern {
			return true
		}
	}
	return false
}

// ValidateSearchMode validates if the provided search mode is supported
func ValidateSearchMode(mode string) error {
	switch SearchMode(mode) {
	case SearchModeBasic, SearchModeFullText, SearchModeVector, SearchModeHybrid, SearchModeAI:
		return nil
	default:
		return fmt.Errorf("unsupported search mode: %s", mode)
	}
}
