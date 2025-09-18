package models

import (
	"os"
	"testing"
	"time"
)

func TestLoadAISearchConfigFromEnvironment_Defaults(t *testing.T) {
	// Clear environment variables
	clearAIEnvVars()

	config, err := LoadAISearchConfigFromEnvironment()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := DefaultAISearchConfig()
	if config.Model != expected.Model {
		t.Errorf("Expected model %s, got %s", expected.Model, config.Model)
	}
	if config.Enabled != expected.Enabled {
		t.Errorf("Expected enabled %v, got %v", expected.Enabled, config.Enabled)
	}
	if config.Timeout != expected.Timeout {
		t.Errorf("Expected timeout %v, got %v", expected.Timeout, config.Timeout)
	}
}

func TestLoadAISearchConfigFromEnvironment_CustomValues(t *testing.T) {
	// Clear environment variables first
	clearAIEnvVars()

	// Set custom environment variables
	os.Setenv("MANTICORE_AI_MODEL", "custom-model/test")
	os.Setenv("MANTICORE_AI_ENABLED", "false")
	os.Setenv("MANTICORE_AI_TIMEOUT", "45s")
	defer clearAIEnvVars()

	config, err := LoadAISearchConfigFromEnvironment()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if config.Model != "custom-model/test" {
		t.Errorf("Expected model 'custom-model/test', got %s", config.Model)
	}
	if config.Enabled != false {
		t.Errorf("Expected enabled false, got %v", config.Enabled)
	}
	if config.Timeout != 45*time.Second {
		t.Errorf("Expected timeout 45s, got %v", config.Timeout)
	}
}

func TestLoadAISearchConfigFromEnvironment_InvalidModel(t *testing.T) {
	clearAIEnvVars()
	os.Setenv("MANTICORE_AI_MODEL", "../dangerous-path")
	defer clearAIEnvVars()

	_, err := LoadAISearchConfigFromEnvironment()
	if err == nil {
		t.Fatal("Expected error for invalid model, got nil")
	}
}

func TestLoadAISearchConfigFromEnvironment_InvalidEnabled(t *testing.T) {
	clearAIEnvVars()
	os.Setenv("MANTICORE_AI_ENABLED", "invalid-bool")
	defer clearAIEnvVars()

	_, err := LoadAISearchConfigFromEnvironment()
	if err == nil {
		t.Fatal("Expected error for invalid enabled value, got nil")
	}
}

func TestLoadAISearchConfigFromEnvironment_InvalidTimeout(t *testing.T) {
	clearAIEnvVars()
	os.Setenv("MANTICORE_AI_TIMEOUT", "invalid-duration")
	defer clearAIEnvVars()

	_, err := LoadAISearchConfigFromEnvironment()
	if err == nil {
		t.Fatal("Expected error for invalid timeout, got nil")
	}
}

func TestLoadAISearchConfigFromEnvironment_NegativeTimeout(t *testing.T) {
	clearAIEnvVars()
	os.Setenv("MANTICORE_AI_TIMEOUT", "-10s")
	defer clearAIEnvVars()

	_, err := LoadAISearchConfigFromEnvironment()
	if err == nil {
		t.Fatal("Expected error for negative timeout, got nil")
	}
}

func TestDefaultAISearchConfig(t *testing.T) {
	config := DefaultAISearchConfig()

	if config.Model != "sentence-transformers/all-MiniLM-L6-v2" {
		t.Errorf("Expected default model 'sentence-transformers/all-MiniLM-L6-v2', got %s", config.Model)
	}
	if config.Enabled != true {
		t.Errorf("Expected default enabled true, got %v", config.Enabled)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", config.Timeout)
	}
}

func TestValidateAIModel_Valid(t *testing.T) {
	validModels := []string{
		"sentence-transformers/all-MiniLM-L6-v2",
		"custom-model/test",
		"model-name",
		"model_name",
		"model123",
	}

	for _, model := range validModels {
		if err := validateAIModel(model); err != nil {
			t.Errorf("Expected model '%s' to be valid, got error: %v", model, err)
		}
	}
}

func TestValidateAIModel_Invalid(t *testing.T) {
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
	}

	for _, model := range invalidModels {
		if err := validateAIModel(model); err == nil {
			t.Errorf("Expected model '%s' to be invalid, got no error", model)
		}
	}
}

func TestValidateSearchMode_Valid(t *testing.T) {
	validModes := []string{
		"basic",
		"fulltext",
		"vector",
		"hybrid",
		"ai",
	}

	for _, mode := range validModes {
		if err := ValidateSearchMode(mode); err != nil {
			t.Errorf("Expected mode '%s' to be valid, got error: %v", mode, err)
		}
	}
}

func TestValidateSearchMode_Invalid(t *testing.T) {
	invalidModes := []string{
		"invalid",
		"",
		"BASIC",
		"AI",
		"unknown",
	}

	for _, mode := range invalidModes {
		if err := ValidateSearchMode(mode); err == nil {
			t.Errorf("Expected mode '%s' to be invalid, got no error", mode)
		}
	}
}

// Helper function to clear AI-related environment variables
func clearAIEnvVars() {
	os.Unsetenv("MANTICORE_AI_MODEL")
	os.Unsetenv("MANTICORE_AI_ENABLED")
	os.Unsetenv("MANTICORE_AI_TIMEOUT")
}
