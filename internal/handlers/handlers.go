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
}

// NewAppState creates a new application state
func NewAppState() *AppState {
	return &AppState{
		Documents:  make([]*models.Document, 0),
		Vectorizer: nil,
		Manticore:  nil,
		Vectors:    make([][]float64, 0),
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

	// Perform search using official client
	var result *models.SearchResponse
	if app.Manticore != nil {
		// Use search engine with official client
		searchEngine := search.NewSearchEngine(app.Manticore, app.Vectorizer)
		result, err = searchEngine.Search(query, mode, page, limit)
		if err != nil {
			log.Printf("Search error: %v", err)
			app.sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Search failed: %v", err))
			return
		}
	} else {
		// No Manticore client available
		app.sendErrorResponse(w, http.StatusServiceUnavailable, "Search service is not available")
		return
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

	// Prepare status response
	status := api.StatusResponse{
		Status:           "ok",
		ManticoreHealthy: manticoreHealthy,
		DocumentsLoaded:  len(app.Documents),
		VectorizerReady:  app.Vectorizer != nil,
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

	// Reset and recreate database schema
	if err := app.Manticore.CreateSchema(); err != nil {
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
