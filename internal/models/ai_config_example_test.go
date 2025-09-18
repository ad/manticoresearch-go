package models

import (
	"os"
	"testing"
	"time"
)

func TestInitializeAISearchConfig_Success(t *testing.T) {
	clearAIEnvVars()
	defer clearAIEnvVars()

	config, err := InitializeAISearchConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if config == nil {
		t.Fatal("Expected config to be non-nil")
	}

	// Should use default values
	expected := DefaultAISearchConfig()
	if config.Model != expected.Model {
		t.Errorf("Expected model %s, got %s", expected.Model, config.Model)
	}
}

func TestInitializeAISearchConfig_WithEnvironmentVars(t *testing.T) {
	clearAIEnvVars()
	os.Setenv("MANTICORE_AI_MODEL", "custom-model")
	os.Setenv("MANTICORE_AI_ENABLED", "false")
	os.Setenv("MANTICORE_AI_TIMEOUT", "60s")
	defer clearAIEnvVars()

	config, err := InitializeAISearchConfig()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if config.Model != "custom-model" {
		t.Errorf("Expected model 'custom-model', got %s", config.Model)
	}
	if config.Enabled != false {
		t.Errorf("Expected enabled false, got %v", config.Enabled)
	}
	if config.Timeout != 60*time.Second {
		t.Errorf("Expected timeout 60s, got %v", config.Timeout)
	}
}

func TestInitializeAISearchConfig_FallbackOnError(t *testing.T) {
	clearAIEnvVars()
	os.Setenv("MANTICORE_AI_MODEL", "../invalid-model")
	defer clearAIEnvVars()

	// Should fall back to default config when environment config is invalid
	config, err := InitializeAISearchConfig()
	if err != nil {
		t.Fatalf("Expected no error (should fallback), got: %v", err)
	}

	// Should use default model due to fallback
	expected := DefaultAISearchConfig()
	if config.Model != expected.Model {
		t.Errorf("Expected fallback to default model %s, got %s", expected.Model, config.Model)
	}
}

func TestValidateAISearchConfig_Valid(t *testing.T) {
	config := &AISearchConfig{
		Model:   "valid-model",
		Enabled: true,
		Timeout: 30 * time.Second,
	}

	if err := validateAISearchConfig(config); err != nil {
		t.Errorf("Expected valid config to pass validation, got: %v", err)
	}
}

func TestValidateAISearchConfig_Nil(t *testing.T) {
	if err := validateAISearchConfig(nil); err == nil {
		t.Error("Expected error for nil config, got nil")
	}
}

func TestValidateAISearchConfig_InvalidModel(t *testing.T) {
	config := &AISearchConfig{
		Model:   "../invalid-model",
		Enabled: true,
		Timeout: 30 * time.Second,
	}

	if err := validateAISearchConfig(config); err == nil {
		t.Error("Expected error for invalid model, got nil")
	}
}

func TestValidateAISearchConfig_InvalidTimeout(t *testing.T) {
	config := &AISearchConfig{
		Model:   "valid-model",
		Enabled: true,
		Timeout: -10 * time.Second,
	}

	if err := validateAISearchConfig(config); err == nil {
		t.Error("Expected error for negative timeout, got nil")
	}
}

func TestGetAISearchStatus_ValidConfig(t *testing.T) {
	config := &AISearchConfig{
		Model:   "test-model",
		Enabled: true,
		Timeout: 45 * time.Second,
	}

	status := GetAISearchStatus(config)

	if status["ai_search_enabled"] != true {
		t.Errorf("Expected ai_search_enabled true, got %v", status["ai_search_enabled"])
	}
	if status["ai_model"] != "test-model" {
		t.Errorf("Expected ai_model 'test-model', got %v", status["ai_model"])
	}
	if status["ai_search_healthy"] != true {
		t.Errorf("Expected ai_search_healthy true, got %v", status["ai_search_healthy"])
	}
	if status["ai_timeout"] != "45s" {
		t.Errorf("Expected ai_timeout '45s', got %v", status["ai_timeout"])
	}
}

func TestGetAISearchStatus_NilConfig(t *testing.T) {
	status := GetAISearchStatus(nil)

	if status["ai_search_enabled"] != false {
		t.Errorf("Expected ai_search_enabled false, got %v", status["ai_search_enabled"])
	}
	if status["ai_model"] != "" {
		t.Errorf("Expected ai_model empty, got %v", status["ai_model"])
	}
	if status["ai_search_healthy"] != false {
		t.Errorf("Expected ai_search_healthy false, got %v", status["ai_search_healthy"])
	}
	if status["error"] == nil {
		t.Error("Expected error message for nil config")
	}
}
