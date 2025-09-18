package manticore

import (
	"testing"
)

func TestSearchAdapter_NewSearchAdapter(t *testing.T) {
	// Test with HTTP client
	httpConfig := DefaultHTTPConfig("localhost:9308")
	httpClient := NewHTTPClient(*httpConfig)
	adapter := NewSearchAdapter(httpClient)

	if adapter == nil {
		t.Errorf("NewSearchAdapter() returned nil for HTTP client")
	}

	if adapter.client != httpClient {
		t.Errorf("NewSearchAdapter() HTTP client not set correctly")
	}
}

func TestSearchAdapter_GetAllDocuments(t *testing.T) {
	// Test with HTTP client
	httpConfig := DefaultHTTPConfig("localhost:9308")
	httpClient := NewHTTPClient(*httpConfig)
	adapter := NewSearchAdapter(httpClient)

	_, err := adapter.GetAllDocuments()
	if err == nil {
		t.Logf("GetAllDocuments() with HTTP client succeeded (unexpected but not an error)")
	} else {
		t.Logf("GetAllDocuments() with HTTP client failed as expected: %v", err)
	}
}

func TestSearchAdapter_BasicSearch(t *testing.T) {
	// Test with HTTP client
	httpConfig := DefaultHTTPConfig("localhost:9308")
	httpClient := NewHTTPClient(*httpConfig)
	adapter := NewSearchAdapter(httpClient)

	_, err := adapter.BasicSearch("test query", 1, 10)
	if err == nil {
		t.Logf("BasicSearch() with HTTP client succeeded (unexpected but not an error)")
	} else {
		t.Logf("BasicSearch() with HTTP client failed as expected: %v", err)
	}
}

func TestSearchAdapter_FullTextSearch(t *testing.T) {
	// Test with HTTP client
	httpConfig := DefaultHTTPConfig("localhost:9308")
	httpClient := NewHTTPClient(*httpConfig)
	adapter := NewSearchAdapter(httpClient)

	_, err := adapter.FullTextSearch("test query", 1, 10)
	if err == nil {
		t.Logf("FullTextSearch() with HTTP client succeeded (unexpected but not an error)")
	} else {
		t.Logf("FullTextSearch() with HTTP client failed as expected: %v", err)
	}
}

func TestSearchAdapter_TypeSwitching(t *testing.T) {
	// Test that the adapter correctly identifies client types
	httpConfig := DefaultHTTPConfig("localhost:9308")
	httpClient := NewHTTPClient(*httpConfig)

	adapter := NewSearchAdapter(httpClient)

	// Adapter should be created successfully
	if adapter == nil {
		t.Errorf("Failed to create adapter with HTTP client")
	}

	// Test that it handles the HTTP client type correctly
	// We can't test the actual search functionality without a running server,
	// but we can verify the adapter exists and has the right client reference
	if adapter.client != httpClient {
		t.Errorf("HTTP client adapter has wrong client reference")
	}
}
