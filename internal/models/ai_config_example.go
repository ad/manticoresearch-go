package models

import (
	"fmt"
	"log"
)

// InitializeAISearchConfig demonstrates how to initialize AI search configuration
// This function shows the proper way to load and validate AI search configuration
func InitializeAISearchConfig() (*AISearchConfig, error) {
	// Load configuration from environment variables
	config, err := LoadAISearchConfigFromEnvironment()
	if err != nil {
		// Log the error and fall back to default configuration
		log.Printf("Warning: Failed to load AI search configuration: %v", err)
		log.Println("Falling back to default AI search configuration")
		config = DefaultAISearchConfig()
	}

	// Log the configuration being used
	log.Printf("AI Search Configuration:")
	log.Printf("  Model: %s", config.Model)
	log.Printf("  Enabled: %v", config.Enabled)
	log.Printf("  Timeout: %v", config.Timeout)

	// Validate the final configuration
	if err := validateAISearchConfig(config); err != nil {
		return nil, fmt.Errorf("invalid AI search configuration: %w", err)
	}

	return config, nil
}

// validateAISearchConfig performs comprehensive validation of AI search configuration
func validateAISearchConfig(config *AISearchConfig) error {
	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	// Validate model
	if err := validateAIModel(config.Model); err != nil {
		return fmt.Errorf("invalid model: %w", err)
	}

	// Validate timeout
	if config.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got: %v", config.Timeout)
	}

	return nil
}

// GetAISearchStatus returns the current status of AI search configuration
func GetAISearchStatus(config *AISearchConfig) map[string]interface{} {
	if config == nil {
		return map[string]interface{}{
			"ai_search_enabled": false,
			"ai_model":          "",
			"ai_search_healthy": false,
			"error":             "AI search configuration not initialized",
		}
	}

	return map[string]interface{}{
		"ai_search_enabled": config.Enabled,
		"ai_model":          config.Model,
		"ai_search_healthy": config.Enabled, // In a real implementation, this would check actual AI service health
		"ai_timeout":        config.Timeout.String(),
	}
}
