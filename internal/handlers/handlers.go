package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ad/manticoresearch-go/internal/document"
	"github.com/ad/manticoresearch-go/internal/manticore"
	"github.com/ad/manticoresearch-go/internal/models"
	"github.com/ad/manticoresearch-go/internal/search"
	"github.com/ad/manticoresearch-go/internal/vectorizer"
	"github.com/ad/manticoresearch-go/pkg/api"
)

// AppState holds the application state including loaded documents and services
type AppState struct {
	Documents  []*models.Document
	Vectorizer *vectorizer.TFIDFVectorizer
	Manticore  manticore.ClientInterface // Client interface for both official and HTTP clients
	Vectors    [][]float64
	AIConfig   *models.AISearchConfig
}

// NewAppState creates a new application state
func NewAppState() *AppState {
	// Load AI configuration
	aiConfig, err := models.LoadAISearchConfigFromEnvironment()
	if err != nil {
		log.Printf("Warning: Failed to load AI search configuration: %v", err)
		log.Println("Falling back to default AI search configuration")
		aiConfig = models.DefaultAISearchConfig()
	}

	return NewAppStateWithConfig(aiConfig)
}

// NewAppStateWithConfig creates a new application state with the provided AI configuration
func NewAppStateWithConfig(aiConfig *models.AISearchConfig) *AppState {
	return &AppState{
		Documents:  make([]*models.Document, 0),
		Vectorizer: nil,
		Manticore:  nil,
		Vectors:    make([][]float64, 0),
		AIConfig:   aiConfig,
	}
}

// SearchHandler handles GET /api/search requests
func (app *AppState) SearchHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow GET requests
	if r.Method != "GET" {
		app.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse query parameters
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		app.sendErrorResponse(w, http.StatusBadRequest, "Query parameter is required")
		return
	}

	// Parse search mode
	modeStr := strings.TrimSpace(r.URL.Query().Get("mode"))
	if modeStr == "" {
		modeStr = "basic" // Default to basic search
	}

	mode, err := search.ValidateSearchMode(modeStr)
	if err != nil {
		app.sendErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// Parse pagination parameters
	page, err := parseIntParam(r.URL.Query().Get("page"), 1)
	if err != nil || page < 1 {
		app.sendErrorResponse(w, http.StatusBadRequest, "Invalid page parameter")
		return
	}

	limit, err := parseIntParam(r.URL.Query().Get("limit"), 10)
	if err != nil || limit < 1 || limit > 100 {
		app.sendErrorResponse(w, http.StatusBadRequest, "Invalid limit parameter (must be between 1 and 100)")
		return
	}

	// Handle AI search mode with graceful degradation
	originalMode := mode
	if mode == models.SearchModeAI {
		if err := app.validateAISearchAvailability(); err != nil {
			log.Printf("AI search not available: %v, degrading to hybrid search", err)
			// Log AI search fallback for monitoring
			app.logAISearchOperation("AI_SEARCH_DEGRADATION", time.Duration(0), false, map[string]interface{}{
				"query":              query,
				"original_mode":      string(originalMode),
				"fallback_mode":      string(models.SearchModeHybrid),
				"degradation_reason": err.Error(),
			})
			mode = models.SearchModeHybrid // Graceful degradation
		}
	}

	// Perform search using official client
	var result *models.SearchResponse
	searchStartTime := time.Now()

	if app.Manticore != nil {
		// Use search engine with official client
		searchEngine := search.NewSearchEngine(app.Manticore, app.Vectorizer, app.AIConfig)
		result, err = searchEngine.Search(query, mode, page, limit)
		searchDuration := time.Since(searchStartTime)

		if err != nil {
			log.Printf("Search error (mode: %s): %v", mode, err)

			// Handle AI search specific errors with fallback
			if originalMode == models.SearchModeAI {
				log.Printf("AI search failed, attempting fallback to vector search")

				// Log AI search failure for monitoring
				app.logAISearchOperation("AI_SEARCH_FAILURE", searchDuration, false, map[string]interface{}{
					"query": query,
					"model": app.getAIModel(),
					"error": err.Error(),
					"page":  page,
					"limit": limit,
				})

				fallbackStartTime := time.Now()
				fallbackResult, fallbackErr := searchEngine.Search(query, models.SearchModeVector, page, limit)
				fallbackDuration := time.Since(fallbackStartTime)

				if fallbackErr != nil {
					log.Printf("Fallback search also failed: %v", fallbackErr)

					// Log complete failure for monitoring
					app.logAISearchOperation("AI_SEARCH_COMPLETE_FAILURE", searchDuration+fallbackDuration, false, map[string]interface{}{
						"query":          query,
						"ai_error":       err.Error(),
						"fallback_error": fallbackErr.Error(),
						"total_duration": searchDuration + fallbackDuration,
					})

					app.sendAISearchErrorResponse(w, err, fallbackErr)
					return
				}

				// Log successful fallback for monitoring
				app.logAISearchOperation("AI_SEARCH_FALLBACK_SUCCESS", searchDuration+fallbackDuration, true, map[string]interface{}{
					"query":            query,
					"fallback_mode":    string(models.SearchModeVector),
					"fallback_results": len(fallbackResult.Documents),
					"ai_error":         err.Error(),
					"total_duration":   searchDuration + fallbackDuration,
				})

				// Add fallback metadata to response
				result = app.addAISearchFallbackMetadata(fallbackResult, err.Error())
			} else {
				app.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Search failed: %v", err))
				return
			}
		} else {
			// Log successful search operation
			if originalMode == models.SearchModeAI {
				app.logAISearchOperation("AI_SEARCH_SUCCESS", searchDuration, true, map[string]interface{}{
					"query":   query,
					"model":   app.getAIModel(),
					"results": len(result.Documents),
					"total":   result.Total,
					"page":    page,
					"limit":   limit,
				})
			}
		}
	} else {
		// No Manticore client available
		if originalMode == models.SearchModeAI {
			app.logAISearchOperation("AI_SEARCH_UNAVAILABLE", time.Duration(0), false, map[string]interface{}{
				"query":  query,
				"reason": "Manticore Search service is not available",
			})
			app.sendAISearchUnavailableResponse(w, "Manticore Search service is not available")
		} else {
			app.sendErrorResponse(w, http.StatusServiceUnavailable, "Search service is not available")
		}
		return
	}

	// Add AI search metadata to response if applicable
	if originalMode == models.SearchModeAI {
		result = app.addAISearchMetadata(result, originalMode != mode)
	}

	// Send successful response
	app.sendSuccessResponse(w, result)
}

