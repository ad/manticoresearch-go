package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ad/manticoresearch-go/internal/document"
	"github.com/ad/manticoresearch-go/internal/handlers"
	"github.com/ad/manticoresearch-go/internal/manticore"
	"github.com/ad/manticoresearch-go/internal/models"
	"github.com/ad/manticoresearch-go/internal/vectorizer"
)

func main() {
	fmt.Println("Manticore Search Tester")

	// Run API tests if requested
	if len(os.Args) > 1 && os.Args[1] == "test-api" {
		runAPITests()
		return
	}

	// Initialize application state
	app := handlers.NewAppState()

	// Initialize Manticore HTTP client from environment
	client, err := manticore.NewClientFromEnvironment()
	if err != nil {
		log.Printf("Warning: Failed to create Manticore client: %v", err)
		log.Println("API will still start, but search functionality may be limited")
	} else {
		app.Manticore = client
	}

	// Wait for Manticore to be ready and connect
	log.Println("Waiting for Manticore Search to be ready...")
	if err := app.Manticore.WaitForReady(60 * time.Second); err != nil {
		log.Printf("Warning: Failed to connect to Manticore: %v", err)
		log.Println("API will still start, but search functionality may be limited")
	} else {
		// Initialize database and index documents
		if err := initializeDatabase(app); err != nil {
			log.Printf("Warning: Failed to initialize database: %v", err)
		}
	}

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Setup routes
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/search", app.SearchHandler)
	mux.HandleFunc("/api/status", app.StatusHandler)
	mux.HandleFunc("/api/reindex", app.ReindexHandler)

	// Serve static files for web interface
	staticDir := "./static"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		log.Printf("Warning: Static directory '%s' not found, creating basic API response", staticDir)
		// Fallback to API-only mode
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				fmt.Fprintf(w, "Manticore Search Tester API\n\nAvailable endpoints:\n- GET /api/search?query=<query>&mode=<mode>&page=<page>&limit=<limit>\n- GET /api/status\n- POST /api/reindex\n\nNote: Web interface files not found in ./static directory")
			} else {
				http.NotFound(w, r)
			}
		})
	} else {
		// Serve index.html for root path
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.ServeFile(w, r, staticDir+"/index.html")
			} else {
				// Try to serve static file, fallback to 404
				filePath := staticDir + r.URL.Path
				if _, err := os.Stat(filePath); err == nil {
					http.ServeFile(w, r, filePath)
				} else {
					http.NotFound(w, r)
				}
			}
		})
		log.Printf("Web interface available at http://localhost:%s", port)
	}

	log.Printf("Server starting on port %s", port)
	log.Printf("API endpoints available at:")
	log.Printf("  - GET  /api/search")
	log.Printf("  - GET  /api/status")
	log.Printf("  - POST /api/reindex")

	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// initializeDatabase sets up the database schema and indexes documents
func initializeDatabase(app *handlers.AppState) error {
	log.Println("Initializing database and indexing documents...")

	// Get data directory
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	// Load documents from data directory
	documents, err := document.ScanDataDirectory(dataDir)
	if err != nil {
		return fmt.Errorf("failed to scan data directory: %v", err)
	}

	if len(documents) == 0 {
		log.Println("Warning: No documents found in data directory")
		return nil
	}

	log.Printf("Found %d documents to index", len(documents))

	// Create and train vectorizer
	vec := vectorizer.NewTFIDFVectorizer()
	vectors := vec.FitTransform(documents)

	// Clear existing data and create fresh schema
	log.Println("Clearing existing data and creating fresh schema...")
	if err := app.Manticore.ResetDatabase(); err != nil {
		log.Printf("Warning: Failed to reset database (this is normal for first run): %v", err)
	}

	// Load AI configuration for schema creation
	aiConfig, err := models.LoadAISearchConfigFromEnvironment()
	if err != nil {
		log.Printf("Warning: Failed to load AI config, using default: %v", err)
		aiConfig = models.DefaultAISearchConfig()
	}

	// Create database schema using new client with AI configuration
	if err := app.Manticore.CreateSchema(aiConfig); err != nil {
		return fmt.Errorf("failed to create schema: %v", err)
	}

	// Index documents using new client
	if err := app.Manticore.IndexDocuments(documents, vectors); err != nil {
		return fmt.Errorf("failed to index documents: %v", err)
	}

	// Update application state
	app.Documents = documents
	app.Vectorizer = vec
	app.Vectors = vectors

	log.Printf("Successfully initialized database with %d documents", len(documents))
	return nil
}

// runAPITests runs basic API tests for debugging
func runAPITests() {
	fmt.Println("Running API endpoint tests...")

	// Test search endpoint
	fmt.Println("\n1. Testing search endpoint...")
	app := handlers.NewAppState()

	// Load test documents
	documents, err := document.ScanDataDirectory("data")
	if err != nil {
		fmt.Printf("Warning: Could not load test documents: %v\n", err)
	} else {
		if len(documents) > 5 {
			app.Documents = documents[:5]
		} else {
			app.Documents = documents
		}
		fmt.Printf("Loaded %d test documents\n", len(app.Documents))
	}

	fmt.Println("\nAPI tests completed!")
}