// StatusHandler handles GET /api/status requests
func (app *AppState) StatusHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow GET requests
	if r.Method != "GET" {
		app.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check Manticore health
	manticoreHealthy := false
	if app.Manticore != nil && app.Manticore.IsConnected() {
		if err := app.Manticore.HealthCheck(); err == nil {
			manticoreHealthy = true
		}
	}

	// Check AI search health with detailed logging
	healthCheckStartTime := time.Now()
	aiSearchHealthy := app.checkAISearchHealth()
	healthCheckDuration := time.Since(healthCheckStartTime)

	aiSearchEnabled := app.AIConfig != nil && app.AIConfig.Enabled
	aiModel := ""
	if app.AIConfig != nil {
		aiModel = app.AIConfig.Model
	}

	// Log AI search health check results for monitoring
	app.logAISearchOperation("AI_SEARCH_HEALTH_CHECK", healthCheckDuration, aiSearchHealthy, map[string]interface{}{
		"ai_enabled":        aiSearchEnabled,
		"ai_model":          aiModel,
		"manticore_healthy": manticoreHealthy,
	})

	// Prepare status response
	status := api.StatusResponse{
		Status:           "ok",
		ManticoreHealthy: manticoreHealthy,
		DocumentsLoaded:  len(app.Documents),
		VectorizerReady:  app.Vectorizer != nil,
		AISearchEnabled:  aiSearchEnabled,
		AIModel:          aiModel,
		AISearchHealthy:  aiSearchHealthy,
	}

	// Send response
	app.sendSuccessResponse(w, status)
}

// ReindexHandler handles POST /api/reindex requests
func (app *AppState) ReindexHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow POST requests
	if r.Method != "POST" {
		app.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check if Manticore is available
	if app.Manticore == nil || !app.Manticore.IsConnected() {
		app.sendErrorResponse(w, http.StatusServiceUnavailable, "Manticore Search is not available")
		return
	}

	// Perform reindexing
	startTime := time.Now()
	log.Println("Manual reindexing requested")

	// Load documents from data directory
	dataDir := getDataDirectory()
	documents, err := document.ScanDataDirectory(dataDir)
	if err != nil {
		log.Printf("Failed to scan data directory: %v", err)
		app.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load documents: %v", err))
		return
	}

	if len(documents) == 0 {
		app.sendErrorResponse(w, http.StatusBadRequest, "No documents found in data directory")
		return
	}

	// Create and train vectorizer
	vec := vectorizer.NewTFIDFVectorizer()
	vectors := vec.FitTransform(documents)

	// Reset and recreate database schema with AI configuration from app state
	if err := app.Manticore.CreateSchema(app.AIConfig); err != nil {
		log.Printf("Failed to create schema: %v", err)
		app.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create database schema: %v", err))
		return
	}

	// Index documents
	if err := app.Manticore.IndexDocuments(documents, vectors); err != nil {
		log.Printf("Failed to index documents: %v", err)
		app.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to index documents: %v", err))
		return
	}

	// Update application state
	app.Documents = documents
	app.Vectorizer = vec
	app.Vectors = vectors

	indexingDuration := time.Since(startTime)
	log.Printf("Manual reindexing completed: %d documents indexed in %v", len(documents), indexingDuration)

	// Prepare response
	response := api.ReindexResponse{
		Message:        "Reindexing completed successfully",
		DocumentsCount: len(documents),
		IndexingTime:   indexingDuration.String(),
	}

	app.sendSuccessResponse(w, response)
}

// sendSuccessResponse sends a successful JSON response
func (app *AppState) sendSuccessResponse(w http.ResponseWriter, data interface{}) {
	response := api.APIResponse{
		Success: true,
		Data:    data,
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// sendErrorResponse sends an error JSON response
func (app *AppState) sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := api.APIResponse{
		Success: false,
		Error:   message,
	}

	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode JSON error response: %v", err)
	}
}

// parseIntParam parses an integer parameter with a default value
func parseIntParam(param string, defaultValue int) (int, error) {
	if param == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(param)
}

// getDataDirectory returns the data directory path from environment or default
func getDataDirectory() string {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	return dataDir
}

// validateAISearchAvailability validates if AI search is available and properly configured
func (app *AppState) validateAISearchAvailability() error {
	// Check if AI configuration is available
	if app.AIConfig == nil {
		return fmt.Errorf("AI search configuration is not loaded")
	}

	// Check if AI search is enabled
	if !app.AIConfig.Enabled {
		return fmt.Errorf("AI search is disabled in configuration")
	}

	// Check if Manticore client is available
	if app.Manticore == nil {
		return fmt.Errorf("Manticore search client is not available")
	}

	// Check if Manticore is connected
	if !app.Manticore.IsConnected() {
		return fmt.Errorf("Manticore search client is not connected")
	}

	return nil
}

// addAISearchMetadata adds AI search specific metadata to the search response
func (app *AppState) addAISearchMetadata(response *models.SearchResponse, fallbackUsed bool) *models.SearchResponse {
	if response == nil {
		return response
	}

	// Update mode to indicate AI search was used or fallback occurred
	if fallbackUsed {
		response.Mode = "hybrid (AI degraded)"
	} else {
		response.Mode = string(models.SearchModeAI)
	}

	// Log AI search metadata for monitoring
	if app.AIConfig != nil {
		if fallbackUsed {
			log.Printf("AI search degraded to hybrid mode, using model: %s", app.AIConfig.Model)
		} else {
			log.Printf("AI search completed successfully using model: %s", app.AIConfig.Model)
		}
	}

	return response
}

// addAISearchFallbackMetadata adds fallback metadata when AI search fails
func (app *AppState) addAISearchFallbackMetadata(response *models.SearchResponse, fallbackReason string) *models.SearchResponse {
	if response == nil {
		return response
	}

	// Update mode to indicate fallback was used
	response.Mode = "hybrid (AI fallback)"

	// Log fallback with detailed information for monitoring
	log.Printf("AI search fallback activated: %s", fallbackReason)
	log.Printf("AI search fallback results: %d documents returned via hybrid search", len(response.Documents))

	return response
}

// sendAISearchUnavailableResponse sends a response when AI search is completely unavailable
func (app *AppState) sendAISearchUnavailableResponse(w http.ResponseWriter, reason string) {
	log.Printf("AI search unavailable: %s", reason)

	response := api.APIResponse{
		Success: false,
		Error:   fmt.Sprintf("AI search is currently unavailable: %s. Please try hybrid or fulltext search instead.", reason),
		Data: map[string]interface{}{
			"error_type":      "ai_search_unavailable",
			"reason":          reason,
			"suggested_modes": []string{"hybrid", "fulltext", "vector"},
			"ai_enabled":      app.AIConfig != nil && app.AIConfig.Enabled,
		},
	}

	w.WriteHeader(http.StatusServiceUnavailable)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode AI search unavailable response: %v", err)
	}
}

// sendAISearchErrorResponse sends a specialized error response for AI search failures
func (app *AppState) sendAISearchErrorResponse(w http.ResponseWriter, aiError, fallbackError error) {
	errorMsg := fmt.Sprintf("AI search failed: %v", aiError)
	if fallbackError != nil {
		errorMsg += fmt.Sprintf(". Fallback search also failed: %v", fallbackError)
	}

	log.Printf("AI search complete failure: AI error: %v, Fallback error: %v", aiError, fallbackError)

	// Determine error category for better user feedback
	errorCategory := app.categorizeAISearchError(aiError)

	response := api.APIResponse{
		Success: false,
		Error:   errorMsg,
		Data: map[string]interface{}{
			"error_type":      "ai_search_failure",
			"error_category":  errorCategory,
			"ai_error":        aiError.Error(),
			"fallback_error":  fallbackError.Error(),
			"suggested_modes": []string{"hybrid", "fulltext"},
			"retry_suggested": errorCategory == "timeout" || errorCategory == "network",
		},
	}

	w.WriteHeader(http.StatusInternalServerError)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode AI search error response: %v", err)
	}
}

// categorizeAISearchError categorizes AI search errors for better user feedback
func (app *AppState) categorizeAISearchError(err error) string {
	if err == nil {
		return "unknown"
	}

	errorStr := strings.ToLower(err.Error())

	if strings.Contains(errorStr, "timeout") || strings.Contains(errorStr, "deadline exceeded") {
		return "timeout"
	}
	if strings.Contains(errorStr, "connection") || strings.Contains(errorStr, "network") {
		return "network"
	}
	if strings.Contains(errorStr, "embedding") {
		return "embedding"
	}
	if strings.Contains(errorStr, "model") {
		return "model"
	}
	if strings.Contains(errorStr, "http 4") {
		return "client_error"
	}
	if strings.Contains(errorStr, "http 5") {
		return "server_error"
	}

	return "unknown"
}

// checkAISearchHealth performs a health check for AI search functionality
func (app *AppState) checkAISearchHealth() bool {
	log.Printf("[AI_SEARCH] [HEALTH_CHECK] Starting AI search health check")

	// Check if AI configuration is available and enabled
	if app.AIConfig == nil {
		log.Printf("[AI_SEARCH] [HEALTH_CHECK] AI configuration is not available")
		return false
	}

	if !app.AIConfig.Enabled {
		log.Printf("[AI_SEARCH] [HEALTH_CHECK] AI search is disabled in configuration")
		return false
	}

	log.Printf("[AI_SEARCH] [HEALTH_CHECK] AI configuration valid - Model: %s, Timeout: %v",
		app.AIConfig.Model, app.AIConfig.Timeout)

	// Check if Manticore client is available and connected
	if app.Manticore == nil {
		log.Printf("[AI_SEARCH] [HEALTH_CHECK] Manticore client is not available")
		return false
	}

	if !app.Manticore.IsConnected() {
		log.Printf("[AI_SEARCH] [HEALTH_CHECK] Manticore client is not connected")
		return false
	}

	log.Printf("[AI_SEARCH] [HEALTH_CHECK] Manticore client is available and connected")

	// Perform a basic health check by validating the configuration
	if err := app.validateAISearchAvailability(); err != nil {
		log.Printf("[AI_SEARCH] [HEALTH_CHECK] AI search availability validation failed: %v", err)
		return false
	}

	log.Printf("[AI_SEARCH] [HEALTH_CHECK] AI search health check passed successfully")

	// Additional health checks could be added here, such as:
	// - Testing AI model availability
	// - Checking embedding generation capability
	// - Validating AI search index readiness

	return true
}

// logAISearchOperation logs AI search operations for monitoring and debugging
func (app *AppState) logAISearchOperation(operation string, duration time.Duration, success bool, details map[string]interface{}) {
	logLevel := "INFO"
	if !success {
		logLevel = "ERROR"
	}

	log.Printf("[AI_SEARCH] [%s] %s completed in %v - Success: %t", logLevel, operation, duration, success)

	for key, value := range details {
		log.Printf("[AI_SEARCH] [%s] %s: %v", logLevel, key, value)
	}

	// Additional monitoring could be added here:
	// - Send metrics to monitoring system
	// - Update health check status
	// - Track error rates and patterns
}

// getAIModel returns the currently configured AI model
func (app *AppState) getAIModel() string {
	if app.AIConfig != nil && app.AIConfig.Model != "" {
		return app.AIConfig.Model
	}
	return "sentence-transformers/all-MiniLM-L6-v2" // Default model
}
